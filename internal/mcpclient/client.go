package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DeclaredTool struct {
	Name           string
	Description    string
	ParametersJSON string
}

func parseStringSliceJSON(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}

	return out, nil
}

func parseStringMapJSON(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]string{}, nil
	}

	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}

	if out == nil {
		return map[string]string{}, nil
	}

	return out, nil
}

func mergeEnv(base []string, extra map[string]string) []string {
	if len(extra) == 0 {
		return base
	}

	out := append([]string(nil), base...)
	for k, v := range extra {
		if strings.TrimSpace(k) == "" {
			continue
		}

		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}

	return out
}

type headerRoundTripper struct {
	next http.RoundTripper
	h    map[string]string
}

func (t *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range t.h {
		if strings.TrimSpace(k) != "" && v != "" {
			req.Header.Set(k, v)
		}
	}

	return t.next.RoundTrip(req)
}

func httpClientWithHeaders(headers map[string]string) *http.Client {
	if len(headers) == 0 {
		return nil
	}

	base := http.DefaultTransport
	if base == nil {
		base = &http.Transport{}
	}

	return &http.Client{
		Transport: &headerRoundTripper{next: base, h: headers},
		Timeout:   0,
	}
}

func transportFor(ctx context.Context, srv *domain.MCPServer) (mcp.Transport, error) {
	if srv == nil {
		return nil, errors.New("nil mcp server")
	}

	tr := strings.ToLower(strings.TrimSpace(srv.Transport))
	headers, err := parseStringMapJSON(srv.HeadersJSON)
	if err != nil {
		return nil, fmt.Errorf("headers_json: %w", err)
	}

	hc := httpClientWithHeaders(headers)

	switch tr {
	case "stdio":
		cmdPath := strings.TrimSpace(srv.Command)
		if cmdPath == "" {
			return nil, errors.New("stdio: пустая команда")
		}

		if err := validateStdioPolicy(srv); err != nil {
			return nil, err
		}

		args, err := parseStringSliceJSON(srv.ArgsJSON)
		if err != nil {
			return nil, fmt.Errorf("args_json: %w", err)
		}

		envExtra, err := parseStringMapJSON(srv.EnvJSON)
		if err != nil {
			return nil, fmt.Errorf("env_json: %w", err)
		}

		cmd := exec.CommandContext(ctx, cmdPath, args...)
		cmd.Env = mergeEnv(os.Environ(), envExtra)
		return &mcp.CommandTransport{Command: cmd}, nil

	case "sse":
		raw := strings.TrimSpace(srv.URL)
		if raw == "" {
			return nil, errors.New("sse: пустой url")
		}

		if err := validateHTTPMCPURL("sse", raw); err != nil {
			return nil, err
		}

		t := &mcp.SSEClientTransport{Endpoint: raw}
		if hc != nil {
			t.HTTPClient = hc
		}

		return t, nil

	case "streamable":
		raw := strings.TrimSpace(srv.URL)
		if raw == "" {
			return nil, errors.New("streamable: пустой url")
		}

		if err := validateHTTPMCPURL("streamable", raw); err != nil {
			return nil, err
		}

		t := &mcp.StreamableClientTransport{
			Endpoint:             raw,
			DisableStandaloneSSE: true,
		}

		if hc != nil {
			t.HTTPClient = hc
		}
		return t, nil

	default:
		return nil, fmt.Errorf("неизвестный transport %q", tr)
	}
}

func timeoutFor(srv *domain.MCPServer) time.Duration {
	sec := int64(srv.TimeoutSeconds)
	if sec <= 0 {
		sec = 120
	}

	if sec > 600 {
		sec = 600
	}

	return time.Duration(sec) * time.Second
}

func buildMCPClientOptions(ctx context.Context, srv *domain.MCPServer, notify *ToolsListCache) *mcp.ClientOptions {
	caps := &mcp.ClientCapabilities{}
	if r := rootsForSession(); len(r) > 0 {
		caps.RootsV2 = &mcp.RootCapabilities{ListChanged: true}
	}

	opts := &mcp.ClientOptions{
		Capabilities: caps,
	}
	if s := samplingClientOptions(ctx); s != nil {
		opts.CreateMessageHandler = s.CreateMessageHandler
		opts.CreateMessageWithToolsHandler = s.CreateMessageWithToolsHandler
	}

	if n := notifyForListChangedHandlers(notify); n != nil && srv != nil && srv.ID > 0 {
		sid := srv.ID
		opts.ToolListChangedHandler = func(context.Context, *mcp.ToolListChangedRequest) {
			n.InvalidateServerTools(sid)
		}

		opts.ResourceListChangedHandler = func(context.Context, *mcp.ResourceListChangedRequest) {
			n.InvalidateServerResources(sid)
		}

		opts.PromptListChangedHandler = func(context.Context, *mcp.PromptListChangedRequest) {
			n.InvalidateServerPrompts(sid)
		}
	}

	if LogServerMessages() && srv != nil {
		opts.LoggingMessageHandler = loggingMessageHandlerForServer(strings.TrimSpace(srv.Name), srv.ID)
	}

	return opts
}

