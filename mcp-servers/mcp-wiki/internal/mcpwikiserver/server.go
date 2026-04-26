package mcpwikiserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"maps"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/magomedcoder/gen/mcp-servers/mcp-wiki/internal/wikiindex"
	"github.com/magomedcoder/gen/pkg/document"
	"github.com/magomedcoder/gen/pkg/mcpsafe"
	"github.com/magomedcoder/gen/pkg/rag"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultChunkSizeRunes     = 1200
	defaultChunkOverlapRunes  = 200
	defaultTopK               = 5
	defaultMinScore           = 0.08
	maxFileSizeBytes          = 12 * 1024 * 1024
	noteAbsentInDocuments     = "Информация отсутствует в предоставленных документах."
	noteIndexEmpty            = "Индекс не загружен. Сначала вызовите index_wiki_folder для загрузки документов."
	replyStyleHintForConsumer = "Сформулируй ответ пользователю связным литературным русским языком только на основе полей answer и sources; не добавляй факты вне процитированных фрагментов."
)

var (
	errStopMaxChunksWalk = errors.New("wiki: max_chunks walk limit")
	errAskWikiEmptyQuery = errors.New(`ask_wiki / ask_wiki_markdown: в arguments нужна непустая строка вопроса по wiki. Укажите query (или question, text, prompt, content, ...) на верхнем уровне или внутри вложенного объекта arguments/params. Пример: {"query":"как настроить деплой"} или {"arguments":"{\"query\":\"...\"}"}.`)
)

const LastIndexReportURI = "wiki://mcp-wiki/last_index_report"

type wikiService struct {
	retriever        wikiindex.Retriever
	mu               sync.RWMutex
	sourceDir        string
	defaultDirectory string
	files            map[string]indexedFileRecord
	logger           *log.Logger

	muLastReport sync.RWMutex
	lastReport   []byte
}

type indexedFileRecord struct {
	Hash   string
	Chunks []wikiindex.InputChunk
}

type indexArgs struct {
	Recursive            *bool    `json:"recursive,omitempty" jsonschema:"Рекурсивный обход поддиректорий (по умолчанию true)"`
	IncludeHidden        *bool    `json:"include_hidden,omitempty" jsonschema:"Индексировать скрытые файлы/папки (.name). По умолчанию false"`
	Incremental          *bool    `json:"incremental,omitempty" jsonschema:"Инкрементальная переиндексация (по умолчанию false)"`
	MaxFiles             int      `json:"max_files,omitempty" jsonschema:"Лимит количества файлов для индексации (0 = без лимита)"`
	MaxChunks            int      `json:"max_chunks,omitempty" jsonschema:"Жёсткий лимит числа чанков в итоговом индексе (0 = без лимита); лишнее отбрасывается после обхода в порядке сортировки путей"`
	TokenLocale          string   `json:"token_locale,omitempty" jsonschema:"Стоп-слова и стемминг Snowball: ru | en | пусто (стоп-слова и стемминг выкл)"`
	IncludeGlobs         []string `json:"include_globs,omitempty" jsonschema:"Если не пусто - индексировать только пути, совпадающие с любым glob-шаблоном (относительный путь или basename; синтаксис doublestar, в т.ч. **)"`
	ExcludeGlobs         []string `json:"exclude_globs,omitempty" jsonschema:"Пропускать файлы, совпадающие с любым glob-шаблоном (doublestar)"`
	ChunkSizeRunes       int      `json:"chunk_size_runes,omitempty" jsonschema:"Размер чанка в рунах (по умолчанию 1200)"`
	ChunkOverlapRunes    int      `json:"chunk_overlap_runes,omitempty" jsonschema:"Перекрытие чанков в рунах (по умолчанию 200)"`
	ExtraPlainExtensions []string `json:"extra_plain_extensions,omitempty" jsonschema:"Список расширений (.ext или ext), индексируемых как обычный UTF-8 текст вне встроенного whitelist"`
}

type askArgs struct {
	Query             string
	TopK              int
	MinScore          float64
	MinMatchedTerms   int
	MinQueryTermRatio float64
}

var askWikiQuerySynonyms = []string{
	"query",
	"question",
	"text",
	"prompt",
	"search",
	"q",
	"input",
	"message",
	"content",
	"body",
	"keyword",
	"keywords",
	"user_query",
	"userQuery",
	"search_query",
}

func firstNonEmptyTrimmed(ss ...string) string {
	for _, s := range ss {
		if t := strings.TrimSpace(s); t != "" {
			return t
		}
	}
	return ""
}

func stringFromAny(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return strings.TrimSpace(t.String())
	case float64:
		if t == math.Trunc(t) && t >= math.MinInt32 && t <= math.MaxInt32 {
			return strconv.FormatInt(int64(t), 10)
		}
		return strings.TrimSpace(strconv.FormatFloat(t, 'g', -1, 64))
	case int:
		return strconv.Itoa(t)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case bool:
		return ""
	default:
		return ""
	}
}

