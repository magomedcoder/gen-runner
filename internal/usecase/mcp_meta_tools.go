package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/mcpclient"
)

func genMcpBuiltinTools() []domain.Tool {
	return []domain.Tool{
		{
			Name:           "gen_mcp_list_resources",
			Description:    `[MCP] Список ресурсов с MCP-сервера (resources/list). Укажи server_id из настроек сессии.`,
			ParametersJSON: `{"type":"object","properties":{"server_id":{"type":"integer","description":"ID MCP-сервера"}},"required":["server_id"]}`,
		},
		{
			Name:           "gen_mcp_read_resource",
			Description:    `[MCP] Прочитать ресурс по URI (resources/read). Возвращает JSON с текстом и/или base64 blob.`,
			ParametersJSON: `{"type":"object","properties":{"server_id":{"type":"integer","description":"ID MCP-сервера"},"uri":{"type":"string","description":"URI ресурса из списка"}},"required":["server_id","uri"]}`,
		},
		{
			Name:           "gen_mcp_list_prompts",
			Description:    `[MCP] Список промптов/шаблонов с MCP-сервера (prompts/list).`,
			ParametersJSON: `{"type":"object","properties":{"server_id":{"type":"integer","description":"ID MCP-сервера"}},"required":["server_id"]}`,
		},
		{
			Name:           "gen_mcp_get_prompt",
			Description:    `[MCP] Получить развёрнутый промпт (prompts/get). arguments - необязательный объект строк для подстановки в шаблон.`,
			ParametersJSON: `{"type":"object","properties":{"server_id":{"type":"integer","description":"ID MCP-сервера"},"name":{"type":"string","description":"Имя промпта"},"arguments":{"type":"object","additionalProperties":{"type":"string"},"description":"Аргументы шаблона"}},"required":["server_id","name"]}`,
		},
	}
}

func (c *ChatUseCase) maybeInjectMCPBuiltinMetaTools(genParams *domain.GenerationParams, settings *domain.ChatSessionSettings) {
	if genParams == nil || settings == nil || !settings.MCPEnabled || len(settings.MCPServerIDs) == 0 || c.mcpServerRepo == nil {
		return
	}

	allowed := allowedToolNameSet(genParams.Tools)
	for _, t := range genMcpBuiltinTools() {
		n := normalizeToolName(t.Name)
		if _, dup := allowed[n]; dup {
			continue
		}

		allowed[n] = struct{}{}
		genParams.Tools = append(genParams.Tools, t)
	}
}

func (c *ChatUseCase) mcpServerForSession(ctx context.Context, sessionID int64, serverID int64) (*domain.MCPServer, error) {
	if c.mcpServerRepo == nil {
		return nil, fmt.Errorf("MCP недоступен")
	}

	settings, err := c.sessionSettingsRepo.GetBySessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if settings == nil || !settings.MCPEnabled {
		return nil, fmt.Errorf("MCP отключён для этой сессии")
	}

	allowed := slices.Contains(settings.MCPServerIDs, serverID)

	if !allowed {
		return nil, fmt.Errorf("MCP-сервер не выбран для сессии")
	}

	sess, err := c.sessionRepo.GetById(ctx, sessionID)
	if err != nil || sess == nil {
		return nil, fmt.Errorf("сессия не найдена")
	}

	srv, err := c.mcpServerRepo.GetByIDAccessible(ctx, serverID, sess.UserId)
	if err != nil {
		return nil, err
	}

	if srv == nil || !srv.Enabled {
		return nil, fmt.Errorf("MCP-сервер недоступен")
	}

	return srv, nil
}

func (c *ChatUseCase) toolGenMcpListResources(ctx context.Context, sessionID int64, params json.RawMessage) (string, error) {
	var body struct {
		ServerID int64 `json:"server_id"`
	}
	if err := json.Unmarshal(params, &body); err != nil {
		return "", fmt.Errorf("аргументы gen_mcp_list_resources: %w", err)
	}

	if body.ServerID <= 0 {
		return "", fmt.Errorf("некорректный server_id")
	}

	srv, err := c.mcpServerForSession(ctx, sessionID, body.ServerID)
	if err != nil {
		return "", err
	}

	var list []mcpclient.DeclaredResource
	if c.mcpToolsListCache != nil {
		list, err = c.mcpToolsListCache.ListResourcesCached(ctx, srv, mcpclient.DefaultToolsListCacheTTL)
	} else {
		list, err = mcpclient.ListResources(ctx, srv)
	}
	if err != nil {
		return "", err
	}

	total := len(list)
	if len(list) > mcpclient.MaxMetaListItems {
		list = list[:mcpclient.MaxMetaListItems]
	}

	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return "", err
	}

	s := string(b)
	if total > mcpclient.MaxMetaListItems {
		s += fmt.Sprintf("\n\n[GEN: показано %d из %d ресурсов]", len(list), total)
	}
	return mcpclient.TruncateLLMReply(s, mcpclient.MaxMetaToolReplyRunes), nil
}

