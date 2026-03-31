package usecase

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/pkg/logger"
)

const (
	maxToolInvocationRounds = 5
	maxToolResultRunes      = 8000

	minToolExecSeconds     = 30
	maxToolExecSeconds     = 300
	defaultToolExecSeconds = 120
)

func toolExecutionDuration(sessionTimeoutSec int32) time.Duration {
	s := int64(sessionTimeoutSec)
	if s <= 0 {
		s = defaultToolExecSeconds
	}

	if s < minToolExecSeconds {
		s = minToolExecSeconds
	}

	if s > maxToolExecSeconds {
		s = maxToolExecSeconds
	}

	return time.Duration(s) * time.Second
}

func runFnWithContext[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		return fn()
	}

	type result struct {
		val T
		err error
	}

	ch := make(chan result, 1)
	go func() {
		v, err := fn()
		ch <- result{v, err}
	}()

	select {
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	case r := <-ch:
		return r.val, r.err
	}
}

type cohereActionRow struct {
	ToolName   string          `json:"tool_name"`
	Parameters json.RawMessage `json:"parameters"`
}

func cloneGenParamsForToolCalls(in *domain.GenerationParams) *domain.GenerationParams {
	if in == nil {
		return nil
	}

	out := *in
	out.ResponseFormat = nil

	return &out
}

func allowedToolNameSet(tools []domain.Tool) map[string]struct{} {
	m := make(map[string]struct{})
	for _, t := range tools {
		n := normalizeToolName(t.Name)
		if n != "" {
			m[n] = struct{}{}
		}
	}

	return m
}

func normalizeToolName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "_")

	return s
}

func drainLLMStringChannel(ch chan string) string {
	var b strings.Builder
	for s := range ch {
		b.WriteString(s)
	}
	return b.String()
}

var reActionJSON = regexp.MustCompile("(?is)(?:Action|Действие):\\s*" + "```" + `json\s*([\s\S]*?)` + "```")

func extractCohereActionJSON(text string) string {
	m := reActionJSON.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}

	return strings.TrimSpace(m[1])
}

func extractFirstFencedToolArray(text string) string {
	s := text
	for len(s) > 0 {
		start := strings.Index(s, "```")
		if start < 0 {
			return ""
		}

		afterOpen := s[start+3:]
		bodyStart := 0
		if nl := strings.IndexByte(afterOpen, '\n'); nl >= 0 {
			first := strings.TrimSpace(afterOpen[:nl])
			if len(first) > 0 && !strings.ContainsAny(first, " \t") {
				bodyStart = nl + 1
			}
		}

		rest := afterOpen[bodyStart:]
		end := strings.Index(rest, "```")
		if end < 0 {
			return ""
		}

		raw := strings.TrimSpace(rest[:end])
		if strings.HasPrefix(strings.TrimSpace(raw), "[") {
			if rows, err := parseCohereActionList(raw); err == nil && len(rows) > 0 && toolActionRowsHaveNames(rows) {
				return raw
			}
		}

		s = afterOpen
	}

	return ""
}

func extractFirstJSONArray(text string) string {
	idx := strings.Index(text, "```json")
	if idx < 0 {
		return ""
	}

	rest := text[idx+len("```json"):]
	end := strings.Index(rest, "```")
	if end < 0 {
		return ""
	}

	raw := strings.TrimSpace(rest[:end])
	if !strings.HasPrefix(strings.TrimSpace(raw), "[") {
		return ""
	}

	return raw
}