func withEphemeralSession(ctx context.Context, srv *domain.MCPServer, notify *ToolsListCache, fn func(context.Context, *mcp.ClientSession) error) error {
	transport, err := transportFor(ctx, srv)
	if err != nil {
		return err
	}

	tctx, cancel := context.WithTimeout(ctx, timeoutFor(srv))
	defer cancel()

	opts := buildMCPClientOptions(ctx, srv, notify)
	cli := mcp.NewClient(&mcp.Implementation{Name: "gen", Version: "1.0.0"}, opts)
	if r := rootsForSession(); len(r) > 0 {
		cli.AddRoots(r...)
	}

	session, err := cli.Connect(tctx, transport, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = session.Close()
	}()

	return fn(tctx, session)
}

func withSession(ctx context.Context, srv *domain.MCPServer, notify *ToolsListCache, fn func(context.Context, *mcp.ClientSession) error) error {
	if useHTTPSessionPool(ctx, srv) {
		return globalHTTPPool.run(ctx, srv, notify, fn)
	}

	return withEphemeralSession(ctx, srv, notify, fn)
}

var callToolSessionRunner = withSession

var callToolInvoker = func(ctx context.Context, session *mcp.ClientSession, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	return session.CallTool(ctx, params)
}

func inputSchemaToParametersJSON(schema any) string {
	if schema == nil {
		return `{"type":"object","properties":{}}`
	}

	b, err := json.Marshal(schema)
	if err != nil || len(b) == 0 || string(b) == "null" {
		return `{"type":"object","properties":{}}`
	}

	return string(b)
}

func ListTools(ctx context.Context, srv *domain.MCPServer) ([]DeclaredTool, error) {
	return listTools(ctx, srv, nil)
}

func listTools(ctx context.Context, srv *domain.MCPServer, notify *ToolsListCache) ([]DeclaredTool, error) {
	var out []DeclaredTool
	err := withSession(ctx, srv, notify, func(cctx context.Context, session *mcp.ClientSession) error {
		res, err := session.ListTools(cctx, &mcp.ListToolsParams{})
		if err != nil {
			return err
		}

		for _, t := range res.Tools {
			if t == nil || strings.TrimSpace(t.Name) == "" {
				continue
			}

			out = append(out, DeclaredTool{
				Name:           t.Name,
				Description:    strings.TrimSpace(t.Description),
				ParametersJSON: inputSchemaToParametersJSON(t.InputSchema),
			})
		}

		return nil
	})

	recordListTools(err)
	return out, err
}

func CallTool(ctx context.Context, srv *domain.MCPServer, mcpToolName string, arguments json.RawMessage, notify ...*ToolsListCache) (string, error) {
	started := time.Now()
	defer func() {
		recordCallToolDuration(time.Since(started))
	}()

	serverID := int64(0)
	if srv != nil {
		serverID = srv.ID
	}

	if strings.TrimSpace(mcpToolName) == "" {
		recordCallToolTransportErr()
		recordCallToolServer(serverID, "transport_err")
		return "", errors.New("пустое имя инструмента MCP")
	}

	var nc *ToolsListCache
	if len(notify) > 0 {
		nc = notify[0]
	}

	var result string
	var callErr error
	attempts := 1
	if callToolAllowsTransportRetry(mcpToolName) {
		attempts = 2
	}

	var err error
	for attempt := 0; attempt < attempts; attempt++ {
		err = callToolSessionRunner(ctx, srv, nc, func(cctx context.Context, session *mcp.ClientSession) error {
			var args any
			if len(arguments) > 0 {
				if err := json.Unmarshal(arguments, &args); err != nil {
					return fmt.Errorf("аргументы инструмента: %w", err)
				}
			}

			if args == nil {
				args = map[string]any{}
			}

			res, err := callToolInvoker(cctx, session, &mcp.CallToolParams{
				Name:      mcpToolName,
				Arguments: args,
			})
			if err != nil {
				return err
			}

			result = CallToolResultString(res)
			if res != nil && res.IsError {
				callErr = errors.New(strings.TrimSpace(result))
			}

			return nil
		})
		if err == nil {
			break
		}
		if attempt < attempts-1 && isRetryableTransportError(ctx, err) {
			recordCallToolRetry()
			continue
		}
	}

	if err != nil {
		recordCallToolTransportErr()
		recordCallToolServer(serverID, "transport_err")
		return "", err
	}

	if callErr != nil {
		recordCallToolMCPError()
		recordCallToolServer(serverID, "mcp_error")
		return result, callErr
	}

	recordCallToolOK()
	recordCallToolServer(serverID, "ok")
	return result, nil
}