func (c *ChatUseCase) toolGenMcpReadResource(ctx context.Context, sessionID int64, params json.RawMessage) (string, error) {
	var body struct {
		ServerID int64  `json:"server_id"`
		URI      string `json:"uri"`
	}
	if err := json.Unmarshal(params, &body); err != nil {
		return "", fmt.Errorf("аргументы gen_mcp_read_resource: %w", err)
	}

	if body.ServerID <= 0 || strings.TrimSpace(body.URI) == "" {
		return "", fmt.Errorf("нужны server_id и uri")
	}

	srv, err := c.mcpServerForSession(ctx, sessionID, body.ServerID)
	if err != nil {
		return "", err
	}

	return mcpclient.ReadResourceJSON(ctx, srv, body.URI, c.mcpToolsListCache)
}

func (c *ChatUseCase) toolGenMcpListPrompts(ctx context.Context, sessionID int64, params json.RawMessage) (string, error) {
	var body struct {
		ServerID int64 `json:"server_id"`
	}
	if err := json.Unmarshal(params, &body); err != nil {
		return "", fmt.Errorf("аргументы gen_mcp_list_prompts: %w", err)
	}

	if body.ServerID <= 0 {
		return "", fmt.Errorf("некорректный server_id")
	}

	srv, err := c.mcpServerForSession(ctx, sessionID, body.ServerID)
	if err != nil {
		return "", err
	}

	var list []mcpclient.DeclaredPrompt
	if c.mcpToolsListCache != nil {
		list, err = c.mcpToolsListCache.ListPromptsCached(ctx, srv, mcpclient.DefaultToolsListCacheTTL)
	} else {
		list, err = mcpclient.ListPrompts(ctx, srv)
	}
	if err != nil {
		return "", err
	}

	total := len(list)
	if len(list) > mcpclient.MaxMetaListItems {
		list = list[:mcpclient.MaxMetaListItems]
	}

	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return "", err
	}

	s := string(b)
	if total > mcpclient.MaxMetaListItems {
		s += fmt.Sprintf("\n\n[GEN: показано %d из %d промптов]", len(list), total)
	}

	return mcpclient.TruncateLLMReply(s, mcpclient.MaxMetaToolReplyRunes), nil
}

func (c *ChatUseCase) toolGenMcpGetPrompt(ctx context.Context, sessionID int64, params json.RawMessage) (string, error) {
	var body struct {
		ServerID  int64             `json:"server_id"`
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}

	if err := json.Unmarshal(params, &body); err != nil {
		return "", fmt.Errorf("аргументы gen_mcp_get_prompt: %w", err)
	}

	if body.ServerID <= 0 || strings.TrimSpace(body.Name) == "" {
		return "", fmt.Errorf("нужны server_id и name")
	}

	srv, err := c.mcpServerForSession(ctx, sessionID, body.ServerID)
	if err != nil {
		return "", err
	}

	return mcpclient.GetPromptText(ctx, srv, body.Name, body.Arguments, c.mcpToolsListCache)
}

func (c *ChatUseCase) appendMCPLLMContext(ctx context.Context, msg *domain.Message, settings *domain.ChatSessionSettings, userID int) {
	if msg == nil || settings == nil || !settings.MCPEnabled || len(settings.MCPServerIDs) == 0 || c.mcpServerRepo == nil {
		return
	}

	var b strings.Builder
	b.WriteString("[MCP] В этой сессии чата включены внешние инструменты. Разрешённые server_id (используй только их):\n")
	for _, sid := range settings.MCPServerIDs {
		if sid <= 0 {
			continue
		}
		line := fmt.Sprintf("- id=%d", sid)
		if srv, err := c.mcpServerRepo.GetByIDAccessible(ctx, sid, userID); err == nil && srv != nil {
			if srv.Enabled {
				if n := strings.TrimSpace(srv.Name); n != "" {
					line = fmt.Sprintf("- id=%d · %s", sid, n)
				}
			} else {
				line = fmt.Sprintf("- id=%d · (отключён в каталоге)", sid)
			}
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("\nИнструменты MCP с сервера перечислены в списке tools; у каждого есть описание и JSON-схема параметров — передавай аргументы строго по схеме.\n\n")
	b.WriteString("Встроенные инструменты GEN для ресурсов и шаблонов промптов (аргументы — JSON):\n")
	b.WriteString("- gen_mcp_list_resources: {\"server_id\": <id>}\n")
	b.WriteString("- gen_mcp_read_resource: {\"server_id\": <id>, \"uri\": \"<uri из ответа gen_mcp_list_resources>\"}\n")
	b.WriteString("- gen_mcp_list_prompts: {\"server_id\": <id>}\n")
	b.WriteString("- gen_mcp_get_prompt: {\"server_id\": <id>, \"name\": \"<имя из gen_mcp_list_prompts>\", \"arguments\": {}}\n")
	b.WriteString("\nПри необходимости сначала вызови list_* для нужного server_id, затем read_resource или get_prompt с данными из ответа.\n")

	msg.Content += "\n\n" + strings.TrimSpace(b.String())
}
