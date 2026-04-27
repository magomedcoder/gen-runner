# MCP Bitrix24

Сервер MCP для задач Bitrix24 через входящий webhook:

- список задач (`tasks.task.list`)
- получение задачи (`tasks.task.get`)
- комментарии (`task.commentitem.getlist`)
- сводка по задаче (`b24_analyze_task`)
- аналитика по запросу (`b24_analyze_tasks_by_query`)
- портфельная аналитика (`b24_analyze_tasks_portfolio`)
- executive summary (`b24_analyze_tasks_executive_summary`)
- SLA-аналитика (`b24_analyze_tasks_sla`)

## Переменные окружения

| Переменная         | Назначение                                                                       |
|--------------------|----------------------------------------------------------------------------------|
| `B24_WEBHOOK_BASE` | Базовый URL webhook, например `https://bitrix24.example.com/rest/43176/00000000` |

---

## Transport stdio

Сборка:

```bash
go build -o ./build/mcp-bitrix24-stdio ./mcp-servers/mcp-bitrix24/cmd/mcp-bitrix24-stdio
```

В GEN:

- `transport = stdio`
- `command` - путь к бинарнику
- `args` - обычно пустой массив (URL задаётся через `B24_WEBHOOK_BASE`)

---

## Transport SSE

Сборка:

```bash
go build -o ./build/mcp-bitrix24-sse ./mcp-servers/mcp-bitrix24/cmd/mcp-bitrix24-sse
```

Запуск:

```bash
export B24_WEBHOOK_BASE="https://bitrix24.example.com/rest/1/00000000"
./build/mcp-bitrix24-sse -listen 127.0.0.1:8785
```

```
transport = sse

url = http://127.0.0.1:8785/
```

---

## Transport streamable HTTP

Сборка:

```bash
go build -o ./build/mcp-bitrix24-streamable ./mcp-servers/mcp-bitrix24/cmd/mcp-bitrix24-streamable
```

Запуск:

```bash
export B24_WEBHOOK_BASE="https://bitrix24.example.com/rest/1/00000000"
./build/mcp-bitrix24-streamable -listen 127.0.0.1:8786
```

```
transport = streamable

url = http://127.0.0.1:8786/
```

---

## Инструменты MCP

Схемы аргументов отдаёт сам сервер MCP (поля инструментов):

| Tool                                  | Назначение                                                                                                    |
|---------------------------------------|---------------------------------------------------------------------------------------------------------------|
| `b24_list_tasks`                      | `tasks.task.list` (`filter`, `select`, `order`, `params`, `start`)                                            |
| `b24_get_task`                        | `tasks.task.get` (`task_id`, `select`)                                                                        |
| `b24_get_task_comments`               | `task.commentitem.getlist` (`task_id`, `order`, `filter`)                                                     |
| `b24_analyze_task`                    | Глубокий анализ одной задачи (`task_id`, `include_comments`)                                                  |
| `b24_analyze_tasks_by_query`          | Аналитика по текстовому запросу (`query`, `task_id`, `filter`, `order`, `start`, `limit`, `include_comments`) |
| `b24_analyze_tasks_portfolio`         | Портфельная аналитика (`filter`, `order`, `start`, `limit`, `include_comments`, `group_by`)                   |
| `b24_analyze_tasks_executive_summary` | Управленческая сводка за период (`filter`, `order`, `start`, `limit`, `period_days`, `include_comments`)      |
| `b24_analyze_tasks_sla`               | SLA-контроль (`filter`, `order`, `start`, `limit`, `soon_hours_threshold`, `include_comments`)                |
| `b24_analyze_tasks_workload`          | Баланс нагрузки по ответственным (`filter`, `order`, `start`, `limit`, `include_comments`, `overload_tasks`)  |
| `b24_analyze_tasks_status_trends`     | Тренды по статусам (`filter`, `order`, `start`, `limit`, `period_days`)                                       |

Примечания:

- `task_id` в tools принимает как число, так и строку с цифрами.
- Для `task.commentitem.getlist` используется безопасный вызов с фиксированным порядком параметров (`TASKID`, `ORDER`, `FILTER`).
- Если комментарии недоступны (например, ограничения новой карточки задач), аналитика продолжает работать в soft-режиме без падения tool.

### Формат финального вывода аналитики

Во всех аналитических tools итог содержит унифицированный блок:

```text
=== Вывод ===
Статус: ...
Главный риск: ...
Приоритетное действие: ...
Срок реакции: ...
```

### Логирование

В логах сервера фиксируются:

- входные аргументы каждого tool (`tool=... args=...`);
- итоговый результат каждого tool (`tool=... result=...`);
- исходящий REST-запрос в Bitrix (`request_body`);
- входящий REST-ответ от Bitrix (`response_body`).

## Примеры запросов

Ниже примеры аргументов для вызова tools:

```json
{
  "tool": "b24_analyze_task",
  "arguments": {
    "task_id": "1822404",
    "include_comments": true
  }
}
```

```json
{
  "tool": "b24_analyze_tasks_by_query",
  "arguments": {
    "query": "покажи просроченные задачи с блокерами",
    "filter": {
      "RESPONSIBLE_ID": 547
    },
    "limit": 20
  }
}
```

```json
{
  "tool": "b24_analyze_tasks_sla",
  "arguments": {
    "filter": {
      "!STATUS": 5
    },
    "soon_hours_threshold": 24,
    "limit": 40
  }
}
```

```json
{
  "tool": "b24_analyze_tasks_workload",
  "arguments": {
    "overload_tasks": 12,
    "limit": 50
  }
}
```

## Рекомендованные сценарии

- **SLA-контроль**: `b24_analyze_tasks_sla` ежедневно для P0/P1 очереди реакции.
- **Портфель команды**: `b24_analyze_tasks_portfolio` для weekly-обзора по ответственным.
- **Executive summary**: `b24_analyze_tasks_executive_summary` для управленческого отчета по периоду.
- **Точечный разбор**: `b24_analyze_task` по конкретной задаче с финальным actionable-выводом.
- **Тренды статусов**: `b24_analyze_tasks_status_trends` для контроля накопления open/deferred.
- **Баланс нагрузки**: `b24_analyze_tasks_workload` для выявления перегруза и перераспределения.

## Известные ограничения Bitrix24

- `task.commentitem.getlist` имеет ограничения в новой карточке задач и может возвращать ошибки на части задач.
- Для `task.commentitem.getlist` важен порядок параметров (`TASKID`, `ORDER`, `FILTER`), в сервере он фиксируется принудительно.
- Права вебхука/пользователя напрямую влияют на полноту данных по задачам и комментариям.
- Возможны лимиты интенсивности запросов (`QUERY_LIMIT_EXCEEDED`) и временные server-side сбои.

---

## Mock REST (локальная отладка)

Отдельный бинарник - не MCP, а простой HTTP-мок Bitrix REST:

```bash
go build -o ./build/mcp-bitrix24-mock-rest ./mcp-servers/mcp-bitrix24/cmd/mcp-bitrix24-mock-rest
./build/mcp-bitrix24-mock-rest -listen 127.0.0.1:8899
```

База методов: `http://127.0.0.1:8899/rest/1/mock-token/<method>`.