func extractLeadingJSONArray(text string) string {
	s := strings.TrimSpace(text)
	if len(s) == 0 || s[0] != '[' {
		return ""
	}

	depth := 0
	inString := false
	escape := false
	for i := 0; i < len(s); i++ {
		b := s[i]
		if escape {
			escape = false
			continue
		}

		if inString {
			if b == '\\' {
				escape = true
			} else if b == '"' {
				inString = false
			}

			continue
		}

		switch b {
		case '"':
			inString = true
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}

	return ""
}

func extractEmbeddedJSONArray(text string) string {
	s := text
	for {
		idx := strings.Index(s, "[")
		if idx < 0 {
			return ""
		}

		sub := s[idx:]
		candidate := extractLeadingJSONArray(sub)
		if candidate != "" {
			rows, err := parseCohereActionList(candidate)
			if err == nil && len(rows) > 0 && toolActionRowsHaveNames(rows) {
				return candidate
			}
		}

		s = s[idx+1:]
	}
}

func toolActionRowsHaveNames(rows []cohereActionRow) bool {
	for _, r := range rows {
		if strings.TrimSpace(r.ToolName) != "" {
			return true
		}
	}

	return false
}

func extractToolActionBlob(text string) string {
	if s := extractCohereActionJSON(text); s != "" {
		return s
	}

	if s := extractFirstJSONArray(text); s != "" {
		return s
	}

	if s := extractFirstFencedToolArray(text); s != "" {
		return s
	}

	if s := extractLeadingJSONArray(text); s != "" {
		return s
	}

	return extractEmbeddedJSONArray(text)
}

func parseCohereActionList(blob string) ([]cohereActionRow, error) {
	blob = strings.TrimSpace(blob)
	if blob == "" {
		return nil, nil
	}

	var rows []cohereActionRow
	if err := json.Unmarshal([]byte(blob), &rows); err != nil {
		return nil, err
	}

	return rows, nil
}

func isDirectAnswerTool(name string) bool {
	switch normalizeToolName(name) {
	case
		"directly_answer",
		"directlyanswer":
		return true
	default:
		return false
	}
}

func directAnswerText(params json.RawMessage) string {
	if len(params) == 0 {
		return ""
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(params, &m); err != nil {
		return strings.TrimSpace(string(params))
	}

	for _, key := range []string{"answer", "text", "message", "content"} {
		if v, ok := m[key]; ok {
			var s string
			if err := json.Unmarshal(v, &s); err == nil {
				return strings.TrimSpace(s)
			}
		}
	}

	return strings.TrimSpace(string(params))
}

func toolCallsToOpenAIJSON(calls []cohereActionRow) (string, error) {
	type fn struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}

	type item struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function fn     `json:"function"`
	}

	out := make([]item, 0, len(calls))
	for i, c := range calls {
		id := fmt.Sprintf("call_%d", i+1)
		args := strings.TrimSpace(string(c.Parameters))
		if args == "" {
			args = "{}"
		}

		out = append(out, item{
			ID:   id,
			Type: "function",
			Function: fn{
				Name:      c.ToolName,
				Arguments: args,
			},
		})
	}

	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func truncateToolResult(s string) string {
	if utf8.RuneCountInString(s) <= maxToolResultRunes {
		return s
	}
	r := []rune(s)

	return string(r[:maxToolResultRunes]) + "\n…(обрезано)"
}

func (c *ChatUseCase) sendMessageWithToolLoop(
	ctx context.Context,
	userID int,
	sessionID int64,
	resolvedModel string,
	messagesForLLM []*domain.Message,
	stopSequences []string,
	timeoutSeconds int32,
	genParams *domain.GenerationParams,
) (chan ChatStreamChunk, error) {
	if genParams == nil || len(genParams.Tools) == 0 {
		return nil, fmt.Errorf("внутренняя ошибка: tool loop без tools")
	}

	out := make(chan ChatStreamChunk, 64)
	go c.runChatToolLoop(ctx, userID, sessionID, resolvedModel, messagesForLLM, stopSequences, timeoutSeconds, genParams, out)

	return out, nil
}

func (c *ChatUseCase) runChatToolLoop(
	ctx context.Context,
	userID int,
	sessionID int64,
	resolvedModel string,
	messagesForLLM []*domain.Message,
	stopSequences []string,
	timeoutSeconds int32,
	genParams *domain.GenerationParams,
	out chan<- ChatStreamChunk,
) {
	defer close(out)

	send := func(chunk ChatStreamChunk) bool {
		select {
		case <-ctx.Done():
			return false
		case out <- chunk:
			return true
		}
	}

	sendErr := func(err error) {
		if err == nil {
			return
		}
		s := err.Error()
		if s == "" {
			s = "ошибка"
		}
		_ = send(ChatStreamChunk{Kind: StreamChunkKindText, Text: s, MessageID: 0})
	}

	sendFinal := func(msgID int64, text string) {
		_ = c.messageRepo.UpdateContent(context.Background(), msgID, text)
		_ = send(ChatStreamChunk{Kind: StreamChunkKindText, Text: text, MessageID: msgID})
	}

	allowed := allowedToolNameSet(genParams.Tools)
	gp := cloneGenParamsForToolCalls(genParams)
	history := append([]*domain.Message(nil), messagesForLLM...)

	for round := 0; round < maxToolInvocationRounds; round++ {
		ch, err := c.llmRepo.SendMessage(ctx, sessionID, resolvedModel, history, stopSequences, timeoutSeconds, gp)
		if err != nil {
			sendErr(err)
			return
		}

		full := drainLLMStringChannel(ch)
		full = strings.TrimSpace(full)
		if full == "" {
			sendErr(fmt.Errorf("модель вернула пустой ответ (tool loop)"))
			return
		}

		blob := extractToolActionBlob(full)

		if blob == "" {
			am := domain.NewMessage(sessionID, full, domain.MessageRoleAssistant)
			if err := c.messageRepo.Create(ctx, am); err != nil {
				sendErr(err)
				return
			}

			sendFinal(am.Id, full)
			return
		}

		rows, err := parseCohereActionList(blob)
		if err != nil {
			logger.W("ChatUseCase: разбор Action JSON: %v - трактуем ответ как финальный текст", err)
			am := domain.NewMessage(sessionID, full, domain.MessageRoleAssistant)
			if err := c.messageRepo.Create(ctx, am); err != nil {
				sendErr(err)
				return
			}

			sendFinal(am.Id, full)
			return
		}

		if len(rows) == 0 {
			am := domain.NewMessage(sessionID, full, domain.MessageRoleAssistant)
			if err := c.messageRepo.Create(ctx, am); err != nil {
				sendErr(err)
				return
			}

			sendFinal(am.Id, full)
			return
		}

		if len(rows) == 1 && isDirectAnswerTool(rows[0].ToolName) {
			ans := directAnswerText(rows[0].Parameters)
			if ans == "" {
				ans = full
			}

			am := domain.NewMessage(sessionID, ans, domain.MessageRoleAssistant)
			if err := c.messageRepo.Create(ctx, am); err != nil {
				sendErr(err)
				return
			}

			sendFinal(am.Id, ans)
			return
		}

		execRows := filterExecutableToolRows(rows)
		if len(execRows) == 0 {
			am := domain.NewMessage(sessionID, full, domain.MessageRoleAssistant)
			if err := c.messageRepo.Create(ctx, am); err != nil {
				sendErr(err)
				return
			}

			sendFinal(am.Id, full)
			return
		}

		toolCallsJSON, err := toolCallsToOpenAIJSON(execRows)
		if err != nil {
			sendErr(err)
			return
		}

		assist := domain.NewMessage(sessionID, full, domain.MessageRoleAssistant)
		assist.ToolCallsJSON = toolCallsJSON
		if err := c.messageRepo.Create(ctx, assist); err != nil {
			sendErr(err)
			return
		}
		history = append(history, assist)

		for i, row := range execRows {
			name := normalizeToolName(row.ToolName)
			if _, ok := allowed[name]; !ok {
				sendErr(fmt.Errorf("инструмент %q не объявлен в настройках сессии", row.ToolName))
				return
			}

			st := strings.TrimSpace(row.ToolName)
			if st == "" {
				st = name
			}

			if !send(ChatStreamChunk{Kind: StreamChunkKindToolStatus, Text: "Выполняется: " + st, ToolName: st, MessageID: 0}) {
				return
			}

			toolCtx, cancelTool := context.WithTimeout(ctx, toolExecutionDuration(timeoutSeconds))
			res, err := c.executeDeclaredTool(toolCtx, userID, sessionID, name, row.Parameters)
			cancelTool()
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					sendErr(fmt.Errorf("таймаут выполнения инструмента %q", row.ToolName))
					return
				}

				sendErr(err)
				return
			}

			tm := domain.NewMessage(sessionID, truncateToolResult(res), domain.MessageRoleTool)
			tm.ToolName = row.ToolName
			tm.ToolCallID = fmt.Sprintf("call_%d", i+1)
			if err := c.messageRepo.Create(ctx, tm); err != nil {
				sendErr(err)
				return
			}

			history = append(history, tm)
		}
	}

	sendErr(fmt.Errorf("превышено число итераций tool-calling (%d)", maxToolInvocationRounds))
}