func stringFromAnyDeep(v any) string {
	if s := stringFromAny(v); s != "" {
		return s
	}

	t, ok := v.(map[string]any)
	if !ok {
		return ""
	}

	for _, sub := range []string{"text", "value", "content", "message", "query", "input", "body", "prompt", "question"} {
		for k, vv := range t {
			if strings.EqualFold(strings.TrimSpace(k), sub) {
				if s := stringFromAny(vv); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func isAskWikiNumericArgKey(k string) bool {
	switch strings.ToLower(strings.TrimSpace(k)) {
	case "top_k", "min_score", "min_matched_terms", "min_query_term_ratio":
		return true
	default:
		return false
	}
}

func flattenAskWikiArgMaps(m map[string]any, depth int) []map[string]any {
	if m == nil || depth > 8 {
		return nil
	}

	out := []map[string]any{m}
	if arr, ok := m["tool_calls"].([]any); ok {
		for _, el := range arr {
			if obj, ok := el.(map[string]any); ok {
				out = append(out, flattenAskWikiArgMaps(obj, depth+1)...)
			}
		}
	}

	for _, key := range []string{"arguments", "params", "payload", "kwargs", "tool_input", "function"} {
		v, ok := m[key]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case map[string]any:
			out = append(out, flattenAskWikiArgMaps(t, depth+1)...)
		case string:
			s := strings.TrimSpace(t)
			if len(s) > 0 && s[0] == '{' {
				var inner map[string]any
				if json.Unmarshal([]byte(s), &inner) == nil && len(inner) > 0 {
					out = append(out, flattenAskWikiArgMaps(inner, depth+1)...)
				}
			}
		}
	}

	return out
}

func queryFromRunnerRawKey(v any) string {
	s := strings.TrimSpace(stringFromAny(v))
	if s == "" {
		return ""
	}

	if q := strings.TrimSpace(askArgsFromJSONRaw([]byte(s)).Query); q != "" {
		return q
	}

	return s
}

func pickQueryFromAskWikiLayers(layers []map[string]any) string {
	for _, layer := range layers {
		if v, ok := layer["_raw"]; ok {
			if q := queryFromRunnerRawKey(v); q != "" {
				return q
			}
		}
	}

	for _, layer := range layers {
		for _, want := range askWikiQuerySynonyms {
			for k, v := range layer {
				if strings.EqualFold(strings.TrimSpace(k), want) {
					s := stringFromAnyDeep(v)
					if s == "" {
						continue
					}

					if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
						if inner := strings.TrimSpace(askArgsFromJSONRaw([]byte(s)).Query); inner != "" {
							return inner
						}
						continue
					}

					return s
				}
			}
		}
	}

	for _, layer := range layers {
		if s := loneNonNumericStringValue(layer); s != "" {
			return s
		}
	}

	return ""
}

func loneNonNumericStringValue(m map[string]any) string {
	var found string
	n := 0
	for k, v := range m {
		if isAskWikiNumericArgKey(k) {
			continue
		}

		s := stringFromAny(v)
		if s == "" {
			s = stringFromAnyDeep(v)
		}

		if s == "" {
			continue
		}

		if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
			if inner := strings.TrimSpace(askArgsFromJSONRaw([]byte(s)).Query); inner != "" {
				s = inner
			} else {
				continue
			}
		}
		n++
		found = s
	}

	if n == 1 {
		return found
	}

	return ""
}

func intFromAny(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int32:
		return int(t), true
	case int64:
		return int(t), true
	case float64:
		if t == math.Trunc(t) && t >= float64(math.MinInt) && t <= float64(math.MaxInt) {
			return int(t), true
		}
	case json.Number:
		i, err := strconv.Atoi(strings.TrimSpace(t.String()))
		if err == nil {
			return i, true
		}
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(t))
		if err == nil {
			return i, true
		}
	}
	return 0, false
}

func floatFromAny(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := strconv.ParseFloat(strings.TrimSpace(t.String()), 64)
		if err == nil {
			return f, true
		}
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err == nil {
			return f, true
		}
	}

	return 0, false
}

func pickIntFromLayers(layers []map[string]any, jsonKeys ...string) int {
	for _, layer := range layers {
		for k, v := range layer {
			for _, want := range jsonKeys {
				if strings.EqualFold(strings.TrimSpace(k), want) {
					if n, ok := intFromAny(v); ok {
						return n
					}
				}
			}
		}
	}

	return 0
}

func pickFloatFromLayers(layers []map[string]any, jsonKeys ...string) float64 {
	for _, layer := range layers {
		for k, v := range layer {
			for _, want := range jsonKeys {
				if strings.EqualFold(strings.TrimSpace(k), want) {
					if f, ok := floatFromAny(v); ok {
						return f
					}
				}
			}
		}
	}

	return 0
}

func askArgsFromToolArguments(raw map[string]any) askArgs {
	if raw == nil {
		raw = map[string]any{}
	}

	layers := flattenAskWikiArgMaps(raw, 0)
	q := pickQueryFromAskWikiLayers(layers)
	return askArgs{
		Query:             q,
		TopK:              pickIntFromLayers(layers, "top_k", "topK"),
		MinScore:          pickFloatFromLayers(layers, "min_score", "minScore"),
		MinMatchedTerms:   pickIntFromLayers(layers, "min_matched_terms", "minMatchedTerms"),
		MinQueryTermRatio: pickFloatFromLayers(layers, "min_query_term_ratio", "minQueryTermRatio"),
	}
}

func rawToolArguments(req *mcp.CallToolRequest) []byte {
	if req == nil || req.Params == nil {
		return nil
	}

	return req.Params.Arguments
}

func askArgsFromJSONRaw(raw []byte) askArgs {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return askArgs{}
	}

	switch s[0] {
	case '"':
		var str string
		if err := json.Unmarshal([]byte(s), &str); err != nil {
			return askArgs{}
		}
		str = strings.TrimSpace(str)
		if str == "" {
			return askArgs{}
		}

		if str[0] == '{' || str[0] == '[' {
			return askArgsFromJSONRaw([]byte(str))
		}

		return askArgs{Query: str}
	case '[':
		var arr []json.RawMessage
		if err := json.Unmarshal([]byte(s), &arr); err != nil {
			return askArgs{}
		}
		for _, el := range arr {
			a := askArgsFromJSONRaw(el)
			if strings.TrimSpace(a.Query) != "" {
				return a
			}
		}
		return askArgs{}
	case '{':
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			return askArgs{}
		}
		return askArgsFromToolArguments(m)
	default:
		return askArgs{}
	}
}

