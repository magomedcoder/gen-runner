package mcpwikiserver

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magomedcoder/gen/mcp-servers/mcp-wiki/internal/wikiindex"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestStartupAutoIndexLoadsDocs(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "auto.md"), []byte("# Auto\n\nstartup index marker zz8822"), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := NewServerWithOptions(Options{DefaultDirectory: dir})
	st, ct := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	cli := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	session, err := cli.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	out, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "wiki_index_status", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || len(out.Content) != 1 {
		t.Fatalf("wiki_index_status: %#v", out)
	}
	tc, ok := out.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected content: %#v", out.Content[0])
	}
	var status struct {
		Chunks int `json:"Chunks"`
	}
	if err := json.Unmarshal([]byte(tc.Text), &status); err != nil {
		t.Fatalf("status json: %v body=%s", err, tc.Text)
	}
	if status.Chunks <= 0 {
		t.Fatalf("expected Chunks>0 after startup index, got %d body=%s", status.Chunks, tc.Text)
	}
}

func TestLastIndexReportResource(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.md"), []byte("# X\n\nhello resource marker zyx9911"), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := NewServerWithOptions(Options{
		DefaultDirectory:     dir,
		SkipStartupAutoIndex: true,
	})
	st, ct := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	cli := mcp.NewClient(&mcp.Implementation{
		Name:    "test",
		Version: "0",
	}, nil)
	session, err := cli.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	lr, err := session.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: LastIndexReportURI,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(lr.Contents) != 1 || !strings.Contains(lr.Contents[0].Text, `"indexed":false`) {
		t.Fatalf("before index: %v", lr.Contents)
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "index_wiki_folder",
		Arguments: map[string]any{
			"max_files":           20,
			"max_chunks":          0,
			"chunk_size_runes":    400,
			"chunk_overlap_runes": 40,
			"token_locale":        "",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	lr2, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: LastIndexReportURI})
	if err != nil {
		t.Fatal(err)
	}

	if len(lr2.Contents) != 1 || !strings.Contains(lr2.Contents[0].Text, `"indexed_files"`) {
		t.Fatalf("after index: %v", lr2.Contents)
	}
}

func TestResourceUpdatedAfterIndexWhenSubscribed(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "n.md"), []byte("# N\n\nnotify test body\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	updated := make(chan string, 2)
	cli := mcp.NewClient(&mcp.Implementation{
		Name:    "test",
		Version: "0",
	}, &mcp.ClientOptions{
		ResourceUpdatedHandler: func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
			if req != nil && req.Params != nil {
				updated <- req.Params.URI
			}
		},
	})

	srv := NewServerWithOptions(Options{
		DefaultDirectory: dir,
	})
	st, ct := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	session, err := cli.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	if err := session.Subscribe(ctx, &mcp.SubscribeParams{URI: LastIndexReportURI}); err != nil {
		t.Fatal(err)
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "index_wiki_folder",
		Arguments: map[string]any{
			"max_files":           20,
			"max_chunks":          0,
			"chunk_size_runes":    400,
			"chunk_overlap_runes": 40,
			"token_locale":        "",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case uri := <-updated:
		if uri != LastIndexReportURI {
			t.Fatalf("unexpected uri: %q", uri)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for resources/updated")
	}
}

func TestAnswerEmptyIndex(t *testing.T) {
	svc := &wikiService{
		retriever: wikiindex.NewLexicalTFIDF(),
		files:     map[string]indexedFileRecord{},
	}
	raw, err := svc.answerJSON(askArgs{
		Query: "что по авторизации?",
	})
	if err != nil {
		t.Fatalf("answer error: %v", err)
	}

	var resp askResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v; raw=%s", err, raw)
	}

	if !resp.Strict {
		t.Fatal("expected strict response")
	}

	if !strings.Contains(resp.Note, "Индекс не загружен") {
		t.Fatalf("unexpected note: %q", resp.Note)
	}

	if len(resp.Sources) != 0 {
		t.Fatalf("expected no sources, got %d", len(resp.Sources))
	}
}

