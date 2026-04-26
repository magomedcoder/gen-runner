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
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/mcpclient"
	"github.com/magomedcoder/gen/internal/service"
	"github.com/magomedcoder/gen/pkg/logger"
)

const (
	maxToolResultRunes     = 8000
	minToolExecSeconds     = 30
	maxToolExecSeconds     = 300
	defaultToolExecSeconds = 120
	defaultToolLoopRounds  = 12
	maxToolLoopRoundsCap   = 128
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

type executableToolCall struct {
	RequestedName string
	ResolvedName  string
	Parameters    json.RawMessage
}

type toolLoopEnvKey struct{}

type toolLoopEnv struct {
	RunnerAddr             string
	ResolvedModel          string
	StopSequences          []string
	TimeoutSeconds         int32
	SamplingGen            *domain.GenerationParams
	UserTurnHasVisionImage bool
}

func withToolLoopEnv(ctx context.Context, env *toolLoopEnv) context.Context {
	if env == nil {
		return ctx
	}

	return context.WithValue(ctx, toolLoopEnvKey{}, env)
}

func toolLoopEnvFrom(ctx context.Context) *toolLoopEnv {
	v, _ := ctx.Value(toolLoopEnvKey{}).(*toolLoopEnv)
	return v
}

func samplingGenParamsForMCP(gp *domain.GenerationParams) *domain.GenerationParams {
	if gp == nil {
		return &domain.GenerationParams{}
	}

	out := *gp
	out.Tools = nil
	out.ResponseFormat = nil

	return &out
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

var mcpAliasFromModelRe = regexp.MustCompile(`^mcp_(\d+)_h([0-9a-f]+)$`)

func tryRecoverSingleMCPServerToolAlias(genParams *domain.GenerationParams, n string) (canon string, serverID int64, ok bool) {
	if genParams == nil {
		return "", 0, false
	}

	m := mcpAliasFromModelRe.FindStringSubmatch(n)
	if len(m) != 3 {
		return "", 0, false
	}

	sid, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || sid <= 0 {
		return "", 0, false
	}

	var canonList []string
	for _, t := range genParams.Tools {
		c := normalizeToolName(t.Name)
		if c == "" {
			continue
		}

		tsid, _, ok := mcpclient.ParseToolAlias(c)
		if !ok || tsid != sid {
			continue
		}

		canonList = append(canonList, c)
	}

	if len(canonList) != 1 {
		return "", sid, false
	}

	return canonList[0], sid, true
}

func logToolResolveMismatch(genParams *domain.GenerationParams, requested string) {
	n := normalizeToolName(requested)
	var mcpNames []string
	if genParams != nil {
		for _, t := range genParams.Tools {
			c := normalizeToolName(t.Name)
			if strings.HasPrefix(c, "mcp_") {
				mcpNames = append(mcpNames, c)
			}
		}
	}

	const maxList = 24
	list := strings.Join(mcpNames, ", ")
	if len(mcpNames) > maxList {
		list = strings.Join(mcpNames[:maxList], ", ") + fmt.Sprintf(" …(всего mcp_*=%d)", len(mcpNames))
	}

	logger.W("ChatUseCase tool-loop: phase=resolve_tools_mismatch requested=%q normalized=%q mcp_declared_count=%d declared_mcp_sample=[%s]", requested, n, len(mcpNames), list)
	if sid, orig, ok := mcpclient.ParseToolAlias(n); ok {
		logger.W("ChatUseCase tool-loop: phase=resolve_tools_mismatch decoded server_id=%d name_from_model_hex=%q", sid, orig)
	} else if mcpAliasFromModelRe.MatchString(n) {
		logger.W("ChatUseCase tool-loop: phase=resolve_tools_mismatch looks_like_mcp_alias_but_hex_decode_failed normalized=%q", n)
	}
}

func resolveDeclaredToolName(genParams *domain.GenerationParams, raw string) (string, bool) {
	n := normalizeToolName(raw)
	if genParams == nil || n == "" {
		return "", false
	}

	allowed := allowedToolNameSet(genParams.Tools)
	if _, ok := allowed[n]; ok {
		return n, true
	}

	type cand struct {
		sid   int64
		canon string
	}

	var hits []cand
	for _, t := range genParams.Tools {
		canon := normalizeToolName(t.Name)
		if canon == "" {
			continue
		}
		if _, ok := allowed[canon]; !ok {
			continue
		}

		sid, orig, ok := mcpclient.ParseToolAlias(canon)
		if !ok || sid <= 0 {
			continue
		}

		if normalizeToolName(orig) == n {
			hits = append(hits, cand{sid: sid, canon: canon})
		}
	}

	if len(hits) == 0 {
		if canon, sid, ok := tryRecoverSingleMCPServerToolAlias(genParams, n); ok {
			logger.W("ChatUseCase tool-loop: phase=mcp_alias_recovered server_id=%d requested=%q resolved=%q (на сервере один MCP-tool; подменён неверный h… суффикс)", sid, strings.TrimSpace(raw), canon)
			return canon, true
		}
		return "", false
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].sid != hits[j].sid {
			return hits[i].sid < hits[j].sid
		}

		return hits[i].canon < hits[j].canon
	})

	return hits[0].canon, true
}

