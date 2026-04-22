package llama

type ChatMessage struct {
	Role       string // Роль сообщения (например - system, user, assistant)
	Content    string // Содержимое сообщения
	ImageBytes []byte // Опциональные байты изображения (jpeg/png/webp/gif) для libmtmd на этом сообщении
}

type ChatResponse struct {
	Content          string // Обычное содержимое ответа
	ReasoningContent string // Извлеченное reasoning/thinking (если модель поддерживает reasoning)
}

type ChatDelta struct {
	Content          string // Токен(ы) обычного контента
	ReasoningContent string // Токен(ы) reasoning-контента
}
