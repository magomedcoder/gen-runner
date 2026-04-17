const Map<String, String> kGrpcErrorInfoReasonMessages = {
  'CHAT_UNAUTHORIZED': 'Нет доступа.',
  'CHAT_INVALID_SESSION_ID': 'Некорректная сессия.',
  'CHAT_INVALID_REQUEST': 'Некорректный запрос.',
  'CHAT_SEND_EMPTY_MESSAGE': 'Укажите текст или вложение.',
  'CHAT_RAG_NOT_CONFIGURED': 'RAG для файлов не настроен.',
  'CHAT_RAG_INDEX_NOT_READY': 'Индекс файлов ещё не готов.',
  'CHAT_RAG_INDEX_FAILED': 'Ошибка индексации для RAG.',
  'CHAT_RAG_NO_HITS': 'По файлам ничего не найдено.',
  'CHAT_MODEL_UNAVAILABLE': 'Выбранная модель недоступна.',
  'CHAT_NO_MODELS_AVAILABLE': 'Нет доступных моделей.',
  'MCP_SERVICE_UNAVAILABLE': 'MCP недоступен.',
  'MCP_SERVER_NOT_FOUND': 'MCP-сервер не найден.',
  'MCP_USER_SERVER_LIMIT_EXCEEDED': 'Достигнут лимит личных MCP-серверов.',
  'MCP_EDIT_OWNED_ONLY': 'Можно редактировать только свои MCP-серверы.',
  'MCP_DELETE_OWNED_ONLY': 'Можно удалять только свои MCP-серверы.',
  'AUTH_ADMIN_REQUIRED': 'Нужны права администратора.',
  'AUTH_TOKEN_INVALID': 'Сессия истекла или токен недействителен.',
  'AUTH_METADATA_MISSING': 'Нет метаданных авторизации.',
  'AUTH_AUTHORIZATION_HEADER_MISSING': 'Нет заголовка авторизации.',
  'AUTH_BEARER_FORMAT_INVALID': 'Неверный формат Bearer-токена.',
  'GEN_INTERNAL_ERROR': 'Внутренняя ошибка сервера.',
  'GEN_NOT_FOUND': 'Не найдено.',
  'GEN_UNAUTHENTICATED': 'Требуется вход.',
  'GEN_PERMISSION_DENIED': 'Доступ запрещён.',
  'GEN_UNAVAILABLE': 'Сервис временно недоступен.',
};

String? messageForGrpcErrorInfoReason(String? reason) {
  if (reason == null || reason.isEmpty) {
    return null;
  }
  return kGrpcErrorInfoReasonMessages[reason];
}