func mergeAskArgsWireDecoded(a, b askArgs) askArgs {
	out := b
	out.Query = firstNonEmptyTrimmed(a.Query, b.Query)
	if out.TopK == 0 {
		out.TopK = a.TopK
	}

	if out.MinScore == 0 {
		out.MinScore = a.MinScore
	}

	if out.MinMatchedTerms == 0 {
		out.MinMatchedTerms = a.MinMatchedTerms
	}

	if out.MinQueryTermRatio == 0 {
		out.MinQueryTermRatio = a.MinQueryTermRatio
	}

	return out
}

func askArgsFromCallToolRequest(req *mcp.CallToolRequest, decoded map[string]any) askArgs {
	wire := askArgsFromJSONRaw(rawToolArguments(req))
	dec := askArgsFromToolArguments(decoded)
	return mergeAskArgsWireDecoded(wire, dec)
}

func truncateBytesForLog(b []byte, max int) string {
	if max <= 0 || len(b) <= max {
		return string(b)
	}

	return string(b[:max]) + "...(truncated)"
}

type indexReport struct {
	Directory    string          `json:"directory"`
	Mode         string          `json:"mode"`
	ScannedFiles int             `json:"scanned_files"`
	IndexedFiles int             `json:"indexed_files"`
	Unchanged    int             `json:"unchanged_files"`
	Removed      int             `json:"removed_files"`
	ActiveFiles  int             `json:"active_files"`
	SkippedFiles int             `json:"skipped_files"`
	IndexedStats wikiindex.Stats `json:"indexed_stats"`
	Warnings     []string        `json:"warnings,omitempty"`
	MaxChunks    int             `json:"max_chunks,omitempty"`
}

type askResponse struct {
	Query          string      `json:"query"`
	Strict         bool        `json:"strict"`
	Answer         string      `json:"answer,omitempty"`
	Note           string      `json:"note,omitempty"`
	ReplyStyleHint string      `json:"reply_style_hint,omitempty"`
	Sources        []askSource `json:"sources"`
}

type askSource struct {
	Ref         int      `json:"ref"`
	FilePath    string   `json:"file_path"`
	ChunkIndex  int      `json:"chunk_index"`
	Score       float64  `json:"score"`
	HeadingPath string   `json:"heading_path,omitempty"`
	Matched     []string `json:"matched_terms,omitempty"`
	Snippet     string   `json:"snippet"`
}

type Options struct {
	DefaultDirectory     string
	Retriever            wikiindex.Retriever
	SkipStartupAutoIndex bool
}

func NewServer() *mcp.Server {
	return NewServerWithOptions(Options{})
}

func wikiMCPServerOptions() *mcp.ServerOptions {
	return &mcp.ServerOptions{
		Instructions: "При подключении клиент получает полный перечень tools этого сервера (MCP capabilities). " +
			"При запуске бинарника с флагом -wiki-dir сервер сам выполняет полную индексацию: рекурсивный обход корня и всех подпапок, чтение поддерживаемых документов, чанки и TF–IDF; повторный index_wiki_folder нужен после изменений файлов на диске. " +
			"Конечный пользователь формулирует вопросы обычным языком; ассистент отвечает строго по полям ask_wiki / ask_wiki_markdown (answer, sources, note), без додумывания фактов и путей. Если в проиндексированных документах нет оснований - так и сообщить. " +
			"Краткий текст подсказок для модели: tool wiki_model_prompts (пустые аргументы), либо prompt wiki_prompts_full, либо resource " + WikiModelPromptsURI + ". Правка в репозитории: " + WikiModelPromptsSource + ". " +
			"wiki_index_status - проверка chunks/source_dir. ask_wiki: непустой query (или синонимы). Не утверждать «нашёл в файле» без реального вызова ask_wiki с этой фразой в query. " +
			"Клиент может показывать инструменты под алиасами - ориентируйтесь по описанию.",
		SubscribeHandler: func(context.Context, *mcp.SubscribeRequest) error {
			return nil
		},
		UnsubscribeHandler: func(context.Context, *mcp.UnsubscribeRequest) error {
			return nil
		},
	}
}