func TestAnswerWithSources(t *testing.T) {
	svc := &wikiService{retriever: wikiindex.NewLexicalTFIDF(), files: map[string]indexedFileRecord{}}
	svc.retriever.Build("/tmp/wiki", []wikiindex.InputChunk{
		{
			FilePath:   "auth.md",
			FileName:   "auth.md",
			ChunkIndex: 0,
			Text:       "JWT токен проверяется в middleware перед запросом к API.",
			Metadata: map[string]any{
				"heading_path": "Безопасность › Авторизация",
			},
		},
	})

	raw, err := svc.answerJSON(askArgs{
		Query: "Как проверяется JWT токен?",
		TopK:  3,
	})
	if err != nil {
		t.Fatalf("answer error: %v", err)
	}

	var resp askResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v; raw=%s", err, raw)
	}

	if !resp.Strict {
		t.Fatal("expected strict response")
	}

	if resp.Answer == "" {
		t.Fatal("expected non-empty answer")
	}

	if len(resp.Sources) == 0 {
		t.Fatal("expected sources")
	}

	if resp.Sources[0].FilePath != "auth.md" {
		t.Fatalf("unexpected file path: %s", resp.Sources[0].FilePath)
	}

	if !strings.Contains(resp.ReplyStyleHint, "литературным русским") {
		t.Fatalf("expected reply_style_hint, got %q", resp.ReplyStyleHint)
	}
}

func TestGlobMatchAny(t *testing.T) {
	if !globMatchAny("docs/guide.md", []string{"docs/*.md"}) {
		t.Fatal("expected match on rel path")
	}

	if globMatchAny("tmp/x.tmp", []string{"*.md"}) {
		t.Fatal("unexpected match")
	}

	if !globMatchAny("junk.tmp", []string{"*.tmp"}) {
		t.Fatal("expected basename match")
	}

	if !globMatchAny("vendor/deep/pkg/x.md", []string{"**/pkg/*.md"}) {
		t.Fatal("expected ** glob on rel path")
	}

	if !globMatchAny("a/b/c/d.md", []string{"**/*.md"}) {
		t.Fatal("expected **/*.md")
	}

	if globMatchAny("a/b/c/d.txt", []string{"**/*.md"}) {
		t.Fatal("unexpected **/*.md for txt")
	}
}

