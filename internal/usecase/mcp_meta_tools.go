package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/mcpclient"
	"github.com/magomedcoder/gen/pkg/logger"
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

func (c *ChatUseCase) maybeInjectMCPBuiltinMetaTools(ctx context.Context, genParams *domain.GenerationParams, settings *domain.ChatSessionSettings, userID int) {
	if genParams == nil || settings == nil || !settings.MCPEnabled || c.mcpServerRepo == nil {
		return
	}
	effective := c.mcpEffectiveServerIDs(ctx, userID, settings)
	if len(effective) == 0 {
		return
	}

	logger.D("MCP meta tools: встраивание gen_mcp_* (серверов в сессии=%d)", len(effective))
	allowed := allowedToolNameSet(genParams.Tools)
	added := 0
	for _, t := range genMcpBuiltinTools() {
		n := normalizeToolName(t.Name)
		if _, dup := allowed[n]; dup {
			continue
		}

		allowed[n] = struct{}{}
		genParams.Tools = append(genParams.Tools, t)
		added++
	}
	if added > 0 {
		logger.I("MCP meta tools: добавлено встроенных инструментов=%d (всего tools=%d)", added, len(genParams.Tools))
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

	sess, err := c.sessionRepo.GetById(ctx, sessionID)
	if err != nil || sess == nil {
		return nil, fmt.Errorf("сессия не найдена")
	}

	effective := c.mcpEffectiveServerIDs(ctx, sess.UserId, settings)
	if !slices.Contains(effective, serverID) {
		return nil, fmt.Errorf("MCP-сервер не выбран для сессии")
	}

	srv, err := c.mcpServerRepo.GetByIDAccessible(ctx, serverID, sess.UserId)
	if err != nil {
		return nil, err
	}

	if srv == nil || !srv.Enabled {
		return nil, fmt.Errorf("MCP-сервер недоступен")
	}

	logger.D("MCP mcpServerForSession: session_id=%d server_id=%d name=%q transport=%q",
		sessionID, serverID, strings.TrimSpace(srv.Name), strings.TrimSpace(srv.Transport))
	return srv, nil
}

func (c *ChatUseCase) toolGenMcpListResources(ctx context.Context, sessionID int64, params json.RawMessage) (string, error) {
	logger.D("MCP tool gen_mcp_list_resources: session_id=%d params_bytes=%d", sessionID, len(params))
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
	out := mcpclient.TruncateLLMReply(s, mcpclient.MaxMetaToolReplyRunes)
	logger.D("MCP tool gen_mcp_list_resources: session_id=%d server_id=%d items=%d reply_runes≈%d",
		sessionID, body.ServerID, total, utf8.RuneCountInString(out))
	return out, nil
}

func (c *ChatUseCase) toolGenMcpReadResource(ctx context.Context, sessionID int64, params json.RawMessage) (string, error) {
	logger.D("MCP tool gen_mcp_read_resource: session_id=%d params_bytes=%d", sessionID, len(params))
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

	s, err := mcpclient.ReadResourceJSON(ctx, srv, body.URI, c.mcpToolsListCache)
	if err != nil {
		return "", err
	}
	logger.D("MCP tool gen_mcp_read_resource: session_id=%d server_id=%d uri_len=%d reply_runes≈%d",
		sessionID, body.ServerID, len(body.URI), utf8.RuneCountInString(s))
	return s, nil
}

func (c *ChatUseCase) toolGenMcpListPrompts(ctx context.Context, sessionID int64, params json.RawMessage) (string, error) {
	logger.D("MCP tool gen_mcp_list_prompts: session_id=%d params_bytes=%d", sessionID, len(params))
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

	out := mcpclient.TruncateLLMReply(s, mcpclient.MaxMetaToolReplyRunes)
	logger.D("MCP tool gen_mcp_list_prompts: session_id=%d server_id=%d prompts=%d reply_runes≈%d",
		sessionID, body.ServerID, total, utf8.RuneCountInString(out))
	return out, nil
}

func (c *ChatUseCase) toolGenMcpGetPrompt(ctx context.Context, sessionID int64, params json.RawMessage) (string, error) {
	logger.D("MCP tool gen_mcp_get_prompt: session_id=%d params_bytes=%d", sessionID, len(params))
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

	s, err := mcpclient.GetPromptText(ctx, srv, body.Name, body.Arguments, c.mcpToolsListCache)
	if err != nil {
		return "", err
	}
	logger.D("MCP tool gen_mcp_get_prompt: session_id=%d server_id=%d name=%q reply_runes≈%d",
		sessionID, body.ServerID, body.Name, utf8.RuneCountInString(s))
	return s, nil
}

func (c *ChatUseCase) appendMCPLLMContext(ctx context.Context, msg *domain.Message, settings *domain.ChatSessionSettings, userID int) {
	if msg == nil || settings == nil || !settings.MCPEnabled || c.mcpServerRepo == nil {
		return
	}
	effective := c.mcpEffectiveServerIDs(ctx, userID, settings)
	if len(effective) == 0 {
		return
	}

	logger.D("MCP appendMCPLLMContext: user_id=%d разрешённых_server_id=%d", userID, len(effective))
	var b strings.Builder
	b.WriteString("[MCP] В этой сессии чата включены внешние инструменты. Разрешённые server_id (используй только их):\n")
	for _, sid := range effective {
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
	b.WriteString("\nИнструменты MCP с сервера перечислены в списке tools; у каждого есть описание и JSON-схема параметров - передавай аргументы строго по схеме.\n\n")
	b.WriteString("Встроенные инструменты GEN для ресурсов и шаблонов промптов (аргументы - JSON):\n")
	b.WriteString("- gen_mcp_list_resources: {\"server_id\": <id>}\n")
	b.WriteString("- gen_mcp_read_resource: {\"server_id\": <id>, \"uri\": \"<uri из ответа gen_mcp_list_resources>\"}\n")
	b.WriteString("- gen_mcp_list_prompts: {\"server_id\": <id>}\n")
	b.WriteString("- gen_mcp_get_prompt: {\"server_id\": <id>, \"name\": \"<имя из gen_mcp_list_prompts>\", \"arguments\": {}}\n")
	b.WriteString("\nПри необходимости сначала вызови list_* для нужного server_id, затем read_resource или get_prompt с данными из ответа.\n")

	msg.Content += "\n\n" + strings.TrimSpace(b.String())
}