func (c *ChatUseCase) executeDeclaredTool(ctx context.Context, userID int, sessionID int64, nameNorm string, params json.RawMessage) (string, error) {
	switch nameNorm {
	case "apply_spreadsheet":
		return c.toolApplySpreadsheet(ctx, userID, sessionID, params)
	case "build_docx":
		return c.toolBuildDocx(ctx, userID, sessionID, params)
	case "apply_markdown_patch":
		return c.toolApplyMarkdownPatch(ctx, userID, sessionID, params)
	case "put_session_file":
		return c.toolPutSessionFile(ctx, userID, sessionID, params)
	default:
		return "", fmt.Errorf("инструмент %q пока не реализован на сервере", nameNorm)
	}
}

func mustStringField(m map[string]json.RawMessage, key string) (string, error) {
	v, ok := m[key]
	if !ok {
		return "", fmt.Errorf("отсутствует поле %q", key)
	}

	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return "", fmt.Errorf("поле %q: ожидается строка", key)
	}

	return strings.TrimSpace(s), nil
}

func optionalStringField(m map[string]json.RawMessage, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}

	var s string
	_ = json.Unmarshal(v, &s)
	return strings.TrimSpace(s)
}

func optionalInt64Field(m map[string]json.RawMessage, key string) (int64, bool, error) {
	v, ok := m[key]
	if !ok {
		return 0, false, nil
	}

	var f float64
	if err := json.Unmarshal(v, &f); err != nil {
		return 0, false, err
	}

	return int64(f), true, nil
}

