package domain

import "errors"

var (
	ErrNotFound                     = errors.New("не найдено")
	ErrUnauthorized                 = errors.New("недостаточно прав")
	ErrNoRunners                    = errors.New("нет активных раннеров")
	ErrRunnerChatModelNotConfigured = errors.New("у активного раннера не задана модель для чата")
	ErrRegenerateToolsNotSupported  = errors.New("перегенерация недоступна при включённых инструментах")
	ErrRAGNotConfigured             = errors.New("RAG по файлам не настроен")
	ErrRAGIndexNotReady             = errors.New("индекс файла для RAG ещё не готов")
	ErrRAGIndexFailed               = errors.New("индексация файла для RAG завершилась с ошибкой")
	ErrRAGNoHits                    = errors.New("по запросу не найдено релевантных фрагментов в индексе")
)