func NewServerWithOptions(opts Options) *mcp.Server {
	r := opts.Retriever
	if r == nil {
		r = wikiindex.NewLexicalTFIDF()
	}
	svc := &wikiService{
		retriever:        r,
		defaultDirectory: strings.TrimSpace(opts.DefaultDirectory),
		files:            map[string]indexedFileRecord{},
		logger:           log.Default(),
	}

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "wiki-rag",
		Title:   "MCP Wiki RAG",
		Version: "0.1.0",
	}, wikiMCPServerOptions())

	registerWikiModelPrompts(srv)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wiki_model_prompts",
		Description: "Возвращает краткий Markdown подсказок для модели (tools, автоиндексация с -wiki-dir, ask_wiki, строгость). Тот же текст, что prompt wiki_prompts_full и resource " + WikiModelPromptsURI + ". Для клиентов без prompts/resources; аргументы не нужны (пустой объект {}).",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return mcpsafe.SafeToolInvoke("mcp-wiki", "wiki_model_prompts", func() (*mcp.CallToolResult, any, error) {
			body := wikiModelPromptsBody()
			if body == "" {
				return nil, nil, fmt.Errorf("wiki_model_prompts: пустой текст")
			}
			svc.logf("tool=wiki_model_prompts event=serve runes=%d", utf8.RuneCountInString(body))
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: body},
				},
			}, nil, nil
		})
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "index_wiki_folder",
		Description: "Полная или инкрементальная переиндексация корня из -wiki-dir: рекурсивный обход всех подпапок, поддерживаемые документы, чанки, TF–IDF. При старте бинарника с -wiki-dir полная индексация уже выполняется автоматически; вызывайте этот инструмент после правок на диске (или incremental=true для дозагрузки). Без индекса ask_wiki не видит текст файлов.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args indexArgs) (*mcp.CallToolResult, any, error) {
		return mcpsafe.SafeToolInvoke("mcp-wiki", "index_wiki_folder", func() (*mcp.CallToolResult, any, error) {
			svc.logf("tool=index_wiki_folder event=start wiki_dir=%q incremental=%t recursive=%t include_hidden=%t max_files=%d max_chunks=%d chunk_size=%d chunk_overlap=%d extra_plain_ext=%d",
				strings.TrimSpace(svc.defaultDirectory),
				boolArg(args.Incremental, false),
				boolArg(args.Recursive, true),
				boolArg(args.IncludeHidden, false),
				args.MaxFiles,
				args.MaxChunks,
				args.ChunkSizeRunes,
				args.ChunkOverlapRunes,
				len(args.ExtraPlainExtensions),
			)

			report, err := svc.indexFolder(ctx, args)
			if err != nil {
				svc.logf("tool=index_wiki_folder event=error err=%v", err)
				return nil, nil, err
			}

			svc.logf("tool=index_wiki_folder event=done mode=%s scanned=%d indexed=%d unchanged=%d removed=%d active=%d skipped=%d warnings=%d chunks=%d",
				report.Mode,
				report.ScannedFiles,
				report.IndexedFiles,
				report.Unchanged,
				report.Removed,
				report.ActiveFiles,
				report.SkippedFiles,
				len(report.Warnings),
				report.IndexedStats.Chunks,
			)

			payload, err := json.Marshal(report)
			if err != nil {
				svc.logf("tool=index_wiki_folder event=marshal_error err=%v", err)
				return nil, nil, err
			}

			svc.setLastIndexReportJSON(payload)

			_ = srv.ResourceUpdated(ctx, &mcp.ResourceUpdatedNotificationParams{URI: LastIndexReportURI})

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: string(payload),
					},
				},
			}, nil, nil
		})
	})

	srv.AddResource(&mcp.Resource{
		URI:         LastIndexReportURI,
		Name:        "last_index_report",
		Description: "JSON отчёт последнего успешного вызова index_wiki_folder (поле directory - фактический проиндексированный корень, indexed_stats, warnings, ...). До первой индексации - {\"indexed\":false}.",
		MIMEType:    "application/json",
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		if req.Params.URI != LastIndexReportURI {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}

		body := svc.snapshotLastIndexReportJSON()
		if len(body) == 0 {
			body = []byte(`{"indexed":false}`)
		}

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      LastIndexReportURI,
				MIMEType: "application/json",
				Text:     string(body),
			}},
		}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ask_wiki",
		Description: "Шаг 2: поиск только по уже проиндексированному содержимому (после index_wiki_folder). arguments: JSON с непустым текстом вопроса в поле query (или question, text, prompt, search, q, input, message). Пример: {\"query\":\"как оформить отпуск\"}. Возвращает JSON: answer, sources (file_path, snippet, score), note при отсутствии совпадений. Модель не имеет прямого доступа к файлам wiki - нельзя «найти уникальную фразу» без реального вызова с текстом запроса; не подставляйте выдуманный ответ. Если пользователь просит проверить видимость wiki: сначала wiki_index_status (chunks>0), затем ask_wiki с якорной строкой. reply_style_hint: перефразировать связным русским только по источникам.",
	}, func(_ context.Context, req *mcp.CallToolRequest, raw map[string]any) (*mcp.CallToolResult, any, error) {
		return mcpsafe.SafeToolInvoke("mcp-wiki", "ask_wiki", func() (*mcp.CallToolResult, any, error) {
			args := askArgsFromCallToolRequest(req, raw)
			svc.logf("tool=ask_wiki event=start query_len=%d top_k=%d min_score=%.4f",
				utf8.RuneCountInString(strings.TrimSpace(args.Query)),
				args.TopK,
				args.MinScore,
			)

			answer, err := svc.answerJSON(args)
			if err != nil {
				if errors.Is(err, errAskWikiEmptyQuery) {
					svc.logf("tool=ask_wiki event=empty_query arguments_wire=%s", truncateBytesForLog(rawToolArguments(req), 900))
				}
				svc.logf("tool=ask_wiki event=error err=%v", err)
				return nil, nil, err
			}
			svc.logf("tool=ask_wiki event=done")

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: answer},
				},
			}, nil, nil
		})
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ask_wiki_markdown",
		Description: "То же по смыслу, что ask_wiki, но ответ в Markdown с блоком источников. arguments: непустой query или синонимы (question, text, prompt, ...), как у ask_wiki. Требует index_wiki_folder. Не отвечать по памяти без вызова инструмента.",
	}, func(_ context.Context, req *mcp.CallToolRequest, raw map[string]any) (*mcp.CallToolResult, any, error) {
		return mcpsafe.SafeToolInvoke("mcp-wiki", "ask_wiki_markdown", func() (*mcp.CallToolResult, any, error) {
			args := askArgsFromCallToolRequest(req, raw)
			svc.logf("tool=ask_wiki_markdown event=start query_len=%d top_k=%d min_score=%.4f",
				utf8.RuneCountInString(strings.TrimSpace(args.Query)),
				args.TopK,
				args.MinScore,
			)

			answer, err := svc.answerMarkdown(args)
			if err != nil {
				if errors.Is(err, errAskWikiEmptyQuery) {
					svc.logf("tool=ask_wiki_markdown event=empty_query arguments_wire=%s", truncateBytesForLog(rawToolArguments(req), 900))
				}
				svc.logf("tool=ask_wiki_markdown event=error err=%v", err)
				return nil, nil, err
			}
			svc.logf("tool=ask_wiki_markdown event=done")

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: answer},
				},
			}, nil, nil
		})
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "wiki_index_status",
		Description: "Быстрая проверка индекса: files, chunks, source_dir. При старте с -wiki-dir индекс обычно уже построен; chunks=0 означает сбой автоиндексации или пустой каталог - тогда index_wiki_folder вручную.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return mcpsafe.SafeToolInvoke("mcp-wiki", "wiki_index_status", func() (*mcp.CallToolResult, any, error) {
			stats := svc.retriever.Stats()
			svc.logf("tool=wiki_index_status event=read files=%d chunks=%d terms=%d source_dir=%q",
				stats.Files,
				stats.Chunks,
				stats.UniqueTerms,
				stats.SourceDir,
			)
			payload, err := json.Marshal(stats)
			if err != nil {
				svc.logf("tool=wiki_index_status event=marshal_error err=%v", err)
				return nil, nil, err
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: string(payload),
					},
				},
			}, nil, nil
		})
	})

	if strings.TrimSpace(svc.defaultDirectory) != "" && !opts.SkipStartupAutoIndex {
		svc.runStartupAutoIndex(context.Background(), srv)
	}

	return srv
}

