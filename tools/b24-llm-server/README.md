# b24-llm-server

HTTP сервер в `b24-llm-server`, который принимает контекст задачи Bitrix24 и возвращает ответ LLM

## Запуск

```bash
go run ./tools/b24-llm-server/cmd
```

Переменные окружения:

- `B24_LLM_SERVER_HOST` - адрес HTTP сервера
- `GEN_RUNNER_ADDR` - адрес gen runner
- `GEN_MODEL` - модель для генерации

## API

- `GET /b24/v1/health` - проверка доступности
- `POST /b24/v1/analyze` - анализ задачи

Пример запроса:

```json
{
  "task_id": "123",
  "task_title": "Собрать релиз",
  "task_description": "Нужно подготовить changelog",
  "task_status": "in_progress",
  "task_deadline": "2026-05-01",
  "task_assignee": "42",
  "comments": [
    {
      "author":"Test",
      "text":"Нужны финальные правки",
      "time":"2026-04-30 11:00"
    }
  ],
  "history": [],
  "prompt": "Сделай план выполнения"
}
```
