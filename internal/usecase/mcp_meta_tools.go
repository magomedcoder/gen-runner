package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/pkg/logger"
)

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

	b.WriteString("\nИнструменты, которые объявляет сам MCP-сервер, перечислены в общем списке tools (имена могут выглядеть как mcp_<id>_h<hex>, это нормально). Вызывай их обычными tool-вызовами по этим именам и передавай аргументы строго по JSON-схеме инструмента.\n")
	b.WriteString("Не добавляй в аргументы поле server_id: привязка к серверу уже зашита в имени инструмента.\n")
	b.WriteString("Если запрос пользователя требует данные из внешней системы (например, задачи/комментарии/статусы), сначала используй подходящий инструмент из списка tools и только после этого формируй ответ.\n")
	b.WriteString("Не утверждай, что инструмента нет или что доступ невозможен, пока не проверишь доступные tools и не попробуешь релевантный вызов.\n")

	msg.Content += "\n\n" + strings.TrimSpace(b.String())
}