func normalizeToolName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "_")

	return s
}

func drainLLMStreamChannelForward(ch chan domain.LLMStreamChunk, forward func(c domain.LLMStreamChunk) bool) (raw string, streamedNonEmpty bool) {
	var b strings.Builder
	for c := range ch {
		if c.Content != "" {
			b.WriteString(c.Content)
		}
		if c.Content == "" && c.ReasoningContent == "" {
			continue
		}

		if !forward(c) {
			for c2 := range ch {
				b.WriteString(c2.Content)
			}

			return b.String(), true
		}

		streamedNonEmpty = true
	}

	return b.String(), streamedNonEmpty
}

func streamToolRoundComplete(send func(ChatStreamChunk) bool, messageID int64, streamed bool, modelFullTrimmed, canonical string) {
	if !streamed {
		_ = send(ChatStreamChunk{
			Kind:      StreamChunkKindText,
			Text:      canonical,
			MessageID: messageID,
		})
		return
	}

	if canonical == modelFullTrimmed {
		_ = send(ChatStreamChunk{
			Kind:      StreamChunkKindText,
			Text:      "",
			MessageID: messageID,
		})
		return
	}

	_ = send(ChatStreamChunk{
		Kind:      StreamChunkKindText,
		Text:      canonical,
		MessageID: messageID,
	})
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
		before, _, ok := strings.Cut(rest, "```")
		if !ok {
			return ""
		}

		raw := strings.TrimSpace(before)
		tr := strings.TrimSpace(raw)
		if strings.HasPrefix(tr, "[") || strings.HasPrefix(tr, "{") {
			if rows, err := parseCohereActionList(raw); err == nil && len(rows) > 0 && toolActionRowsHaveNames(rows) {
				return raw
			}
		}

		s = afterOpen
	}

	return ""
}