func (s *wikiService) runStartupAutoIndex(ctx context.Context, srv *mcp.Server) {
	if strings.TrimSpace(s.defaultDirectory) == "" {
		return
	}

	const maxDuration = 24 * time.Hour
	cctx, cancel := context.WithTimeout(ctx, maxDuration)
	defer cancel()

	s.logf("component=startup event=auto_index_begin directory=%q", strings.TrimSpace(s.defaultDirectory))
	report, err := s.indexFolder(cctx, indexArgs{})
	if err != nil {
		s.logf("component=startup event=auto_index_fail err=%v", err)
		return
	}

	payload, err := json.Marshal(report)
	if err != nil {
		s.logf("component=startup event=auto_index_marshal err=%v", err)
		return
	}

	s.setLastIndexReportJSON(payload)
	if srv != nil {
		_ = srv.ResourceUpdated(cctx, &mcp.ResourceUpdatedNotificationParams{URI: LastIndexReportURI})
	}

	s.logf("component=startup event=auto_index_done mode=%s scanned=%d indexed=%d active=%d chunks=%d",
		report.Mode,
		report.ScannedFiles,
		report.IndexedFiles,
		report.ActiveFiles,
		report.IndexedStats.Chunks,
	)
}

func (s *wikiService) indexFolder(ctx context.Context, args indexArgs) (indexReport, error) {
	startedAt := time.Now()
	root := strings.TrimSpace(s.defaultDirectory)
	if root == "" {
		return indexReport{}, errors.New("каталог wiki не задан: укажите флаг -wiki-dir при запуске бинарника MCP-сервера")
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return indexReport{}, fmt.Errorf("resolve wiki directory: %w", err)
	}

	st, err := os.Stat(absRoot)
	if err != nil {
		return indexReport{}, fmt.Errorf("wiki directory недоступна: %w", err)
	}

	if !st.IsDir() {
		return indexReport{}, errors.New("-wiki-dir должен указывать на папку")
	}

	recursive := boolArg(args.Recursive, true)
	includeHidden := boolArg(args.IncludeHidden, false)
	incremental := boolArg(args.Incremental, false)

	chunkSize := args.ChunkSizeRunes
	if chunkSize <= 0 {
		chunkSize = defaultChunkSizeRunes
	}

	chunkOverlap := max(args.ChunkOverlapRunes, 0)
	if chunkOverlap == 0 {
		chunkOverlap = defaultChunkOverlapRunes
	}

	report := indexReport{
		Directory: absRoot,
		Mode:      "full",
	}
	if incremental {
		report.Mode = "incremental"
	}

	stopWalk := errors.New("max files reached")
	fileLimit := args.MaxFiles
	warnings := make([]string, 0, 8)
	seenSupported := map[string]struct{}{}
	updates := map[string]indexedFileRecord{}

	s.mu.RLock()
	prevSourceDir := s.sourceDir
	prevFiles := copyIndexedFiles(s.files)
	s.mu.RUnlock()

	if incremental && prevSourceDir != "" && prevSourceDir != absRoot {
		warnings = append(warnings, "incremental отключен: папка отличается от предыдущей индексации, выполнен full rebuild")
		incremental = false
		report.Mode = "full"
		prevFiles = map[string]indexedFileRecord{}
	}

	s.logf("component=indexer event=walk_begin directory=%q mode=%s recursive=%t include_hidden=%t", absRoot, report.Mode, recursive, includeHidden)

	walkErr := filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			warnings = append(warnings, fmt.Sprintf("skip %s: %v", path, walkErr))
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if path == absRoot {
			return nil
		}

		name := d.Name()
		if d.IsDir() {
			if !recursive {
				return filepath.SkipDir
			}
			if !includeHidden && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if !includeHidden && strings.HasPrefix(name, ".") {
			report.SkippedFiles++
			return nil
		}

		report.ScannedFiles++
		if fileLimit > 0 && report.ScannedFiles > fileLimit {
			return stopWalk
		}

		if !document.IsSupportedOrPlainExtra(name, args.ExtraPlainExtensions) {
			report.SkippedFiles++
			return nil
		}

		relPath, err := filepath.Rel(absRoot, path)
		if err != nil {
			relPath = path
		}

		relPath = filepath.ToSlash(relPath)
		seenSupported[relPath] = struct{}{}
		s.logf("component=indexer event=doc_seen path=%q", relPath)

		if len(args.IncludeGlobs) > 0 && !globMatchAny(relPath, args.IncludeGlobs) {
			report.SkippedFiles++
			s.logf("component=indexer event=doc_skip path=%q reason=glob_not_included", relPath)
			return nil
		}
		if len(args.ExcludeGlobs) > 0 && globMatchAny(relPath, args.ExcludeGlobs) {
			report.SkippedFiles++
			s.logf("component=indexer event=doc_skip path=%q reason=glob_excluded", relPath)
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		content, err := os.ReadFile(path)
		if err != nil {
			report.SkippedFiles++
			warnings = append(warnings, fmt.Sprintf("read %s: %v", path, err))
			s.logf("component=indexer event=doc_skip path=%q reason=read err=%v", relPath, err)
			return nil
		}

		hash := digest(content)

		if incremental {
			if old, ok := prevFiles[relPath]; ok && old.Hash == hash {
				report.Unchanged++
				s.logf("component=indexer event=doc_unchanged path=%q", relPath)
				return nil
			}
		}

		if len(content) > maxFileSizeBytes {
			report.SkippedFiles++
			warnings = append(warnings, fmt.Sprintf("skip %s: file too large (%d bytes)", path, len(content)))
			s.logf("component=indexer event=doc_skip path=%q reason=file_too_large bytes=%d", relPath, len(content))
			return nil
		}

		text, pdfBounds, err := document.ExtractTextForRAGOrPlainExtra(ctx, name, content, args.ExtraPlainExtensions)
		if err != nil {
			report.SkippedFiles++
			warnings = append(warnings, fmt.Sprintf("extract %s: %v", path, err))
			s.logf("component=indexer event=doc_skip path=%q reason=extract err=%v", relPath, err)
			return nil
		}

		text = document.NormalizeExtractedText(text)
		if strings.TrimSpace(text) == "" {
			report.SkippedFiles++
			s.logf("component=indexer event=doc_skip path=%q reason=empty_text_after_extract", relPath)
			return nil
		}

		chunks := rag.SplitTextWithPDFPageBounds(relPath, text, rag.SplitOptions{
			ChunkSizeRunes:    chunkSize,
			ChunkOverlapRunes: chunkOverlap,
		}, pdfBounds)

		if len(chunks) == 0 {
			report.SkippedFiles++
			s.logf("component=indexer event=doc_skip path=%q reason=no_chunks_after_split", relPath)
			return nil
		}

		nextFileChunks := make([]wikiindex.InputChunk, 0, len(chunks))
		for _, ch := range chunks {
			nextFileChunks = append(nextFileChunks, wikiindex.InputChunk{
				FilePath:   relPath,
				FileName:   name,
				ChunkIndex: ch.Index,
				Text:       ch.Text,
				Metadata:   ch.Metadata,
			})
		}

		maxC := args.MaxChunks
		if maxC > 0 {
			cur := countChunksInUpdates(updates)
			if cur >= maxC {
				warnings = append(warnings, fmt.Sprintf("обход остановлен: достигнут max_chunks=%d (файл %s не добавлен)", maxC, relPath))
				return errStopMaxChunksWalk
			}
			if cur+len(nextFileChunks) > maxC {
				allow := maxC - cur
				nextFileChunks = nextFileChunks[:allow]
				warnings = append(warnings, fmt.Sprintf("файл %s: обрезано до %d чанков по max_chunks=%d", relPath, allow, maxC))
			}
			if len(nextFileChunks) == 0 {
				report.SkippedFiles++
				s.logf("component=indexer event=doc_skip path=%q reason=max_chunks_truncated_empty", relPath)
				return errStopMaxChunksWalk
			}
		}

		report.IndexedFiles++
		s.logf("component=indexer event=doc_indexed path=%q chunks=%d", relPath, len(nextFileChunks))
		updates[relPath] = indexedFileRecord{
			Hash:   hash,
			Chunks: nextFileChunks,
		}

		if maxC > 0 && countChunksInUpdates(updates) >= maxC {
			warnings = append(warnings, fmt.Sprintf("обход завершён: достигнут max_chunks=%d", maxC))
			return errStopMaxChunksWalk
		}

		return nil
	})

	if errors.Is(walkErr, stopWalk) && fileLimit > 0 {
		s.logf("component=indexer event=walk_stopped reason=max_files limit=%d scanned=%d", fileLimit, report.ScannedFiles)
	}

	if errors.Is(walkErr, errStopMaxChunksWalk) {
		s.logf("component=indexer event=walk_stopped reason=max_chunks limit=%d", args.MaxChunks)
	}

	if walkErr != nil && !errors.Is(walkErr, stopWalk) && !errors.Is(walkErr, errStopMaxChunksWalk) {
		return indexReport{}, walkErr
	}

	s.mu.Lock()
	if !incremental {
		s.files = map[string]indexedFileRecord{}
		s.sourceDir = absRoot
	} else {
		if s.sourceDir == "" {
			s.sourceDir = absRoot
		}

		for rel := range s.files {
			if _, ok := seenSupported[rel]; !ok {
				delete(s.files, rel)
				report.Removed++
			}
		}
	}

	maps.Copy(s.files, updates)

	inputChunks := make([]wikiindex.InputChunk, 0, 1024)
	relPaths := make([]string, 0, len(s.files))
	for rel := range s.files {
		relPaths = append(relPaths, rel)
	}

	sort.Strings(relPaths)

	for _, rel := range relPaths {
		inputChunks = append(inputChunks, s.files[rel].Chunks...)
	}

	maxChunks := args.MaxChunks
	if maxChunks > 0 && len(inputChunks) > maxChunks {
		before := len(inputChunks)
		inputChunks = inputChunks[:maxChunks]
		warnings = append(warnings, fmt.Sprintf("индекс обрезан по max_chunks: было %d чанков, оставлено %d", before, maxChunks))

		nextFiles := map[string]indexedFileRecord{}
		for _, ch := range inputChunks {
			rec := nextFiles[ch.FilePath]
			if rec.Hash == "" {
				if old, ok := s.files[ch.FilePath]; ok {
					rec.Hash = old.Hash
				}
			}
			rec.Chunks = append(rec.Chunks, ch)
			nextFiles[ch.FilePath] = rec
		}
		s.files = nextFiles
	}

	report.ActiveFiles = len(s.files)
	report.MaxChunks = maxChunks
	s.mu.Unlock()

	report.IndexedStats = s.retriever.Build(absRoot, inputChunks, wikiindex.BuildOptions{
		TokenLocale: args.TokenLocale,
	})

	sort.Strings(warnings)
	if len(warnings) > 20 {
		warnings = warnings[:20]
		warnings = append(warnings, "... warnings truncated ...")
	}
	report.Warnings = warnings
	s.logf("component=indexer event=build_complete directory=%q mode=%s duration_ms=%d files=%d chunks=%d terms=%d",
		absRoot,
		report.Mode,
		time.Since(startedAt).Milliseconds(),
		report.IndexedStats.Files,
		report.IndexedStats.Chunks,
		report.IndexedStats.UniqueTerms,
	)

	return report, nil
}

