package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/mcpclient"
	"github.com/magomedcoder/gen/pkg/logger"
)

func (c *ChatUseCase) maybeInjectWebSearchTool(ctx context.Context, genParams *domain.GenerationParams, settings *domain.ChatSessionSettings) {
	if genParams == nil || settings == nil || !settings.WebSearchEnabled {
		return
	}
	if c.webSearcherFor(ctx, settings) == nil {
		return
	}
	for _, t := range genParams.Tools {
		if normalizeToolName(t.Name) == "web_search" {
			return
		}
	}
	genParams.Tools = append(genParams.Tools, webSearchToolDefinition())
}

func (c *ChatUseCase) injectWebSearchAndMCP(ctx context.Context, genParams *domain.GenerationParams, settings *domain.ChatSessionSettings, userID int, sessionID int64) {
	c.maybeInjectWebSearchTool(ctx, genParams, settings)
	c.maybeInjectMCPTools(ctx, genParams, settings, userID)
	c.maybeInjectMCPBuiltinMetaTools(ctx, genParams, settings, userID)
	logLLMVisibleToolCatalog(sessionID, userID, genParams, settings)
}

const maxLLMToolNamesListRunes = 12000

func logLLMVisibleToolCatalog(sessionID int64, userID int, genParams *domain.GenerationParams, settings *domain.ChatSessionSettings) {
	if genParams == nil {
		logger.D("LLM tool catalog: session_id=%d user_id=%d genParams=nil", sessionID, userID)
		return
	}

	n := len(genParams.Tools)
	if n == 0 {
		logger.I("LLM tool catalog: session_id=%d user_id=%d count=0 (модель без tool-calling)", sessionID, userID)
		if settings != nil {
			logger.D("LLM tool catalog: session_id=%d web_search_enabled=%t mcp_enabled=%t mcp_server_ids=%v", sessionID, settings.WebSearchEnabled, settings.MCPEnabled, settings.MCPServerIDs)
		}
		return
	}

	names := make([]string, 0, n)
	var nWebSearch, nGenMcp, nMcpAlias, nOther int
	for _, t := range genParams.Tools {
		raw := strings.TrimSpace(t.Name)
		if raw == "" {
			raw = "(без имени)"
		}

		names = append(names, raw)
		low := strings.ToLower(raw)
		switch {
		case low == "web_search":
			nWebSearch++
		case strings.HasPrefix(low, "gen_mcp_"):
			nGenMcp++
		case strings.HasPrefix(low, "mcp_"):
			nMcpAlias++
		default:
			nOther++
		}
	}

	list := strings.Join(names, ", ")
	truncated := false
	if utf8.RuneCountInString(list) > maxLLMToolNamesListRunes {
		runes := []rune(list)
		list = string(runes[:maxLLMToolNamesListRunes]) + fmt.Sprintf("…(обрезано, всего=%d)", len(names))
		truncated = true
	}

	var ws, mcp bool
	var mcpIDs []int64
	if settings != nil {
		ws = settings.WebSearchEnabled
		mcp = settings.MCPEnabled
		mcpIDs = settings.MCPServerIDs
	}

	logger.I("LLM tool catalog: session_id=%d user_id=%d count=%d web_search=%d gen_mcp_*=%d mcp_*=%d прочие=%d truncated=%t session web_search=%t mcp=%t mcp_server_ids=%v", sessionID, userID, n, nWebSearch, nGenMcp, nMcpAlias, nOther, truncated, ws, mcp, mcpIDs)
	logger.I("LLM tool catalog: session_id=%d names: %s", sessionID, list)
}

func (c *ChatUseCase) mcpEffectiveServerIDs(_ context.Context, _ int, settings *domain.ChatSessionSettings) []int64 {
	if settings == nil || !settings.MCPEnabled || c.mcpServerRepo == nil {
		return nil
	}

	explicit := settings.MCPServerIDs
	if len(explicit) == 0 {
		return nil
	}

	out := make([]int64, 0, len(explicit))
	seen := map[int64]struct{}{}
	for _, sid := range explicit {
		if sid <= 0 {
			continue
		}

		if _, ok := seen[sid]; ok {
			continue
		}

		seen[sid] = struct{}{}
		out = append(out, sid)
	}

	slices.Sort(out)

	return out
}