func optionalInt32Field(m map[string]json.RawMessage, key string) (int32, bool, error) {
	v, ok := m[key]
	if !ok {
		return 0, false, nil
	}

	var f float64
	if err := json.Unmarshal(v, &f); err != nil {
		return 0, false, err
	}

	return int32(f), true, nil
}

func (c *ChatUseCase) toolApplySpreadsheet(ctx context.Context, userID int, sessionID int64, params json.RawMessage) (string, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(params, &m); err != nil {
		return "", fmt.Errorf("parameters apply_spreadsheet: %w", err)
	}

	ops, err := mustStringField(m, "operations_json")
	if err != nil {
		return "", err
	}

	previewSheet := optionalStringField(m, "preview_sheet")
	previewRange := optionalStringField(m, "preview_range")

	var workbook []byte
	if fid, ok, err := optionalInt64Field(m, "workbook_file_id"); err != nil {
		return "", err
	} else if ok && fid > 0 {
		_, data, err := c.loadSessionAttachmentForSend(ctx, userID, sessionID, fid)
		if err != nil {
			return "", err
		}
		workbook = data
	}

	var wbIn []byte
	if len(workbook) > 0 {
		wbIn = bytes.Clone(workbook)
	}

	type sheetOut struct {
		wbOut       []byte
		previewTSV  string
		exportedCSV string
	}

	so, err := runFnWithContext(ctx, func() (sheetOut, error) {
		wb, p, e, err := c.ApplySpreadsheet(context.Background(), wbIn, ops, previewSheet, previewRange)
		return sheetOut{wb, p, e}, err
	})
	if err != nil {
		return "", err
	}

	wbOut, previewTSV, exportedCSV := so.wbOut, so.previewTSV, so.exportedCSV

	out := map[string]any{
		"ok":           true,
		"preview_tsv":  truncateToolResult(previewTSV),
		"exported_csv": truncateToolResult(exportedCSV),
	}
	if len(wbOut) > 0 {
		fname := "workbook.xlsx"
		if fid, ok, err := optionalInt64Field(m, "workbook_file_id"); err == nil && ok && fid > 0 {
			fname = fmt.Sprintf("workbook_%d.xlsx", fid)
		}

		id, err := c.PutSessionFile(ctx, userID, sessionID, fname, wbOut, 0)
		if err != nil {
			n := min(256, len(wbOut))
			out["workbook_base64_prefix"] = base64.StdEncoding.EncodeToString(wbOut[:n])
			out["put_file_error"] = err.Error()
		} else {
			out["workbook_file_id"] = id
		}
	}

	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func filterExecutableToolRows(rows []cohereActionRow) []cohereActionRow {
	var out []cohereActionRow
	for _, r := range rows {
		if !isDirectAnswerTool(r.ToolName) {
			out = append(out, r)
		}
	}

	return out
}

func (c *ChatUseCase) toolBuildDocx(ctx context.Context, userID int, sessionID int64, params json.RawMessage) (string, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(params, &m); err != nil {
		return "", fmt.Errorf("parameters build_docx: %w", err)
	}

	spec, err := mustStringField(m, "spec_json")
	if err != nil {
		return "", err
	}

	docx, err := runFnWithContext(ctx, func() ([]byte, error) {
		return c.BuildDocx(context.Background(), spec)
	})

	if err != nil {
		return "", err
	}

	id, err := c.PutSessionFile(ctx, userID, sessionID, "document.docx", docx, 0)
	if err != nil {
		return "", err
	}

	out := map[string]any{
		"ok":       true,
		"file_id":  id,
		"filename": "document.docx",
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func (c *ChatUseCase) toolPutSessionFile(ctx context.Context, userID int, sessionID int64, params json.RawMessage) (string, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(params, &m); err != nil {
		return "", fmt.Errorf("parameters put_session_file: %w", err)
	}

	fname, err := mustStringField(m, "filename")
	if err != nil {
		return "", err
	}

	b64 := strings.TrimSpace(optionalStringField(m, "content_base64"))
	utf8Body := optionalStringField(m, "content")
	var body []byte
	if b64 != "" {
		dec, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return "", fmt.Errorf("content_base64: %w", err)
		}

		body = dec
	} else {
		if _, has := m["content"]; !has {
			return "", fmt.Errorf("нужен параметр content (строка UTF-8) или content_base64")
		}

		body = []byte(utf8Body)
	}

	if len(body) == 0 {
		return "", fmt.Errorf("пустой content")
	}

	var ttl int32
	if v, ok, err := optionalInt32Field(m, "ttl_seconds"); err != nil {
		return "", err
	} else if ok {
		ttl = v
	}

	id, err := c.PutSessionFile(ctx, userID, sessionID, fname, body, ttl)
	if err != nil {
		return "", err
	}

	base := filepath.Base(fname)
	out := map[string]any{"ok": true, "file_id": id, "filename": base}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func (c *ChatUseCase) toolApplyMarkdownPatch(ctx context.Context, userID int, sessionID int64, params json.RawMessage) (string, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(params, &m); err != nil {
		return "", fmt.Errorf("parameters apply_markdown_patch: %w", err)
	}

	patch, err := mustStringField(m, "patch_json")
	if err != nil {
		return "", err
	}

	baseText := optionalStringField(m, "base_text")
	fid, hasFid, err := optionalInt64Field(m, "base_file_id")
	if err != nil {
		return "", err
	}

	var base string
	if hasFid && fid > 0 {
		if strings.TrimSpace(baseText) != "" {
			return "", fmt.Errorf("нельзя одновременно задавать base_text и base_file_id")
		}

		_, data, err := c.loadSessionAttachmentForSend(ctx, userID, sessionID, fid)
		if err != nil {
			return "", err
		}

		if !utf8.Valid(data) {
			return "", fmt.Errorf("base_file_id: содержимое не UTF-8")
		}

		base = string(data)
	} else {
		base = baseText
	}

	text, err := runFnWithContext(ctx, func() (string, error) {
		return c.ApplyMarkdownPatch(context.Background(), base, patch)
	})

	if err != nil {
		return "", err
	}

	out := map[string]any{"ok": true, "text": truncateToolResult(text)}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}

	return string(b), nil
}