func (s *wikiService) answerJSON(args askArgs) (string, error) {
	resp, err := s.buildAnswer(args)
	if err != nil {
		return "", err
	}

	return mustJSON(resp), nil
}

func (s *wikiService) answerMarkdown(args askArgs) (string, error) {
	resp, err := s.buildAnswer(args)
	if err != nil {
		return "", err
	}

	return responseToMarkdown(resp), nil
}

func (s *wikiService) buildAnswer(args askArgs) (askResponse, error) {
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return askResponse{}, errAskWikiEmptyQuery
	}

	topK := args.TopK
	if topK <= 0 {
		topK = defaultTopK
	}

	minScore := args.MinScore
	if minScore <= 0 {
		minScore = defaultMinScore
	}

	stats := s.retriever.Stats()
	if stats.Chunks == 0 {
		s.logf("component=qa event=empty_index query_len=%d", utf8.RuneCountInString(query))
		return askResponse{
			Query:   query,
			Strict:  true,
			Note:    noteIndexEmpty,
			Sources: []askSource{},
		}, nil
	}

	found := s.retriever.Search(query, topK*2)
	if len(found) == 0 {
		s.logf("component=qa event=no_matches query_len=%d top_k=%d min_score=%.4f", utf8.RuneCountInString(query), topK, minScore)
		return askResponse{
			Query:   query,
			Strict:  true,
			Note:    noteAbsentInDocuments,
			Sources: []askSource{},
		}, nil
	}

	filtered := make([]wikiindex.SearchResult, 0, topK)
	for _, r := range found {
		if r.Score < minScore {
			continue
		}
		filtered = append(filtered, r)
		if len(filtered) == topK {
			break
		}
	}

	if len(filtered) == 0 {
		s.logf("component=qa event=below_threshold query_len=%d candidates=%d min_score=%.4f", utf8.RuneCountInString(query), len(found), minScore)
		return askResponse{
			Query:   query,
			Strict:  true,
			Note:    noteAbsentInDocuments,
			Sources: []askSource{},
		}, nil
	}

	minTerms := args.MinMatchedTerms
	if minTerms <= 0 {
		minTerms = 1
	}

	termFiltered := make([]wikiindex.SearchResult, 0, len(filtered))
	for _, r := range filtered {
		if len(r.MatchedTerms) < minTerms {
			continue
		}
		termFiltered = append(termFiltered, r)
	}

	if len(termFiltered) == 0 {
		s.logf("component=qa event=min_matched_terms query_len=%d min_matched_terms=%d candidates_after_score=%d", utf8.RuneCountInString(query), minTerms, len(filtered))
		return askResponse{
			Query:   query,
			Strict:  true,
			Note:    noteAbsentInDocuments,
			Sources: []askSource{},
		}, nil
	}

	ratio := args.MinQueryTermRatio
	uq := len(wikiindex.TokenizeForSearch(query, s.retriever.TokenLocale()))
	ratioFiltered := termFiltered
	if ratio > 0 && ratio <= 1 && uq > 0 {
		need := max(int(math.Ceil(float64(uq)*ratio)), 1)
		next := make([]wikiindex.SearchResult, 0, len(termFiltered))
		for _, r := range termFiltered {
			if len(r.MatchedTerms) < need {
				continue
			}
			next = append(next, r)
		}
		ratioFiltered = next
	}

	if len(ratioFiltered) == 0 {
		s.logf("component=qa event=min_query_term_ratio query_len=%d ratio=%.3f query_terms=%d candidates=%d", utf8.RuneCountInString(query), ratio, uq, len(termFiltered))
		return askResponse{
			Query:   query,
			Strict:  true,
			Note:    noteAbsentInDocuments,
			Sources: []askSource{},
		}, nil
	}

	terms := s.queryTermsFromIndex(query)
	summary := make([]string, 0, len(ratioFiltered))
	sources := make([]askSource, 0, len(ratioFiltered))
	for i, res := range ratioFiltered {
		snippet := bestSentence(res.Chunk.Text, terms)
		if snippet == "" {
			snippet = truncateRunes(strings.TrimSpace(res.Chunk.Text), 220)
		}

		summary = append(summary, fmt.Sprintf("%s [%d]", snippet, i+1))
		src := askSource{
			Ref:        i + 1,
			FilePath:   res.Chunk.FilePath,
			ChunkIndex: res.Chunk.ChunkIndex,
			Score:      res.Score,
			Matched:    res.MatchedTerms,
			Snippet:    snippet,
		}

		if heading, ok := res.Chunk.Metadata["heading_path"].(string); ok {
			src.HeadingPath = strings.TrimSpace(heading)
		}

		sources = append(sources, src)
	}

	resp := askResponse{
		Query:          query,
		Strict:         true,
		Answer:         strings.Join(summary, " "),
		ReplyStyleHint: replyStyleHintForConsumer,
		Sources:        sources,
	}
	s.logf("component=qa event=answered query_len=%d selected=%d top_k=%d min_score=%.4f min_matched_terms=%d min_query_term_ratio=%.3f",
		utf8.RuneCountInString(query),
		len(resp.Sources),
		topK,
		minScore,
		minTerms,
		ratio,
	)

	return resp, nil
}