func (c *ChatUseCase) maybeInjectMCPTools(ctx context.Context, genParams *domain.GenerationParams, settings *domain.ChatSessionSettings, userID int) {
	if genParams == nil || settings == nil {
		return
	}
	effective := c.mcpEffectiveServerIDs(ctx, userID, settings)
	if len(effective) == 0 {
		return
	}

	logger.D("MCP inject tools: user_id=%d выбранных_серверов=%d уже_есть_tools=%d", userID, len(effective), len(genParams.Tools))
	before := len(genParams.Tools)
	allowed := allowedToolNameSet(genParams.Tools)
	for _, sid := range effective {
		if sid <= 0 {
			continue
		}

		srv, err := c.mcpServerRepo.GetByIDAccessible(ctx, sid, userID)
		if err != nil || srv == nil || !srv.Enabled {
			if err != nil {
				logger.W("MCP inject tools: сервер id=%d недоступен: %v", sid, err)
			} else if srv == nil || !srv.Enabled {
				logger.D("MCP inject tools: сервер id=%d пропуск (nil или disabled)", sid)
			}
			continue
		}

		logger.D("MCP inject tools: загрузка списка server_id=%d name=%q transport=%q", sid, strings.TrimSpace(srv.Name), strings.TrimSpace(srv.Transport))

		declared, err := c.mcpToolsListCache.ListToolsCached(ctx, srv, mcpclient.DefaultToolsListCacheTTL)
		if err != nil {
			logger.E("MCP: не удалось получить список инструментов server_id=%d name=%q transport=%q url=%q: %v. Подсказка: GEN ходит в MCP с машины, где запущен backend; url=127.0.0.1 - это loopback сервера GEN, а не ваш ПК.", sid, strings.TrimSpace(srv.Name), strings.TrimSpace(srv.Transport), strings.TrimSpace(srv.URL), err)
			continue
		}

		prefix := strings.TrimSpace(srv.Name)
		addedFromServer := 0
		for _, t := range declared {
			if strings.TrimSpace(t.Name) == "" {
				continue
			}
			alias := mcpclient.ToolAlias(sid, t.Name)
			n := normalizeToolName(alias)
			if _, dup := allowed[n]; dup {
				logger.D("MCP inject tools: server_id=%d пропуск дубликата alias=%q", sid, alias)
				continue
			}

			allowed[n] = struct{}{}
			desc := strings.TrimSpace(t.Description)
			if prefix != "" {
				desc = "[MCP " + prefix + "] " + desc
			} else {
				desc = "[MCP #" + strconv.FormatInt(sid, 10) + "] " + desc
			}

			genParams.Tools = append(genParams.Tools, domain.Tool{
				Name:           alias,
				Description:    strings.TrimSpace(desc),
				ParametersJSON: t.ParametersJSON,
			})

			addedFromServer++
			logger.D("MCP inject tools: server_id=%d alias=%q orig=%q", sid, alias, t.Name)
		}

		if addedFromServer > 0 {
			logger.I("MCP inject tools: server_id=%d name=%q в_genparams=%d объявлено_на_сервере=%d (всего tools=%d)", sid, strings.TrimSpace(srv.Name), addedFromServer, len(declared), len(genParams.Tools))
		} else {
			logger.D("MCP inject tools: server_id=%d name=%q в_genparams=0 объявлено=%d (все дубликаты или пусто)", sid, strings.TrimSpace(srv.Name), len(declared))
		}
	}
	added := len(genParams.Tools) - before
	if added > 0 {
		logger.I("MCP inject tools: итого добавлено MCP-инструментов=%d (всего tools=%d)", added, len(genParams.Tools))
	}
}

func (c *ChatUseCase) toolMCP(ctx context.Context, sessionID int64, serverID int64, mcpToolName string, params json.RawMessage) (string, error) {
	if c.mcpServerRepo == nil {
		return "", fmt.Errorf("MCP недоступен")
	}

	logger.D("MCP toolMCP: session_id=%d server_id=%d tool=%q params_json_bytes=%d", sessionID, serverID, mcpToolName, len(params))
	settings, err := c.sessionSettingsRepo.GetBySessionID(ctx, sessionID)
	if err != nil {
		return "", err
	}

	if settings == nil || !settings.MCPEnabled {
		return "", fmt.Errorf("MCP отключён для этой сессии")
	}

	sess, err := c.sessionRepo.GetById(ctx, sessionID)
	if err != nil || sess == nil {
		return "", fmt.Errorf("сессия не найдена")
	}

	effective := c.mcpEffectiveServerIDs(ctx, sess.UserId, settings)
	if !slices.Contains(effective, serverID) {
		return "", fmt.Errorf("этот MCP-сервер не выбран для сессии")
	}

	srv, err := c.mcpServerRepo.GetByIDAccessible(ctx, serverID, sess.UserId)
	if err != nil {
		return "", err
	}

	if srv == nil || !srv.Enabled {
		return "", fmt.Errorf("MCP-сервер недоступен")
	}

	toolCtx := ctx
	if env := toolLoopEnvFrom(ctx); env != nil && mcpclient.SamplingEnabled() {
		logger.D("MCP toolMCP: session_id=%d sampling=enabled model=%q", sessionID, env.ResolvedModel)
		toolCtx = mcpclient.WithSamplingRunner(ctx, &mcpclient.SamplingRunner{
			LLM:            c.llmRepo,
			SessionID:      sessionID,
			RunnerAddr:     env.RunnerAddr,
			Model:          env.ResolvedModel,
			StopSequences:  env.StopSequences,
			TimeoutSeconds: env.TimeoutSeconds,
			GenParams:      env.SamplingGen,
		})
	} else {
		logger.D("MCP toolMCP: session_id=%d sampling=disabled", sessionID)
	}

	return runFnWithContext(ctx, func() (string, error) {
		t0 := time.Now()
		out, err := mcpclient.CallTool(toolCtx, srv, mcpToolName, params, c.mcpToolsListCache)
		d := time.Since(t0)
		if err != nil {
			logger.W("MCP: вызов инструмента сессия=%d server_id=%d инструмент=%q длительность=%s ошибка=%v", sessionID, serverID, mcpToolName, d, err)
		} else {
			logger.I("MCP: вызов инструмента сессия=%d server_id=%d инструмент=%q длительность=%s ответ_bytes=%d", sessionID, serverID, mcpToolName, d, len(out))
		}

		return out, err
	})
}
