package domain

import "errors"

var (
	ErrNotFound                     = errors.New("не найдено")
	ErrUnauthorized                 = errors.New("недостаточно прав")
	ErrNoRunners                    = errors.New("нет активных раннеров")
	ErrRunnerChatModelNotConfigured = errors.New("у активного раннера не задана модель для чата")
	ErrRegenerateToolsNotSupported  = errors.New("перегенерация недоступна при включённых инструментах")
)