func (s *wikiService) queryTermsFromIndex(query string) map[string]struct{} {
	parts := wikiindex.TokenizeForSearch(query, s.retriever.TokenLocale())
	out := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		out[p] = struct{}{}
	}
	return out
}

func globMatchAny(relPath string, globs []string) bool {
	rel := filepath.ToSlash(relPath)
	base := filepath.ToSlash(filepath.Base(rel))
	for _, g := range globs {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		pat := filepath.ToSlash(g)
		if ok, _ := doublestar.Match(pat, rel); ok {
			return true
		}
		if ok, _ := doublestar.Match(pat, base); ok {
			return true
		}
	}
	return false
}

func countChunksInUpdates(updates map[string]indexedFileRecord) int {
	n := 0
	for _, rec := range updates {
		n += len(rec.Chunks)
	}
	return n
}

func bestSentence(text string, terms map[string]struct{}) string {
	sentences := splitSentences(text)
	if len(sentences) == 0 {
		return ""
	}

	best := ""
	bestScore := -1
	for _, s := range sentences {
		score := sentenceMatchScore(s, terms)
		if score > bestScore {
			bestScore = score
			best = s
		}
	}

	if bestScore <= 0 {
		return ""
	}

	return truncateRunes(best, 220)
}

func splitSentences(s string) []string {
	raw := strings.FieldsFunc(s, func(r rune) bool {
		return r == '.' || r == '!' || r == '?' || r == '\n'
	})

	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}

		out = append(out, item)
	}

	return out
}