func TestIndexFolder_ExtraPlainExtensions(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.adoc"), []byte("asciidoc token extra_plain_9921 here\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "skip.zzz"), []byte("no"), 0o600); err != nil {
		t.Fatal(err)
	}

	svc := &wikiService{
		retriever:        wikiindex.NewLexicalTFIDF(),
		files:            map[string]indexedFileRecord{},
		logger:           log.Default(),
		defaultDirectory: dir,
	}

	rep, err := svc.indexFolder(ctx, indexArgs{
		MaxFiles:             50,
		ChunkSizeRunes:       400,
		ChunkOverlapRunes:    40,
		ExtraPlainExtensions: []string{"adoc"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if rep.IndexedFiles != 1 {
		t.Fatalf("indexed files: %d (want 1, .zzz skipped)", rep.IndexedFiles)
	}

	if rep.ScannedFiles < 2 {
		t.Fatalf("scanned: %d", rep.ScannedFiles)
	}
}

func TestIndexFolder_MaxChunks(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "b.md")
	long := strings.Repeat("слово ", 800)
	if err := os.WriteFile(a, []byte("# A\n\n"+long), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(b, []byte("# B\n\n"+long), 0o600); err != nil {
		t.Fatal(err)
	}

	svc := &wikiService{
		retriever:        wikiindex.NewLexicalTFIDF(),
		files:            map[string]indexedFileRecord{},
		logger:           log.Default(),
		defaultDirectory: dir,
	}

	rep, err := svc.indexFolder(ctx, indexArgs{
		MaxChunks:      3,
		ChunkSizeRunes: 200,
	})

	if err != nil {
		t.Fatal(err)
	}

	if rep.IndexedStats.Chunks != 3 {
		t.Fatalf("expected 3 chunks after cap, got %d", rep.IndexedStats.Chunks)
	}

	if rep.MaxChunks != 3 {
		t.Fatalf("report max_chunks: %d", rep.MaxChunks)
	}

	joined := strings.Join(rep.Warnings, " ")
	if !strings.Contains(joined, "max_chunks") {
		t.Fatalf("expected max_chunks warning, got %v", rep.Warnings)
	}
}

func TestAnswerAbsentInDocumentsPhrase(t *testing.T) {
	svc := &wikiService{retriever: wikiindex.NewLexicalTFIDF(), files: map[string]indexedFileRecord{}}
	svc.retriever.Build("/tmp/wiki", []wikiindex.InputChunk{
		{
			FilePath:   "a.md",
			FileName:   "a.md",
			ChunkIndex: 0,
			Text:       "совсем другая тема про котиков",
		},
	})

	raw, err := svc.answerJSON(askArgs{
		Query:    "квантовая гравитация и струны",
		TopK:     3,
		MinScore: 0.95,
	})

	if err != nil {
		t.Fatal(err)
	}

	var resp askResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}

	if resp.Note != noteAbsentInDocuments {
		t.Fatalf("note: %q", resp.Note)
	}
}

func TestAnswerMarkdown(t *testing.T) {
	svc := &wikiService{retriever: wikiindex.NewLexicalTFIDF(), files: map[string]indexedFileRecord{}}
	svc.retriever.Build("/tmp/wiki", []wikiindex.InputChunk{
		{
			FilePath:   "ops.md",
			FileName:   "ops.md",
			ChunkIndex: 2,
			Text:       "Сервис деплоится через GitOps pipeline после merge в main.",
		},
	})

	md, err := svc.answerMarkdown(askArgs{
		Query: "Как деплоится сервис?",
		TopK:  2,
	})

	if err != nil {
		t.Fatalf("answer markdown error: %v", err)
	}

	if !strings.Contains(md, "Источники:") {
		t.Fatalf("expected sources section, got: %s", md)
	}

	if !strings.Contains(md, "ops.md") {
		t.Fatalf("expected source file in markdown, got: %s", md)
	}
}

func TestAskArgsFromToolArguments(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  map[string]any
		want string
	}{
		{"query", map[string]any{"query": "  x  "}, "x"},
		{"Query", map[string]any{"Query": "Pascal"}, "Pascal"},
		{"nested_arguments_map", map[string]any{"arguments": map[string]any{"question": "nested"}}, "nested"},
		{"nested_arguments_json_string", map[string]any{"arguments": `{"text":"from json"}`}, "from json"},
		{"function_wrapper", map[string]any{"function": map[string]any{"arguments": `{"content":"wrap"}`}}, "wrap"},
		{"lone_string_key", map[string]any{"custom_user_field": "only value"}, "only value"},
		{"query_nested_object", map[string]any{"query": map[string]any{"text": "deep"}}, "deep"},
		{"tool_calls", map[string]any{"tool_calls": []any{map[string]any{"function": map[string]any{"arguments": `{"query":"tc"}`}}}}, "tc"},
		{"runner_raw_fallback", map[string]any{"_raw": `{"query":"from _raw"}`}, "from _raw"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := askArgsFromToolArguments(tc.raw).Query
			if got != tc.want {
				t.Fatalf("Query: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestAskArgsFromJSONRaw(t *testing.T) {
	t.Parallel()
	if g := askArgsFromJSONRaw([]byte(`"plain root question"`)).Query; g != "plain root question" {
		t.Fatalf("root string: %q", g)
	}

	inner, err := json.Marshal(map[string]string{"query": "double encoded"})
	if err != nil {
		t.Fatal(err)
	}

	wrapped, err := json.Marshal(string(inner))
	if err != nil {
		t.Fatal(err)
	}

	if g := askArgsFromJSONRaw(wrapped).Query; g != "double encoded" {
		t.Fatalf("double-encoded: %q", g)
	}
}

func TestWikiModelPromptsRegistered(t *testing.T) {
	ctx := context.Background()
	srv := NewServerWithOptions(Options{})
	st, ct := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	cli := mcp.NewClient(&mcp.Implementation{
		Name:    "test",
		Version: "0",
	}, nil)
	session, err := cli.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	list, err := session.ListPrompts(ctx, &mcp.ListPromptsParams{})
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, p := range list.Prompts {
		names = append(names, p.Name)
	}

	if len(names) < 1 {
		t.Fatalf("expected at least one prompt, got %v", names)
	}

	found := slices.Contains(names, "wiki_prompts_full")
	if !found {
		t.Fatalf("missing wiki_prompts_full in %v", names)
	}

	got, err := session.GetPrompt(ctx, &mcp.GetPromptParams{
		Name: "wiki_prompts_full",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(got.Messages) != 1 {
		t.Fatalf("messages: %d", len(got.Messages))
	}

	tc, ok := got.Messages[0].Content.(*mcp.TextContent)
	if !ok || !strings.Contains(tc.Text, "MCP Wiki") {
		t.Fatalf("unexpected prompt body: %#v", got.Messages[0].Content)
	}

	rr, err := session.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: WikiModelPromptsURI,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rr.Contents) != 1 {
		t.Fatalf("resource contents: %d", len(rr.Contents))
	}
	if rr.Contents[0].Text == "" || !strings.Contains(rr.Contents[0].Text, "MCP Wiki") {
		t.Fatalf("unexpected resource body: %#v", rr.Contents[0])
	}

	tres, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wiki_model_prompts",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tres == nil || len(tres.Content) != 1 {
		t.Fatalf("wiki_model_prompts content: %#v", tres)
	}
	tt, ok := tres.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(tt.Text, "MCP Wiki") {
		t.Fatalf("unexpected wiki_model_prompts body: %#v", tres.Content[0])
	}
}