func extractFirstJSONArray(text string) string {
	_, after, ok := strings.Cut(text, "```json")
	if !ok {
		return ""
	}

	rest := after
	before, _, ok := strings.Cut(rest, "```")
	if !ok {
		return ""
	}

	raw := strings.TrimSpace(before)
	tr := strings.TrimSpace(raw)
	if !strings.HasPrefix(tr, "[") && !strings.HasPrefix(tr, "{") {
		return ""
	}

	if rows, err := parseCohereActionList(raw); err != nil || len(rows) == 0 || !toolActionRowsHaveNames(rows) {
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

func extractLeadingJSONObject(text string) string {
	s := strings.TrimSpace(text)
	if len(s) == 0 || s[0] != '{' {
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
		case '{':
			depth++
		case '}':
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

func extractEmbeddedJSONObject(text string) string {
	s := text
	for {
		idx := strings.Index(s, "{")
		if idx < 0 {
			return ""
		}

		sub := s[idx:]
		candidate := extractLeadingJSONObject(sub)
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

	if s := extractLeadingJSONObject(text); s != "" {
		if rows, err := parseCohereActionList(s); err == nil && len(rows) > 0 && toolActionRowsHaveNames(rows) {
			return s
		}
	}

	if s := extractEmbeddedJSONArray(text); s != "" {
		return s
	}

	return extractEmbeddedJSONObject(text)
}

func parseCohereActionList(blob string) ([]cohereActionRow, error) {
	blob = strings.TrimSpace(blob)
	if blob == "" {
		return nil, nil
	}

	var asSlice []cohereActionRow
	if err := json.Unmarshal([]byte(blob), &asSlice); err == nil {
		if len(asSlice) > 0 {
			return normalizeToolActionRows(asSlice)
		}

		if strings.HasPrefix(strings.TrimSpace(blob), "[") {
			return nil, fmt.Errorf("пустой список вызовов инструментов")
		}
	}

	type legacyNameArgs struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	var legacy legacyNameArgs
	if err := json.Unmarshal([]byte(blob), &legacy); err == nil && strings.TrimSpace(legacy.Name) != "" {
		args := legacy.Arguments
		if len(args) == 0 || string(args) == "null" {
			args = json.RawMessage(`{}`)
		}

		return normalizeToolActionRows([]cohereActionRow{{
			ToolName:   strings.TrimSpace(legacy.Name),
			Parameters: args,
		}})
	}

	type legacyToolArgs struct {
		Tool      string          `json:"tool"`
		Arguments json.RawMessage `json:"arguments"`
	}
	var legacyTool legacyToolArgs
	if err := json.Unmarshal([]byte(blob), &legacyTool); err == nil && strings.TrimSpace(legacyTool.Tool) != "" {
		args := legacyTool.Arguments
		if len(args) == 0 || string(args) == "null" {
			args = json.RawMessage(`{}`)
		}

		return normalizeToolActionRows([]cohereActionRow{{
			ToolName:   strings.TrimSpace(legacyTool.Tool),
			Parameters: args,
		}})
	}

	type legacyToolParams struct {
		ToolName   string          `json:"tool_name"`
		Parameters json.RawMessage `json:"parameters"`
	}
	var tp legacyToolParams
	if err := json.Unmarshal([]byte(blob), &tp); err == nil && strings.TrimSpace(tp.ToolName) != "" {
		args := tp.Parameters
		if len(args) == 0 || string(args) == "null" {
			args = json.RawMessage(`{}`)
		}

		return normalizeToolActionRows([]cohereActionRow{{
			ToolName:   strings.TrimSpace(tp.ToolName),
			Parameters: args,
		}})
	}

	return nil, fmt.Errorf("неверный формат вызова инструментов (ожидается JSON-массив с tool_name/parameters или объект name/arguments/tool/arguments)")
}

func normalizeToolActionRows(rows []cohereActionRow) ([]cohereActionRow, error) {
	out := make([]cohereActionRow, 0, len(rows))
	for _, row := range rows {
		params, err := normalizeToolParameters(row.Parameters)
		if err != nil {
			return nil, fmt.Errorf("arguments для %q: %w", strings.TrimSpace(row.ToolName), err)
		}
		out = append(out, cohereActionRow{
			ToolName:   strings.TrimSpace(row.ToolName),
			Parameters: params,
		})
	}
	return out, nil
}

func normalizeToolParameters(raw json.RawMessage) (json.RawMessage, error) {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(`{}`), nil
	}

	if raw[0] == '"' {
		var encoded string
		if err := json.Unmarshal(raw, &encoded); err != nil {
			return nil, err
		}

		encoded = strings.TrimSpace(encoded)
		if encoded == "" {
			return json.RawMessage(`{}`), nil
		}

		raw = json.RawMessage(encoded)
	}

	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}

	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	return json.RawMessage(b), nil
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

func maxToolInvocationRounds(reg *service.Registry) int {
	n := defaultToolLoopRounds
	if reg != nil {
		if hinted := reg.AggregateChatHints().MaxToolInvocationRounds; hinted > 0 {
			n = hinted
		}
	}
	if n < 1 {
		return 1
	}
	if n > maxToolLoopRoundsCap {
		return maxToolLoopRoundsCap
	}
	return n
}

func resolveExecutableToolCalls(genParams *domain.GenerationParams, rows []cohereActionRow) ([]executableToolCall, error) {
	out := make([]executableToolCall, 0, len(rows))
	for _, row := range rows {
		resolved, ok := resolveDeclaredToolName(genParams, row.ToolName)
		if !ok {
			logToolResolveMismatch(genParams, row.ToolName)
			return nil, fmt.Errorf("инструмент %q не объявлен в настройках сессии (имя должно совпасть с одним из tools; для MCP - точный алиас mcp_<id>_h<hex> или короткое имя инструмента на сервере)", row.ToolName)
		}
		out = append(out, executableToolCall{
			RequestedName: row.ToolName,
			ResolvedName:  resolved,
			Parameters:    row.Parameters,
		})
	}
	return out, nil
}

func executableCallsToActionRows(in []executableToolCall) []cohereActionRow {
	out := make([]cohereActionRow, 0, len(in))
	for _, c := range in {
		out = append(out, cohereActionRow{
			ToolName:   c.ResolvedName,
			Parameters: c.Parameters,
		})
	}
	return out
}

func truncateToolResult(s string) string {
	if utf8.RuneCountInString(s) <= maxToolResultRunes {
		return s
	}
	r := []rune(s)

	return string(r[:maxToolResultRunes]) + "\n...(обрезано)"
}

func (c *ChatUseCase) sendMessageWithToolLoop(
	ctx context.Context,
	userID int,
	sessionID int64,
	runnerAddr string,
	resolvedModel string,
	messagesForLLM []*domain.Message,
	stopSequences []string,
	timeoutSeconds int32,
	genParams *domain.GenerationParams,
	historyInitiallyTrimmed bool,
	ragStream *ragStreamMeta,
) (chan ChatStreamChunk, error) {
	if genParams == nil || len(genParams.Tools) == 0 {
		return nil, fmt.Errorf("внутренняя ошибка: tool loop без tools")
	}

	out := make(chan ChatStreamChunk, 64)
	logger.I("ChatUseCase tool-loop: session_id=%d user_id=%d phase=spawn_goroutine runner=%q model=%q tools=%d stop_seq=%d timeout_sec=%d", sessionID, userID, runnerAddr, resolvedModel, len(genParams.Tools), len(stopSequences), timeoutSeconds)
	go c.runChatToolLoop(ctx, userID, sessionID, runnerAddr, resolvedModel, messagesForLLM, stopSequences, timeoutSeconds, genParams, historyInitiallyTrimmed, ragStream, out)

	return out, nil
}

func (c *ChatUseCase) runChatToolLoop(
	ctx context.Context,
	userID int,
	sessionID int64,
	runnerAddr string,
	resolvedModel string,
	messagesForLLM []*domain.Message,
	stopSequences []string,
	timeoutSeconds int32,
	genParams *domain.GenerationParams,
	historyInitiallyTrimmed bool,
	ragStream *ragStreamMeta,
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
		logger.W("ChatUseCase tool-loop: session_id=%d user_id=%d phase=client_error err=%v", sessionID, userID, err)
		s := err.Error()
		if s == "" {
			s = "ошибка"
		}
		_ = send(ChatStreamChunk{Kind: StreamChunkKindText, Text: s, MessageID: 0})
	}
	defer func() {
		if r := recover(); r != nil {
			logger.E("ChatUseCase tool-loop panic: session_id=%d user_id=%d panic=%v", sessionID, userID, r)
			sendErr(fmt.Errorf("внутренняя ошибка обработки инструментов"))
		}
	}()

	gp := cloneGenParamsForToolCalls(genParams)
	history := append([]*domain.Message(nil), messagesForLLM...)
	userTurnVision := lastUserMessageHasVisionAttachment(messagesForLLM)

	logger.I("ChatUseCase tool-loop: session_id=%d user_id=%d phase=enter runner=%q model=%q history_msgs=%d tools=%d rag=%t history_notice=%t user_turn_vision=%t", sessionID, userID, runnerAddr, resolvedModel, len(history), len(gp.Tools), ragStream != nil, historyInitiallyTrimmed, userTurnVision)

	if ragStream != nil {
		_ = send(ragStream.asChunk())
	}

	if historyInitiallyTrimmed {
		_ = send(ChatStreamChunk{Kind: StreamChunkKindNotice, Text: HistoryTruncatedClientNotice})
	}

	maxToolRounds := maxToolInvocationRounds(c.runnerReg)
	logger.I("ChatUseCase tool-loop: session_id=%d user_id=%d max_rounds=%d tools=%d", sessionID, userID, maxToolRounds, len(gp.Tools))

	for round := range maxToolRounds {
		logger.I("ChatUseCase tool-loop: session_id=%d round=%d/%d phase=llm_request runner=%q model=%q history_msgs=%d tools=%d", sessionID, round+1, maxToolRounds, runnerAddr, resolvedModel, len(history), len(gp.Tools))
		ch, runnerToolFn, err := c.llmRepo.SendMessageWithRunnerToolActionOnRunner(ctx, runnerAddr, sessionID, resolvedModel, history, stopSequences, timeoutSeconds, gp)
		if err != nil {
			logger.W("ChatUseCase tool-loop: session_id=%d round=%d phase=llm_request_err runner=%q err=%v", sessionID, round+1, runnerAddr, err)
			sendErr(err)
			return
		}

		raw, streamed := drainLLMStreamChannelForward(ch, func(c domain.LLMStreamChunk) bool {
			if c.ReasoningContent != "" {
				if !send(ChatStreamChunk{Kind: StreamChunkKindReasoning, Text: c.ReasoningContent, MessageID: 0}) {
					return false
				}
			}
			if c.Content != "" {
				return send(ChatStreamChunk{Kind: StreamChunkKindText, Text: c.Content, MessageID: 0})
			}
			return true
		})
		full := strings.TrimSpace(raw)
		logger.I("ChatUseCase tool-loop: session_id=%d round=%d phase=llm_stream_done streamed=%t assistant_text_runes=%d", sessionID, round+1, streamed, utf8.RuneCountInString(full))
		if full == "" {
			logger.W("ChatUseCase tool-loop: session_id=%d round=%d phase=llm_empty_response", sessionID, round+1)
			sendErr(fmt.Errorf("модель вернула пустой ответ (tool loop)"))
			return
		}

		blob := strings.TrimSpace(runnerToolFn())
		blobSource := "runner"
		if blob == "" {
			blob = extractToolActionBlob(full)
			blobSource = "text_extract"
		}
		logger.I("ChatUseCase tool-loop: session_id=%d round=%d phase=model_output blob_source=%s blob_bytes=%d assistant_text_runes=%d", sessionID, round+1, blobSource, len(blob), utf8.RuneCountInString(full))

		if blob == "" {
			logger.I("ChatUseCase tool-loop: session_id=%d round=%d phase=final_answer tool_blob=empty (нет вызовов инструментов)", sessionID, round+1)
			am := domain.NewMessage(sessionID, full, domain.MessageRoleAssistant)
			if err := c.messageRepo.Create(ctx, am); err != nil {
				sendErr(err)
				return
			}

			streamToolRoundComplete(send, am.Id, streamed, full, full)
			return
		}

		rows, err := parseCohereActionList(blob)
		if err != nil {
			logger.W("ChatUseCase tool-loop: session_id=%d round=%d phase=parse_tools err=%v (ответ как финальный текст)", sessionID, round+1, err)
			am := domain.NewMessage(sessionID, full, domain.MessageRoleAssistant)
			if err := c.messageRepo.Create(ctx, am); err != nil {
				sendErr(err)
				return
			}

			streamToolRoundComplete(send, am.Id, streamed, full, full)
			return
		}

		if len(rows) == 0 {
			logger.I("ChatUseCase tool-loop: session_id=%d round=%d phase=parse_tools rows=0 (пустой список вызовов)", sessionID, round+1)
			am := domain.NewMessage(sessionID, full, domain.MessageRoleAssistant)
			if err := c.messageRepo.Create(ctx, am); err != nil {
				sendErr(err)
				return
			}

			streamToolRoundComplete(send, am.Id, streamed, full, full)
			return
		}

		if len(rows) == 1 && isDirectAnswerTool(rows[0].ToolName) {
			logger.I("ChatUseCase tool-loop: session_id=%d round=%d phase=final_direct_answer tool=%q", sessionID, round+1, strings.TrimSpace(rows[0].ToolName))
			ans := directAnswerText(rows[0].Parameters)
			if ans == "" {
				ans = full
			}

			am := domain.NewMessage(sessionID, ans, domain.MessageRoleAssistant)
			if err := c.messageRepo.Create(ctx, am); err != nil {
				sendErr(err)
				return
			}

			streamToolRoundComplete(send, am.Id, streamed, full, ans)
			return
		}

		execRows := filterExecutableToolRows(rows)
		if len(execRows) == 0 {
			logger.I("ChatUseCase tool-loop: session_id=%d round=%d phase=final_after_filter parsed_rows=%d executable=0 (остались только direct_answer/пусто)", sessionID, round+1, len(rows))
			am := domain.NewMessage(sessionID, full, domain.MessageRoleAssistant)
			if err := c.messageRepo.Create(ctx, am); err != nil {
				sendErr(err)
				return
			}

			streamToolRoundComplete(send, am.Id, streamed, full, full)
			return
		}

		logger.I("ChatUseCase tool-loop: session_id=%d round=%d phase=parse_ok parsed_rows=%d executable_rows=%d", sessionID, round+1, len(rows), len(execRows))

		execCalls, err := resolveExecutableToolCalls(gp, execRows)
		if err != nil {
			logger.W("ChatUseCase tool-loop: session_id=%d round=%d phase=resolve_tools_err err=%v", sessionID, round+1, err)
			sendErr(err)
			return
		}
		logger.I("ChatUseCase tool-loop: session_id=%d round=%d phase=tools_resolved calls=%d names=%q", sessionID, round+1, len(execCalls), toolCallNamesForLog(execCalls))

		toolCallsJSON, err := toolCallsToOpenAIJSON(executableCallsToActionRows(execCalls))
		if err != nil {
			logger.W("ChatUseCase tool-loop: session_id=%d round=%d phase=tool_calls_json_err err=%v", sessionID, round+1, err)
			sendErr(err)
			return
		}

		toolResults := make([]string, len(execCalls))
		failedCallIdx := -1
		for i, call := range execCalls {
			st := c.toolProgressDisplayName(ctx, sessionID, call.ResolvedName, call.RequestedName)
			logger.I("ChatUseCase tool-loop: session_id=%d round=%d phase=tool_exec_start index=%d/%d requested=%q resolved=%q display=%q", sessionID, round+1, i+1, len(execCalls), call.RequestedName, call.ResolvedName, st)

			if !send(ChatStreamChunk{Kind: StreamChunkKindToolStatus, Text: "Выполняется: " + st, ToolName: st, MessageID: 0}) {
				return
			}

			toolCtx, cancelTool := context.WithTimeout(ctx, toolExecutionDuration(timeoutSeconds))
			toolCtx = withToolLoopEnv(toolCtx, &toolLoopEnv{
				RunnerAddr:             runnerAddr,
				ResolvedModel:          resolvedModel,
				StopSequences:          stopSequences,
				TimeoutSeconds:         timeoutSeconds,
				SamplingGen:            samplingGenParamsForMCP(gp),
				UserTurnHasVisionImage: userTurnVision,
			})
			res, err := c.executeDeclaredTool(toolCtx, userID, sessionID, gp, call.ResolvedName, call.Parameters)
			cancelTool()
			if err != nil {
				deadline := errors.Is(err, context.DeadlineExceeded)
				if deadline {
					logger.W("ChatUseCase tool-loop: session_id=%d round=%d phase=tool_exec_timeout index=%d/%d tool=%q", sessionID, round+1, i+1, len(execCalls), call.RequestedName)
				} else {
					logger.W("ChatUseCase tool-loop: session_id=%d round=%d phase=tool_exec_err index=%d/%d tool=%q err=%v", sessionID, round+1, i+1, len(execCalls), call.RequestedName, err)
				}

				toolResults[i] = toolLoopErrorToolMessage(call, err, res, deadline)
				failedCallIdx = i
				for j := i + 1; j < len(execCalls); j++ {
					toolResults[j] = toolLoopSkippedToolMessage(execCalls[j], call)
				}

				_ = send(ChatStreamChunk{Kind: StreamChunkKindNotice, Text: "Инструмент вернул ошибку. Формирую понятный ответ…"})
				break
			}

			toolResults[i] = truncateToolResult(res)
			logger.I("ChatUseCase tool-loop: session_id=%d round=%d phase=tool_exec_ok index=%d/%d tool=%q result_runes=%d", sessionID, round+1, i+1, len(execCalls), call.ResolvedName, utf8.RuneCountInString(toolResults[i]))
		}

		if failedCallIdx >= 0 {
			logger.I("ChatUseCase tool-loop: session_id=%d round=%d phase=tool_errors_as_tool_msgs failed_index=%d/%d (следующий LLM-раунд)", sessionID, round+1, failedCallIdx+1, len(execCalls))
		}

		assist := domain.NewMessage(sessionID, full, domain.MessageRoleAssistant)
		assist.ToolCallsJSON = toolCallsJSON

		toolMsgs := make([]*domain.Message, len(execCalls))
		if err := c.chatTx.WithinTx(ctx, func(ctx context.Context, r domain.ChatRepos) error {
			if err := r.Message.Create(ctx, assist); err != nil {
				return err
			}
			for i, call := range execCalls {
				tm := domain.NewMessage(sessionID, toolResults[i], domain.MessageRoleTool)
				tm.ToolName = call.ResolvedName
				tm.ToolCallID = fmt.Sprintf("call_%d", i+1)
				if err := r.Message.Create(ctx, tm); err != nil {
					return err
				}
				toolMsgs[i] = tm
			}
			return nil
		}); err != nil {
			logger.W("ChatUseCase tool-loop: session_id=%d round=%d phase=persist_tx_err err=%v", sessionID, round+1, err)
			sendErr(err)
			return
		}

		logger.I("ChatUseCase tool-loop: session_id=%d round=%d phase=persist_ok assistant_msg_id=%d tool_msgs=%d", sessionID, round+1, assist.Id, len(toolMsgs))

		history = append(history, assist)
		history = append(history, toolMsgs...)
		var loopTrimmed bool
		history, loopTrimmed = c.capLLMHistoryTokens(ctx, history, 1+len(toolMsgs), sessionID, resolvedModel, runnerAddr, false)
		if loopTrimmed {
			logger.I("ChatUseCase tool-loop: session_id=%d round=%d phase=history_trim после добавления tool-сообщений", sessionID, round+1)
			_ = send(ChatStreamChunk{Kind: StreamChunkKindNotice, Text: HistoryTruncatedClientNotice})
		}
		logger.I("ChatUseCase tool-loop: session_id=%d round=%d phase=round_continue history_msgs=%d (следующий LLM-раунд)", sessionID, round+1, len(history))
	}

	logger.W("ChatUseCase tool-loop: session_id=%d user_id=%d phase=loop_exhausted max_rounds=%d", sessionID, userID, maxToolRounds)
	sendErr(fmt.Errorf("превышено число итераций вызова инструментов (%d)", maxToolRounds))
}

func toolLoopErrorToolMessage(call executableToolCall, err error, partialResult string, deadlineExceeded bool) string {
	var b strings.Builder
	b.WriteString("Статус: ошибка выполнения инструмента.\n")
	b.WriteString("Твоя задача: кратко и по-русски объясни пользователю, что пошло не так и что можно сделать (повторить запрос, проверить права, уточнить параметры).\n\n")
	if deadlineExceeded {
		b.WriteString("Причина: истёк таймаут ожидания ответа инструмента.\n")
	} else if err != nil {
		b.WriteString("Причина (технически): ")
		b.WriteString(strings.TrimSpace(err.Error()))
		b.WriteByte('\n')
	}

	b.WriteString("Запрошенное имя инструмента: ")
	b.WriteString(strings.TrimSpace(call.RequestedName))
	b.WriteString("\nВнутреннее имя: ")
	b.WriteString(strings.TrimSpace(call.ResolvedName))
	b.WriteByte('\n')
	pr := strings.TrimSpace(partialResult)
	errText := ""
	if err != nil {
		errText = strings.TrimSpace(err.Error())
	}

	if pr != "" && pr != errText {
		b.WriteString("\nДополнительно от сервера или среды выполнения:\n")
		b.WriteString(pr)
		b.WriteByte('\n')
	}

	return truncateToolResult(strings.TrimSpace(b.String()))
}

func toolLoopSkippedToolMessage(skipped, failed executableToolCall) string {
	s := fmt.Sprintf(
		"Статус: вызов не выполнялся - цепочка прервана из-за ошибки при инструменте %q.\n"+
			"Твоя задача: при необходимости кратко сообщи пользователю, что часть инструментов не была вызвана.\n\n"+
			"Пропущенный инструмент (как запросил пользователь/модель): %q",
		strings.TrimSpace(failed.RequestedName),
		strings.TrimSpace(skipped.RequestedName),
	)

	return truncateToolResult(s)
}

func (c *ChatUseCase) toolProgressDisplayName(ctx context.Context, sessionID int64, normalizedName string, rawToolName string) string {
	n := normalizeToolName(normalizedName)
	if sid, orig, ok := mcpclient.ParseToolAlias(n); ok {
		label := fmt.Sprintf("MCP #%d", sid)
		if c.mcpServerRepo != nil {
			var uid int
			if sess, err := c.sessionRepo.GetById(ctx, sessionID); err == nil && sess != nil {
				uid = sess.UserId
			}

			if srv, err := c.mcpServerRepo.GetByIDAccessible(ctx, sid, uid); err == nil && srv != nil {
				if nm := strings.TrimSpace(srv.Name); nm != "" {
					label = "MCP  " + nm
				}
			}
		}

		orig = strings.TrimSpace(orig)
		if orig != "" {
			return label + "  " + orig
		}

		return label
	}

	st := strings.TrimSpace(rawToolName)
	if st != "" {
		return st
	}

	return n
}

func webSearchToolDefinition() domain.Tool {
	return domain.Tool{
		Name:           "web_search",
		Description:    "Поиск актуальной информации в интернете: новости, цены, погода, документация, свежие факты. Используй, когда ответ зависит от текущих данных или знания модели могут быть устаревшими. Передай короткий точный запрос на языке пользователя или на английском.",
		ParametersJSON: `{"type":"object","properties":{"query":{"type":"string","description":"Поисковый запрос: несколько ключевых слов или короткая фраза."}},"required":["query"]}`,
	}
}

func toolCallNamesForLog(calls []executableToolCall) string {
	if len(calls) == 0 {
		return ""
	}

	parts := make([]string, 0, len(calls))
	for i, c := range calls {
		if i >= 12 {
			parts = append(parts, fmt.Sprintf("…+%d", len(calls)-12))
			break
		}

		rn := strings.TrimSpace(c.RequestedName)
		if rn == "" {
			rn = "(?)"
		}

		parts = append(parts, rn)
	}

	return strings.Join(parts, ",")
}

func (c *ChatUseCase) executeDeclaredTool(ctx context.Context, userID int, sessionID int64, genParams *domain.GenerationParams, nameNorm string, params json.RawMessage) (res string, err error) {
	params = maybePruneToolArgsJSON(genParams, nameNorm, params)
	t0 := time.Now()
	logger.I("Tool: phase=start session_id=%d user_id=%d tool=%q params_bytes=%d", sessionID, userID, nameNorm, len(params))
	defer func() {
		d := time.Since(t0)
		if err != nil {
			logger.W("Tool: phase=done session_id=%d user_id=%d tool=%q duration=%s err=%v", sessionID, userID, nameNorm, d, err)
		} else {
			logger.I("Tool: phase=done session_id=%d user_id=%d tool=%q duration=%s result_bytes=%d", sessionID, userID, nameNorm, d, len(res))
		}
	}()

	switch nameNorm {
	case "apply_spreadsheet":
		return c.toolApplySpreadsheet(ctx, userID, sessionID, params)
	case "build_docx":
		return c.toolBuildDocx(ctx, userID, sessionID, params)
	case "apply_markdown_patch":
		return c.toolApplyMarkdownPatch(ctx, userID, sessionID, params)
	case "put_session_file":
		return c.toolPutSessionFile(ctx, userID, sessionID, params)
	case "web_search":
		return c.toolWebSearch(ctx, userID, sessionID, params)
	default:
		if sid, orig, ok := mcpclient.ParseToolAlias(nameNorm); ok {
			if env := toolLoopEnvFrom(ctx); env != nil && env.UserTurnHasVisionImage &&
				len(c.mcpToolsAllowlistWhenUserImage) > 0 &&
				!mcpToolAllowedWhenUserHasImage(c.mcpToolsAllowlistWhenUserImage, sid, orig) {
				logger.W("Tool: phase=mcp_blocked_user_image session_id=%d server_id=%d mcp_tool=%q allowlist_entries=%d", sessionID, sid, orig, len(c.mcpToolsAllowlistWhenUserImage))
				return "", fmt.Errorf("инструмент %q недоступен при вложённом изображении; разрешите в конфиге mcp.tools_allowlist_when_user_image_attached как %d:%s", orig, sid, orig)
			}
			logger.I("Tool: phase=dispatch_mcp session_id=%d alias=%q server_id=%d mcp_tool=%q params_bytes=%d", sessionID, nameNorm, sid, orig, len(params))
			res, err = c.toolMCP(ctx, sessionID, sid, orig, params)
			return res, err
		}
		err = fmt.Errorf("инструмент %q пока не реализован на сервере", nameNorm)
		return "", err
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
	logger.I("Tool: apply_spreadsheet phase=start session_id=%d user_id=%d params_bytes=%d", sessionID, userID, len(params))
	var m map[string]json.RawMessage
	if err := json.Unmarshal(params, &m); err != nil {
		return "", fmt.Errorf("параметры apply_spreadsheet: %w", err)
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
		_, data, _, err := c.loadSessionAttachmentForSend(ctx, userID, sessionID, fid)
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

func (c *ChatUseCase) toolWebSearch(ctx context.Context, _ int, sessionID int64, params json.RawMessage) (string, error) {
	settings, err := c.sessionSettingsRepo.GetBySessionID(ctx, sessionID)
	if err != nil {
		return "", err
	}
	searcher := c.webSearcherFor(ctx, settings)
	if searcher == nil {
		return "", fmt.Errorf("веб-поиск не настроен на сервере или выбранный источник недоступен")
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(params, &m); err != nil {
		return "", fmt.Errorf("параметры web_search: %w", err)
	}

	q, err := mustStringField(m, "query")
	if err != nil {
		return "", err
	}

	logger.I("Tool: web_search phase=search session_id=%d query_len=%d", sessionID, len(q))

	out, err := runFnWithContext(ctx, func() (string, error) {
		return searcher.Search(ctx, q)
	})
	if err != nil {
		return "", err
	}

	return out, nil
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
	logger.I("Tool: build_docx phase=start session_id=%d user_id=%d params_bytes=%d", sessionID, userID, len(params))
	var m map[string]json.RawMessage
	if err := json.Unmarshal(params, &m); err != nil {
		return "", fmt.Errorf("параметры build_docx: %w", err)
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
	logger.I("Tool: put_session_file phase=start session_id=%d user_id=%d params_bytes=%d", sessionID, userID, len(params))
	var m map[string]json.RawMessage
	if err := json.Unmarshal(params, &m); err != nil {
		return "", fmt.Errorf("параметры put_session_file: %w", err)
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
	logger.I("Tool: apply_markdown_patch phase=start session_id=%d user_id=%d params_bytes=%d", sessionID, userID, len(params))
	var m map[string]json.RawMessage
	if err := json.Unmarshal(params, &m); err != nil {
		return "", fmt.Errorf("параметры apply_markdown_patch: %w", err)
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

		_, data, _, err := c.loadSessionAttachmentForSend(ctx, userID, sessionID, fid)
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