func sentenceMatchScore(sentence string, terms map[string]struct{}) int {
	if len(terms) == 0 {
		return 0
	}

	words := strings.FieldsFunc(strings.ToLower(sentence), func(r rune) bool {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return false
		}
		return true
	})

	matched := map[string]struct{}{}
	for _, w := range words {
		if _, ok := terms[w]; ok {
			matched[w] = struct{}{}
		}
	}

	return len(matched)
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}

	if utf8.RuneCountInString(s) <= max {
		return s
	}

	runes := []rune(s)

	return strings.TrimSpace(string(runes[:max])) + "..."
}

func boolArg(v *bool, def bool) bool {
	if v == nil {
		return def
	}

	return *v
}

func (s *wikiService) setLastIndexReportJSON(payload []byte) {
	cp := append([]byte(nil), payload...)
	s.muLastReport.Lock()
	s.lastReport = cp
	s.muLastReport.Unlock()
}

func (s *wikiService) snapshotLastIndexReportJSON() []byte {
	s.muLastReport.RLock()
	defer s.muLastReport.RUnlock()
	if len(s.lastReport) == 0 {
		return nil
	}

	return append([]byte(nil), s.lastReport...)
}

func (s *wikiService) logf(format string, args ...any) {
	if s != nil && s.logger != nil {
		s.logger.Printf(format, args...)
		return
	}

	log.Printf(format, args...)
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func mustJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return `{"strict":true,"note":"json encoding error","sources":[]}`
	}

	return string(raw)
}

func copyIndexedFiles(in map[string]indexedFileRecord) map[string]indexedFileRecord {
	if len(in) == 0 {
		return map[string]indexedFileRecord{}
	}

	out := make(map[string]indexedFileRecord, len(in))
	for path, rec := range in {
		clonedChunks := make([]wikiindex.InputChunk, 0, len(rec.Chunks))
		for _, ch := range rec.Chunks {
			clonedChunks = append(clonedChunks, wikiindex.InputChunk{
				FilePath:   ch.FilePath,
				FileName:   ch.FileName,
				ChunkIndex: ch.ChunkIndex,
				Text:       ch.Text,
				Metadata:   cloneMeta(ch.Metadata),
			})
		}

		out[path] = indexedFileRecord{
			Hash:   rec.Hash,
			Chunks: clonedChunks,
		}
	}

	return out
}

func cloneMeta(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}

	out := make(map[string]any, len(in))
	maps.Copy(out, in)

	return out
}

func responseToMarkdown(resp askResponse) string {
	var b strings.Builder
	b.WriteString("Ответ сформирован строго по проиндексированным wiki-документам.\n\n")
	if strings.TrimSpace(resp.Note) != "" {
		b.WriteString("Не удалось дать подтверждённый ответ: ")
		b.WriteString(resp.Note)
		return strings.TrimSpace(b.String())
	}

	if strings.TrimSpace(resp.ReplyStyleHint) != "" {
		b.WriteString("Подсказка для формулировки (только по источникам ниже): ")
		b.WriteString(resp.ReplyStyleHint)
		b.WriteString("\n\n")
	}

	b.WriteString("Ответ:\n")
	b.WriteString(resp.Answer)
	b.WriteString("\n\nИсточники:\n")
	for _, src := range resp.Sources {
		b.WriteString("- [")
		b.WriteString(fmt.Sprintf("%d", src.Ref))
		b.WriteString("] ")
		b.WriteString(src.FilePath)
		b.WriteString(" (chunk=")
		b.WriteString(fmt.Sprintf("%d", src.ChunkIndex))
		b.WriteString(", score=")
		b.WriteString(fmt.Sprintf("%.3f", src.Score))
		b.WriteString(")")
		if src.HeadingPath != "" {
			b.WriteString(" heading=")
			b.WriteString(src.HeadingPath)
		}

		b.WriteString("\n  > ")
		b.WriteString(src.Snippet)
		b.WriteByte('\n')
	}

	return strings.TrimSpace(b.String())
}
