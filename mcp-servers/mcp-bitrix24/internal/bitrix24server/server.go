package bitrix24server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/magomedcoder/gen/pkg/collections"
	"github.com/magomedcoder/gen/pkg/mcpresult"
	"github.com/magomedcoder/gen/pkg/mcpsafe"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Config struct {
	WebhookBase           string
	LogLevel              string
	RetryMax              int
	RetryBackoffMS        int
	DisableHeavyAnalytics bool
}

func NewServer(cfg Config) (*mcp.Server, error) {
	timeout := 20 * time.Second
	if strings.TrimSpace(cfg.LogLevel) == "" {
		cfg.LogLevel = "info"
	}

	if cfg.RetryMax < 0 {
		cfg.RetryMax = 0
	}

	if cfg.RetryBackoffMS <= 0 {
		cfg.RetryBackoffMS = 300
	}

	log.Printf("[b24-mcp] init server timeout=%s webhook_base_set=%t log_level=%s retry_max=%d retry_backoff_ms=%d", timeout, strings.TrimSpace(cfg.WebhookBase) != "", cfg.LogLevel, cfg.RetryMax, cfg.RetryBackoffMS)
	initTelemetryReporter()

	client, err := newBitrixClient(cfg.WebhookBase, timeout, cfg.LogLevel, cfg.RetryMax, time.Duration(cfg.RetryBackoffMS)*time.Millisecond)
	if err != nil {
		return nil, err
	}
	heavyAnalyticsEnabled := !cfg.DisableHeavyAnalytics

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "bitrix24",
		Version: "1.0.0",
	}, nil)

	type listTasksArgs struct {
		Filter map[string]any `json:"filter,omitempty" jsonschema:"Фильтры задач Bitrix24 (например UF_* поля)"`
		Select []string       `json:"select,omitempty" jsonschema:"Список полей для выборки"`
		Order  map[string]any `json:"order,omitempty" jsonschema:"Сортировка, например {\"CREATED_DATE\":\"desc\"}"`
		Params map[string]any `json:"params,omitempty" jsonschema:"Дополнительные параметры, например {\"WITH_TIMER_INFO\":true}"`
		Start  *int           `json:"start,omitempty" jsonschema:"Пагинация Bitrix24: offset"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "b24_list_tasks",
		Description: "Получить список задач Bitrix24 (tasks.task.list, read-only). Если нужен конкретный ID задачи — используйте b24_get_task.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args listTasksArgs) (*mcp.CallToolResult, any, error) {
		return mcpsafe.SafeToolInvoke("mcp-bitrix24", "b24_list_tasks", func() (*mcp.CallToolResult, any, error) {
			ctx, rid := withRequestID(ctx)
			log.Printf("[b24-mcp] tool=b24_list_tasks req_id=%s", rid)
			logToolArgs("b24_list_tasks", args)
			if err := validateListTasksArgs(args.Start, args.Filter); err != nil {
				return nil, nil, err
			}

			log.Printf("[b24-mcp] tool=b24_list_tasks filter_keys=%d select=%d order_keys=%d has_start=%t", len(args.Filter), len(args.Select), len(args.Order), args.Start != nil)
			if taskID, ok := extractTaskIDFromListFilter(args.Filter); ok {
				log.Printf("[b24-mcp] tool=b24_list_tasks redirect=tasks.task.get task_id=%d reason=explicit_id_filter", taskID)
				payload := map[string]any{
					"taskId": taskID,
				}

				if len(args.Select) > 0 {
					payload["select"] = collections.CloneStringSlice(args.Select)
				}

				resp, err := client.call(ctx, "tasks.task.get", payload)
				if err != nil {
					log.Printf("[b24-mcp] tool=b24_list_tasks redirect_task_id=%d err=%v", taskID, err)
					return nil, nil, err
				}

				task, err := extractTask(resp)
				if err != nil {
					log.Printf("[b24-mcp] tool=b24_list_tasks redirect_task_id=%d stage=extract_task err=%v", taskID, err)
					return nil, nil, err
				}

				normalized := map[string]any{
					"result": map[string]any{
						"mode":            "single_task_by_id_filter",
						"task":            task,
						"task_normalized": normalizeTaskSnapshot(task),
					},
					"raw": resp,
				}

				log.Printf("[b24-mcp] tool=b24_list_tasks redirect_task_id=%d ok", taskID)
				logToolResult("b24_list_tasks", normalized)
				return mcpresult.TextAndPayload("b24_list_tasks", normalized), nil, nil
			}

			payload := map[string]any{}
			if args.Filter != nil {
				payload["filter"] = collections.CloneAnyMap(args.Filter)
			}

			if len(args.Select) > 0 {
				payload["select"] = collections.CloneStringSlice(args.Select)
			}

			if args.Order != nil {
				payload["order"] = collections.CloneAnyMap(args.Order)
			}

			if args.Params != nil {
				payload["params"] = collections.CloneAnyMap(args.Params)
			}

			if args.Start != nil {
				payload["start"] = *args.Start
			}

			resp, err := client.call(ctx, "tasks.task.list", payload)
			if err != nil {
				log.Printf("[b24-mcp] tool=b24_list_tasks err=%v", err)
				return nil, nil, err
			}

			tasks, _ := extractTaskListPage(resp)
			normalizedTasks := make([]TaskSnapshot, 0, len(tasks))
			for _, task := range tasks {
				normalizedTasks = append(normalizedTasks, normalizeTaskSnapshot(task))
			}

			normalized := map[string]any{
				"result": map[string]any{
					"mode":             "task_list",
					"tasks":            tasks,
					"tasks_normalized": normalizedTasks,
					"count":            len(tasks),
					"next":             resp["next"],
				},
				"raw": resp,
			}

			log.Printf("[b24-mcp] tool=b24_list_tasks ok")
			logToolResult("b24_list_tasks", normalized)

			return mcpresult.TextAndPayload("b24_list_tasks", normalized), nil, nil
		})
	})

	type getTaskArgs struct {
		TaskID any      `json:"task_id" jsonschema:"ID задачи (число или строка с цифрами)"`
		Select []string `json:"select,omitempty" jsonschema:"Список полей для выборки"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "b24_get_task",
		Description: "Получить задачу по ID (tasks.task.get, read-only)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args getTaskArgs) (*mcp.CallToolResult, any, error) {
		return mcpsafe.SafeToolInvoke("mcp-bitrix24", "b24_get_task", func() (*mcp.CallToolResult, any, error) {
			ctx, rid := withRequestID(ctx)
			log.Printf("[b24-mcp] tool=b24_get_task req_id=%s", rid)
			logToolArgs("b24_get_task", args)
			taskID, err := parsePositiveTaskID(args.TaskID)
			if err != nil {
				return nil, nil, err
			}
			log.Printf("[b24-mcp] tool=b24_get_task task_id=%d select=%d", taskID, len(args.Select))

			payload := map[string]any{
				"taskId": taskID,
			}

			if len(args.Select) > 0 {
				payload["select"] = collections.CloneStringSlice(args.Select)
			}

			resp, err := client.call(ctx, "tasks.task.get", payload)
			if err != nil {
				log.Printf("[b24-mcp] tool=b24_get_task task_id=%d err=%v", taskID, err)
				return nil, nil, err
			}

			task, err := extractTask(resp)
			if err != nil {
				log.Printf("[b24-mcp] tool=b24_get_task task_id=%d stage=extract_task err=%v", taskID, err)
				return nil, nil, err
			}

			normalized := map[string]any{
				"result": map[string]any{
					"task":            task,
					"task_normalized": normalizeTaskSnapshot(task),
				},
				"raw": resp,
			}

			log.Printf("[b24-mcp] tool=b24_get_task task_id=%d ok", taskID)
			logToolResult("b24_get_task", normalized)

			return mcpresult.TextAndPayload("b24_get_task", normalized), nil, nil
		})
	})

	type getCommentsArgs struct {
		TaskID any            `json:"task_id,omitempty" jsonschema:"ID задачи (число или строка с цифрами)"`
		Order  map[string]any `json:"order,omitempty" jsonschema:"Сортировка комментариев, например {\"POST_DATE\":\"asc\"}"`
		Filter map[string]any `json:"filter,omitempty" jsonschema:"Фильтр комментариев, например {\"AUTHOR_ID\":503}"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "b24_get_task_comments",
		Description: "Получить комментарии задачи (task.commentitem.getlist, read-only)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args getCommentsArgs) (*mcp.CallToolResult, any, error) {
		return mcpsafe.SafeToolInvoke("mcp-bitrix24", "b24_get_task_comments", func() (*mcp.CallToolResult, any, error) {
			ctx, rid := withRequestID(ctx)
			log.Printf("[b24-mcp] tool=b24_get_task_comments req_id=%s", rid)
			logToolArgs("b24_get_task_comments", args)
			taskIDPtr, err := parseOptionalTaskID(args.TaskID)
			if err != nil {
				return nil, nil, err
			}
			if taskIDPtr == nil {
				msg := "Для b24_get_task_comments нужно передать task_id. " +
					"Пример: {\"task_id\": 1001}. " +
					"Если нужен обзор задач, используйте b24_list_tasks или b24_analyze_tasks_by_query."
				log.Printf("[b24-mcp] tool=b24_get_task_comments отсутствует_task_id")
				logToolResult("b24_get_task_comments", msg)
				return mcpresult.Text(msg), nil, nil
			}
			taskID := *taskIDPtr
			if taskID <= 0 {
				return nil, nil, errTaskIDMustBePositive
			}

			log.Printf("[b24-mcp] tool=b24_get_task_comments task_id=%d order_keys=%d filter_keys=%d", taskID, len(args.Order), len(args.Filter))

			resp, err := client.callTaskCommentItemGetList(ctx, taskID, collections.CloneAnyMap(args.Order), collections.CloneAnyMap(args.Filter))
			if err != nil {
				log.Printf("[b24-mcp] tool=b24_get_task_comments task_id=%d err=%v", taskID, err)
				return nil, nil, err
			}

			comments := extractComments(resp)
			normalizedComments := normalizeCommentSnapshots(comments, taskID)
			commentsTotal := commentsTotalFromResponse(resp)
			parseWarnings := commentParseWarnings(commentsTotal, len(normalizedComments))
			normalized := map[string]any{
				"result": map[string]any{
					"comments":                comments,
					"comments_normalized":     normalizedComments,
					"count":                   len(comments),
					"count_normalized":        len(normalizedComments),
					"comments_total":          commentsTotal,
					"comments_parsed":         len(normalizedComments),
					"comments_parse_warnings": parseWarnings,
				},
				"raw": resp,
			}

			if len(comments) == 0 {
				log.Printf("[b24-mcp] tool=b24_get_task_comments task_id=%d warn=comments_empty raw_result_type=%T", taskID, resp["result"])
			}

			log.Printf("[b24-mcp] tool=b24_get_task_comments task_id=%d ok", taskID)
			logToolResult("b24_get_task_comments", normalized)

			return mcpresult.TextAndPayload("b24_get_task_comments", normalized), nil, nil
		})
	})

	type getTaskTimelineArgs struct {
		TaskID          any   `json:"task_id" jsonschema:"ID задачи (число или строка с цифрами)"`
		IncludeComments *bool `json:"include_comments,omitempty" jsonschema:"Включать комментарии в ленту (по умолчанию true)"`
		Limit           *int  `json:"limit,omitempty" jsonschema:"Макс. событий в ленте (1..200), по умолчанию 50"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "b24_get_task_timeline",
		Description: "Получить таймлайн задачи: ключевые события + комментарии (read-only)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args getTaskTimelineArgs) (*mcp.CallToolResult, any, error) {
		return mcpsafe.SafeToolInvoke("mcp-bitrix24", "b24_get_task_timeline", func() (*mcp.CallToolResult, any, error) {
			if !heavyAnalyticsEnabled {
				msg := "b24_get_task_timeline отключен feature-flag `DisableHeavyAnalytics`."
				return mcpresult.Text(msg), nil, nil
			}

			ctx, rid := withRequestID(ctx)
			log.Printf("[b24-mcp] tool=b24_get_task_timeline req_id=%s", rid)
			logToolArgs("b24_get_task_timeline", args)
			taskID, err := parsePositiveTaskID(args.TaskID)
			if err != nil {
				return nil, nil, err
			}

			limit := 50
			if args.Limit != nil {
				if *args.Limit < 1 || *args.Limit > 200 {
					return nil, nil, fmt.Errorf("limit должно быть in range [1..200]")
				}
				limit = *args.Limit
			}

			includeComments := true
			if args.IncludeComments != nil {
				includeComments = *args.IncludeComments
			}

			taskResp, err := client.call(ctx, "tasks.task.get", map[string]any{
				"taskId": taskID,
				"select": []string{
					"ID", "TITLE", "STATUS", "CREATED_DATE", "CHANGED_DATE", "DEADLINE", "CLOSED_DATE", "ACTIVITY_DATE", "CREATED_BY", "RESPONSIBLE_ID", "PRIORITY",
				},
			})

			if err != nil {
				log.Printf("[b24-mcp] tool=b24_get_task_timeline task_id=%d stage=task_get err=%v", taskID, err)
				return nil, nil, err
			}

			taskRaw, err := extractTask(taskResp)
			if err != nil {
				log.Printf("[b24-mcp] tool=b24_get_task_timeline task_id=%d stage=extract_task err=%v", taskID, err)
				return nil, nil, err
			}

			taskNormalized := normalizeTaskSnapshot(taskRaw)

			var commentsRaw []map[string]any
			var commentsNormalized []CommentSnapshot
			if includeComments {
				commentsResp, err := client.callTaskCommentItemGetList(ctx, taskID, map[string]any{"POST_DATE": "desc"}, nil)
				if err != nil {
					if isIgnorableCommentError(err) {
						log.Printf("[b24-mcp] tool=b24_get_task_timeline task_id=%d stage=comments soft_skip err=%v", taskID, err)
					} else {
						log.Printf("[b24-mcp] tool=b24_get_task_timeline task_id=%d stage=comments err=%v", taskID, err)
						return nil, nil, err
					}
				} else {
					commentsRaw = extractComments(commentsResp)
					commentsNormalized = normalizeCommentSnapshots(commentsRaw, taskID)
				}
			}

			timeline := buildTaskTimeline(taskNormalized, commentsNormalized)
			if len(timeline) > limit {
				timeline = timeline[:limit]
			}

			payload := map[string]any{
				"result": map[string]any{
					"task":                taskRaw,
					"task_normalized":     taskNormalized,
					"comments":            commentsRaw,
					"comments_normalized": commentsNormalized,
					"timeline":            timeline,
					"timeline_count":      len(timeline),
					"include_comments":    includeComments,
					"limit":               limit,
				},
			}
			log.Printf("[b24-mcp] tool=b24_get_task_timeline task_id=%d ok timeline=%d", taskID, len(timeline))
			logToolResult("b24_get_task_timeline", payload)
			return mcpresult.TextAndPayload("b24_get_task_timeline", payload), nil, nil
		})
	})

	type analyzeBlockersArgs struct {
		TaskID any  `json:"task_id,omitempty" jsonschema:"ID задачи (число или строка с цифрами)"`
		Limit  *int `json:"limit,omitempty" jsonschema:"Макс. найденных блокеров (1..100), по умолчанию 20"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "b24_analyze_task_blockers",
		Description: "Анализ блокеров по задаче на основе комментариев (read-only)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args analyzeBlockersArgs) (*mcp.CallToolResult, any, error) {
		return mcpsafe.SafeToolInvoke("mcp-bitrix24", "b24_analyze_task_blockers", func() (*mcp.CallToolResult, any, error) {
			if !heavyAnalyticsEnabled {
				msg := "b24_analyze_task_blockers отключен feature-flag `DisableHeavyAnalytics`."
				return mcpresult.Text(msg), nil, nil
			}

			ctx, rid := withRequestID(ctx)
			log.Printf("[b24-mcp] tool=b24_analyze_task_blockers req_id=%s", rid)
			logToolArgs("b24_analyze_task_blockers", args)
			taskIDPtr, err := parseOptionalTaskID(args.TaskID)
			if err != nil {
				return nil, nil, err
			}
			if taskIDPtr == nil {
				msg := "Для b24_analyze_task_blockers нужно передать task_id. " +
					"Пример: {\"task_id\": 1001}. " +
					"Если нужен обзор без конкретной задачи, используйте b24_analyze_tasks_by_query."
				log.Printf("[b24-mcp] tool=b24_analyze_task_blockers отсутствует_task_id")
				logToolResult("b24_analyze_task_blockers", msg)
				return mcpresult.Text(msg), nil, nil
			}
			taskID := *taskIDPtr
			if taskID <= 0 {
				return nil, nil, errTaskIDMustBePositive
			}

			limit := 20
			if args.Limit != nil {
				if *args.Limit < 1 || *args.Limit > 100 {
					return nil, nil, fmt.Errorf("limit должно быть in range [1..100]")
				}
				limit = *args.Limit
			}

			taskResp, err := client.call(ctx, "tasks.task.get", map[string]any{
				"taskId": taskID,
				"select": []string{"ID", "TITLE", "STATUS", "DEADLINE", "RESPONSIBLE_ID", "CREATED_BY"},
			})
			if err != nil {
				log.Printf("[b24-mcp] tool=b24_analyze_task_blockers task_id=%d stage=task_get err=%v", taskID, err)
				return nil, nil, err
			}

			taskRaw, err := extractTask(taskResp)
			if err != nil {
				log.Printf("[b24-mcp] tool=b24_analyze_task_blockers task_id=%d stage=extract_task err=%v", taskID, err)
				return nil, nil, err
			}

			taskNormalized := normalizeTaskSnapshot(taskRaw)

			commentsResp, err := client.callTaskCommentItemGetList(ctx, taskID, map[string]any{"POST_DATE": "desc"}, nil)
			if err != nil {
				if isIgnorableCommentError(err) {
					log.Printf("[b24-mcp] tool=b24_analyze_task_blockers task_id=%d stage=comments soft_skip err=%v", taskID, err)
					payload := map[string]any{
						"result": map[string]any{
							"task_normalized":   taskNormalized,
							"blockers":          []BlockerSignal{},
							"blockers_count":    0,
							"blocker_owners":    []string{},
							"summary":           "Комментарии недоступны, блокеры не могут быть достоверно определены.",
							"insufficient_data": true,
						},
					}
					return mcpresult.TextAndPayload("b24_analyze_task_blockers", payload), nil, nil
				}

				return nil, nil, err
			}

			commentsRaw := extractComments(commentsResp)
			commentsNormalized := normalizeCommentSnapshots(commentsRaw, taskID)
			signals := detectBlockerSignals(commentsNormalized, time.Now())
			if len(signals) > limit {
				signals = signals[:limit]
			}

			owners := blockerOwners(signals)

			payload := map[string]any{
				"result": map[string]any{
					"task":              taskRaw,
					"task_normalized":   taskNormalized,
					"comments_count":    len(commentsNormalized),
					"blockers":          signals,
					"blockers_count":    len(signals),
					"blocker_owners":    owners,
					"summary":           blockerSummary(signals),
					"insufficient_data": false,
				},
			}
			log.Printf("[b24-mcp] tool=b24_analyze_task_blockers task_id=%d ok blockers=%d", taskID, len(signals))
			logToolResult("b24_analyze_task_blockers", payload)
			return mcpresult.TextAndPayload("b24_analyze_task_blockers", payload), nil, nil
		})
	})

	type analyzeExecutionDriftArgs struct {
		TaskID any `json:"task_id,omitempty" jsonschema:"ID задачи (число или строка с цифрами)"`
	}
	analyzeExecutionDriftHandler := func(toolName string) func(context.Context, *mcp.CallToolRequest, analyzeExecutionDriftArgs) (*mcp.CallToolResult, any, error) {
		return func(ctx context.Context, _ *mcp.CallToolRequest, args analyzeExecutionDriftArgs) (*mcp.CallToolResult, any, error) {
			return mcpsafe.SafeToolInvoke("mcp-bitrix24", toolName, func() (*mcp.CallToolResult, any, error) {
				if !heavyAnalyticsEnabled {
					msg := toolName + " отключен feature-flag `DisableHeavyAnalytics`."
					return mcpresult.Text(msg), nil, nil
				}
				ctx, rid := withRequestID(ctx)
				log.Printf("[b24-mcp] tool=%s req_id=%s", toolName, rid)
				logToolArgs(toolName, args)
				taskIDPtr, err := parseOptionalTaskID(args.TaskID)
				if err != nil {
					return nil, nil, err
				}
				if taskIDPtr == nil {
					msg := "Для " + toolName + " нужно передать task_id. " +
						"Пример: {\"task_id\": 1001}. " +
						"Если нужен обзор без конкретной задачи, используйте b24_analyze_tasks_by_query."
					log.Printf("[b24-mcp] tool=%s отсутствует_task_id", toolName)
					logToolResult(toolName, msg)
					return mcpresult.Text(msg), nil, nil
				}
				taskID := *taskIDPtr
				if taskID <= 0 {
					return nil, nil, errTaskIDMustBePositive
				}

				taskResp, err := client.call(ctx, "tasks.task.get", map[string]any{
					"taskId": taskID,
					"select": []string{
						"ID", "TITLE", "STATUS", "DEADLINE", "RESPONSIBLE_ID", "CREATED_BY", "TIME_ESTIMATE", "TIME_SPENT_IN_LOGS", "CHANGED_DATE",
					},
				})
				if err != nil {
					log.Printf("[b24-mcp] tool=%s task_id=%d stage=task_get err=%v", toolName, taskID, err)
					return nil, nil, err
				}

				taskRaw, err := extractTask(taskResp)
				if err != nil {
					return nil, nil, err
				}

				taskNorm := normalizeTaskSnapshot(taskRaw)

				var commentsNorm []CommentSnapshot
				commentsResp, err := client.callTaskCommentItemGetList(ctx, taskID, map[string]any{"POST_DATE": "desc"}, nil)
				if err == nil {
					commentsNorm = normalizeCommentSnapshots(extractComments(commentsResp), taskID)
				} else if isIgnorableCommentError(err) {
					log.Printf("[b24-mcp] tool=%s task_id=%d stage=comments soft_skip err=%v", toolName, taskID, err)
				} else {
					return nil, nil, err
				}

				report := buildExecutionDriftReport(taskNorm, commentsNorm, time.Now())
				payload := map[string]any{
					"result": map[string]any{
						"task_normalized": taskNorm,
						"drift_report":    report,
						"actions":         executionDriftActions(report),
					},
				}

				log.Printf("[b24-mcp] tool=%s task_id=%d ok drift=%s", toolName, taskID, report.DriftLevel)
				logToolResult(toolName, payload)
				return mcpresult.TextAndPayload(toolName, payload), nil, nil
			})
		}
	}

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "b24_analyze_task_execution_drift",
		Description: "Анализ дрифта исполнения задачи: факт/план времени и коммуникационная тишина (read-only)",
	}, analyzeExecutionDriftHandler("b24_analyze_task_execution_drift"))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "b24_analyze_tasks_execution_drift",
		Description: "Алиас к b24_analyze_task_execution_drift (совместимость вызовов модели).",
	}, analyzeExecutionDriftHandler("b24_analyze_tasks_execution_drift"))

	type analyzeArgs struct {
		TaskID          any   `json:"task_id,omitempty" jsonschema:"ID задачи для анализа (число или строка с цифрами)"`
		IncludeComments *bool `json:"include_comments,omitempty" jsonschema:"Подтянуть комментарии для анализа (по умолчанию true)"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "b24_analyze_task",
		Description: "Быстрый анализ задачи: статус, дедлайн, активность, риски (read-only)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args analyzeArgs) (*mcp.CallToolResult, any, error) {
		return mcpsafe.SafeToolInvoke("mcp-bitrix24", "b24_analyze_task", func() (*mcp.CallToolResult, any, error) {
			ctx, rid := withRequestID(ctx)
			log.Printf("[b24-mcp] tool=b24_analyze_task req_id=%s", rid)
			logToolArgs("b24_analyze_task", args)
			taskIDPtr, err := parseOptionalTaskID(args.TaskID)
			if err != nil {
				return nil, nil, err
			}

			if taskIDPtr == nil {
				msg := "Для b24_analyze_task нужно передать task_id. " +
					"Пример: {\"task_id\": 1001}. " +
					"Если нужен обзор без конкретной задачи, используйте b24_analyze_tasks_by_query."
				log.Printf("[b24-mcp] tool=b24_analyze_task отсутствует_task_id")
				logToolResult("b24_analyze_task", msg)
				return mcpresult.Text(msg), nil, nil
			}

			taskID := *taskIDPtr
			if taskID <= 0 {
				return nil, nil, errTaskIDMustBePositive
			}

			includeComments := true
			if args.IncludeComments != nil {
				includeComments = *args.IncludeComments
			}

			log.Printf("[b24-mcp] tool=b24_analyze_task task_id=%d include_comments=%t", taskID, includeComments)

			taskResp, err := client.call(ctx, "tasks.task.get", map[string]any{
				"taskId": taskID,
				"select": []string{
					"ID",
					"TITLE",
					"CREATED_DATE",
					"DEADLINE",
					"STATUS",
					"CREATED_BY",
					"RESPONSIBLE_ID",
				},
			})
			if err != nil {
				log.Printf("[b24-mcp] tool=b24_analyze_task task_id=%d stage=task_get err=%v", taskID, err)
				return nil, nil, err
			}

			task, err := extractTask(taskResp)
			if err != nil {
				log.Printf("[b24-mcp] tool=b24_analyze_task task_id=%d stage=extract_task err=%v", taskID, err)
				return nil, nil, err
			}

			var comments []map[string]any
			if includeComments {
				commentResp, err := client.callTaskCommentItemGetList(ctx, taskID, map[string]any{"ID": "asc"}, nil)

				if err != nil {
					if isIgnorableCommentError(err) {
						log.Printf("[b24-mcp] tool=b24_analyze_task task_id=%d stage=comments soft_skip err=%v", taskID, err)
					} else {
						log.Printf("[b24-mcp] tool=b24_analyze_task task_id=%d stage=comments err=%v", taskID, err)
						return nil, nil, err
					}
				} else {
					comments = extractComments(commentResp)
				}
			}

			analysis := analyzeTask(task, comments, time.Now())
			log.Printf("[b24-mcp] tool=b24_analyze_task task_id=%d ok comments=%d", taskID, len(comments))
			logToolResult("b24_analyze_task", analysis)
			return mcpresult.TextAndJSON("b24_analyze_task", analysis), nil, nil
		})
	})

	type analyticsQueryArgs struct {
		Query           string         `json:"query,omitempty" jsonschema:"Текст requestа по аналитике, например: 'покажи просроченные и блокеры'. Если не задан, будет общий аналитический обзор."`
		TaskID          any            `json:"task_id,omitempty" jsonschema:"Опционально: анализ конкретной задачи (число или строка с цифрами)"`
		Filter          map[string]any `json:"filter,omitempty" jsonschema:"Фильтр для tasks.task.list, если нужен анализ набора задач"`
		Order           map[string]any `json:"order,omitempty" jsonschema:"Сортировка для tasks.task.list"`
		Start           *int           `json:"start,omitempty" jsonschema:"Смещение в tasks.task.list"`
		Limit           *int           `json:"limit,omitempty" jsonschema:"Макс. задач для аналитики (1..50), по умолчанию 20"`
		IncludeComments *bool          `json:"include_comments,omitempty" jsonschema:"Подтягивать комментарии при аналитике (по умолчанию false)"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "b24_analyze_tasks_by_query",
		Description: "Выполнить пользовательский request по аналитике задач (просрочки, риски, блокеры, активность, read-only)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args analyticsQueryArgs) (*mcp.CallToolResult, any, error) {
		return mcpsafe.SafeToolInvoke("mcp-bitrix24", "b24_analyze_tasks_by_query", func() (*mcp.CallToolResult, any, error) {
			ctx, rid := withRequestID(ctx)
			log.Printf("[b24-mcp] tool=b24_analyze_tasks_by_query req_id=%s", rid)
			logToolArgs("b24_analyze_tasks_by_query", args)
			if err := validateAnalyzeTasksByQueryArgs(args.Start, args.Limit); err != nil {
				return nil, nil, err
			}

			query := strings.TrimSpace(args.Query)
			if query == "" {
				query = "общий аналитический обзор"
			}

			taskID, err := parseOptionalTaskID(args.TaskID)
			if err != nil {
				return nil, nil, err
			}

			result, err := runAnalyticsQuery(ctx, client, query, taskID, args.Filter, args.Order, args.Start, args.Limit, args.IncludeComments)
			if err != nil {
				return nil, nil, err
			}

			logToolResult("b24_analyze_tasks_by_query", result)
			return mcpresult.TextAndJSON("b24_analyze_tasks_by_query", result), nil, nil
		})
	})

	type portfolioArgs struct {
		Filter          map[string]any `json:"filter,omitempty" jsonschema:"Фильтр задач для выборки портфеля"`
		Order           map[string]any `json:"order,omitempty" jsonschema:"Сортировка tasks.task.list"`
		Start           *int           `json:"start,omitempty" jsonschema:"Смещение в tasks.task.list"`
		Limit           *int           `json:"limit,omitempty" jsonschema:"Макс. задач для анализа (1..50), по умолчанию 30"`
		IncludeComments *bool          `json:"include_comments,omitempty" jsonschema:"Подтягивать комментарии (по умолчанию false)"`
		GroupBy         string         `json:"group_by,omitempty" jsonschema:"Группировка: responsible|creator|status (по умолчанию responsible)"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "b24_analyze_tasks_portfolio",
		Description: "Портфельная аналитика задач: сводка по responseственным/постановщикам, просрочки и риски (read-only)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args portfolioArgs) (*mcp.CallToolResult, any, error) {
		return mcpsafe.SafeToolInvoke("mcp-bitrix24", "b24_analyze_tasks_portfolio", func() (*mcp.CallToolResult, any, error) {
			ctx, rid := withRequestID(ctx)
			log.Printf("[b24-mcp] tool=b24_analyze_tasks_portfolio req_id=%s", rid)
			logToolArgs("b24_analyze_tasks_portfolio", args)
			if err := validatePortfolioArgs(args.Start, args.Limit, args.GroupBy); err != nil {
				return nil, nil, err
			}

			result, err := runPortfolioAnalytics(ctx, client, args.Filter, args.Order, args.Start, args.Limit, args.IncludeComments, args.GroupBy)
			if err != nil {
				return nil, nil, err
			}

			logToolResult("b24_analyze_tasks_portfolio", result)
			return mcpresult.TextAndJSON("b24_analyze_tasks_portfolio", result), nil, nil
		})
	})

	type executiveSummaryArgs struct {
		Filter          map[string]any `json:"filter,omitempty" jsonschema:"Фильтр задач для выборки отчета"`
		Order           map[string]any `json:"order,omitempty" jsonschema:"Сортировка tasks.task.list"`
		Start           *int           `json:"start,omitempty" jsonschema:"Смещение в tasks.task.list"`
		Limit           *int           `json:"limit,omitempty" jsonschema:"Макс. задач для отчета (1..50), по умолчанию 40"`
		PeriodDays      *int           `json:"period_days,omitempty" jsonschema:"Длина периода в днях для тренда (1..30), по умолчанию 7"`
		IncludeComments *bool          `json:"include_comments,omitempty" jsonschema:"Подтягивать комментарии для оценки блокеров (по умолчанию false)"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "b24_analyze_tasks_executive_summary",
		Description: "Управленческая сводка по задачам за период с трендами и ключевыми рисками (read-only)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args executiveSummaryArgs) (*mcp.CallToolResult, any, error) {
		return mcpsafe.SafeToolInvoke("mcp-bitrix24", "b24_analyze_tasks_executive_summary", func() (*mcp.CallToolResult, any, error) {
			ctx, rid := withRequestID(ctx)
			log.Printf("[b24-mcp] tool=b24_analyze_tasks_executive_summary req_id=%s", rid)
			logToolArgs("b24_analyze_tasks_executive_summary", args)
			if err := validateExecutiveSummaryArgs(args.Start, args.Limit, args.PeriodDays); err != nil {
				return nil, nil, err
			}

			result, err := runExecutiveSummary(ctx, client, args.Filter, args.Order, args.Start, args.Limit, args.PeriodDays, args.IncludeComments)
			if err != nil {
				return nil, nil, err
			}

			logToolResult("b24_analyze_tasks_executive_summary", result)
			return mcpresult.TextAndJSON("b24_analyze_tasks_executive_summary", result), nil, nil
		})
	})

	type slaArgs struct {
		Filter             map[string]any `json:"filter,omitempty" jsonschema:"Фильтр задач для SLA-аналитики"`
		Order              map[string]any `json:"order,omitempty" jsonschema:"Сортировка tasks.task.list"`
		Start              *int           `json:"start,omitempty" jsonschema:"Смещение в tasks.task.list"`
		Limit              *int           `json:"limit,omitempty" jsonschema:"Макс. задач (1..50), по умолчанию 40"`
		SoonHoursThreshold *int           `json:"soon_hours_threshold,omitempty" jsonschema:"Порог 'скоро дедлайн' в часах, по умолчанию 24"`
		IncludeComments    *bool          `json:"include_comments,omitempty" jsonschema:"Подтягивать комментарии для блокеров (по умолчанию false)"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "b24_analyze_tasks_sla",
		Description: "SLA-контроль задач: просрочки, дедлайн-сегодня/скоро, приоритет реакции (read-only)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args slaArgs) (*mcp.CallToolResult, any, error) {
		return mcpsafe.SafeToolInvoke("mcp-bitrix24", "b24_analyze_tasks_sla", func() (*mcp.CallToolResult, any, error) {
			ctx, rid := withRequestID(ctx)
			log.Printf("[b24-mcp] tool=b24_analyze_tasks_sla req_id=%s", rid)
			logToolArgs("b24_analyze_tasks_sla", args)
			if err := validateSLAArgs(args.Start, args.Limit, args.SoonHoursThreshold); err != nil {
				return nil, nil, err
			}

			result, err := runSLASummary(ctx, client, args.Filter, args.Order, args.Start, args.Limit, args.SoonHoursThreshold, args.IncludeComments)
			if err != nil {
				return nil, nil, err
			}

			logToolResult("b24_analyze_tasks_sla", result)
			return mcpresult.TextAndJSON("b24_analyze_tasks_sla", result), nil, nil
		})
	})

	type workloadArgs struct {
		TaskID          any            `json:"task_id,omitempty" jsonschema:"ID задачи (опционально; при передаче будет выполнен возврат комментариев по задаче для совместимости)"`
		Filter          map[string]any `json:"filter,omitempty" jsonschema:"Фильтр задач для анализа нагрузки"`
		Order           map[string]any `json:"order,omitempty" jsonschema:"Сортировка tasks.task.list"`
		Start           *int           `json:"start,omitempty" jsonschema:"Смещение в tasks.task.list"`
		Limit           *int           `json:"limit,omitempty" jsonschema:"Макс. задач (1..50), по умолчанию 40"`
		IncludeComments *bool          `json:"include_comments,omitempty" jsonschema:"Подтягивать комментарии для оценки блокеров (по умолчанию false)"`
		OverloadTasks   *int           `json:"overload_tasks,omitempty" jsonschema:"Порог перегруза по числу активных задач на responseственного (1..100), по умолчанию 12"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "b24_analyze_tasks_workload",
		Description: "Анализ нагрузки по responseственным: объем, риски, зоны перегруза и рекомендации по выравниванию (read-only)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args workloadArgs) (*mcp.CallToolResult, any, error) {
		return mcpsafe.SafeToolInvoke("mcp-bitrix24", "b24_analyze_tasks_workload", func() (*mcp.CallToolResult, any, error) {
			ctx, rid := withRequestID(ctx)
			log.Printf("[b24-mcp] tool=b24_analyze_tasks_workload req_id=%s", rid)
			logToolArgs("b24_analyze_tasks_workload", args)
			if args.TaskID != nil {
				taskID, err := parsePositiveTaskID(args.TaskID)
				if err != nil {
					return nil, nil, err
				}
				log.Printf("[b24-mcp] tool=b24_analyze_tasks_workload redirect=b24_get_task_comments task_id=%d reason=task_id_argument", taskID)

				commentsResp, err := client.callTaskCommentItemGetList(ctx, taskID, map[string]any{"POST_DATE": "desc"}, nil)
				if err != nil {
					log.Printf("[b24-mcp] tool=b24_analyze_tasks_workload redirect_task_id=%d err=%v", taskID, err)
					return nil, nil, err
				}
				comments := extractComments(commentsResp)
				normalizedComments := normalizeCommentSnapshots(comments, taskID)
				commentsTotal := commentsTotalFromResponse(commentsResp)
				parseWarnings := commentParseWarnings(commentsTotal, len(normalizedComments))

				redirected := map[string]any{
					"result": map[string]any{
						"mode":                    "redirect_to_task_comments_by_task_id",
						"redirected_from_tool":    "b24_analyze_tasks_workload",
						"redirected_to_tool":      "b24_get_task_comments",
						"task_id":                 taskID,
						"comments":                comments,
						"comments_normalized":     normalizedComments,
						"count":                   len(comments),
						"count_normalized":        len(normalizedComments),
						"comments_total":          commentsTotal,
						"comments_parsed":         len(normalizedComments),
						"comments_parse_warnings": parseWarnings,
					},
					"raw": commentsResp,
				}
				logToolResult("b24_analyze_tasks_workload", redirected)
				return mcpresult.TextAndPayload("b24_analyze_tasks_workload", redirected), nil, nil
			}
			if err := validateWorkloadArgs(args.Start, args.Limit, args.OverloadTasks); err != nil {
				return nil, nil, err
			}

			result, err := runWorkloadSummary(ctx, client, args.Filter, args.Order, args.Start, args.Limit, args.IncludeComments, args.OverloadTasks)
			if err != nil {
				return nil, nil, err
			}

			logToolResult("b24_analyze_tasks_workload", result)
			return mcpresult.TextAndJSON("b24_analyze_tasks_workload", result), nil, nil
		})
	})

	type statusTrendsArgs struct {
		Filter     map[string]any `json:"filter,omitempty" jsonschema:"Фильтр задач для трендового анализа"`
		Order      map[string]any `json:"order,omitempty" jsonschema:"Сортировка tasks.task.list"`
		Start      *int           `json:"start,omitempty" jsonschema:"Смещение в tasks.task.list"`
		Limit      *int           `json:"limit,omitempty" jsonschema:"Макс. задач (1..50), по умолчанию 50"`
		PeriodDays *int           `json:"period_days,omitempty" jsonschema:"Период анализа в днях (1..30), по умолчанию 7"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "b24_analyze_tasks_status_trends",
		Description: "Трендовая аналитика по статусам задач за период: open/in-progress/done/deferred (read-only)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args statusTrendsArgs) (*mcp.CallToolResult, any, error) {
		return mcpsafe.SafeToolInvoke("mcp-bitrix24", "b24_analyze_tasks_status_trends", func() (*mcp.CallToolResult, any, error) {
			ctx, rid := withRequestID(ctx)
			log.Printf("[b24-mcp] tool=b24_analyze_tasks_status_trends req_id=%s", rid)
			logToolArgs("b24_analyze_tasks_status_trends", args)
			if err := validateStatusTrendsArgs(args.Start, args.Limit, args.PeriodDays); err != nil {
				return nil, nil, err
			}

			result, err := runStatusTrends(ctx, client, args.Filter, args.Order, args.Start, args.Limit, args.PeriodDays)
			if err != nil {
				return nil, nil, err
			}

			logToolResult("b24_analyze_tasks_status_trends", result)
			return mcpresult.TextAndJSON("b24_analyze_tasks_status_trends", result), nil, nil
		})
	})

	type responsiblePerformanceArgs struct {
		ResponsibleID   string         `json:"responsible_id" jsonschema:"ID responseственного (обязателен)"`
		Filter          map[string]any `json:"filter,omitempty" jsonschema:"Доп. фильтр задач для выборки"`
		Order           map[string]any `json:"order,omitempty" jsonschema:"Сортировка tasks.task.list"`
		Start           *int           `json:"start,omitempty" jsonschema:"Смещение в tasks.task.list"`
		Limit           *int           `json:"limit,omitempty" jsonschema:"Макс. задач (1..50), по умолчанию 30"`
		IncludeComments *bool          `json:"include_comments,omitempty" jsonschema:"Подтягивать комментарии (по умолчанию false)"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "b24_analyze_responsible_performance",
		Description: "Сводка по responseственному: объем, просрочки, high-risk, блокеры (read-only)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args responsiblePerformanceArgs) (*mcp.CallToolResult, any, error) {
		return mcpsafe.SafeToolInvoke("mcp-bitrix24", "b24_analyze_responsible_performance", func() (*mcp.CallToolResult, any, error) {
			ctx, rid := withRequestID(ctx)
			log.Printf("[b24-mcp] tool=b24_analyze_responsible_performance req_id=%s", rid)
			logToolArgs("b24_analyze_responsible_performance", args)
			if err := validateResponsiblePerformanceArgs(args.Start, args.Limit, args.ResponsibleID); err != nil {
				return nil, nil, err
			}

			result, err := runResponsiblePerformance(ctx, client, strings.TrimSpace(args.ResponsibleID), args.Filter, args.Order, args.Start, args.Limit, args.IncludeComments)
			if err != nil {
				return nil, nil, err
			}

			logToolResult("b24_analyze_responsible_performance", result)
			return mcpresult.TextAndJSON("b24_analyze_responsible_performance", result), nil, nil
		})
	})

	type projectHealthArgs struct {
		Filter          map[string]any `json:"filter,omitempty" jsonschema:"Фильтр задач для health-аналитики"`
		Order           map[string]any `json:"order,omitempty" jsonschema:"Сортировка tasks.task.list"`
		Start           *int           `json:"start,omitempty" jsonschema:"Смещение в tasks.task.list"`
		Limit           *int           `json:"limit,omitempty" jsonschema:"Макс. задач (1..50), по умолчанию 30"`
		IncludeComments *bool          `json:"include_comments,omitempty" jsonschema:"Подтягивать комментарии (по умолчанию false)"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "b24_analyze_project_health",
		Description: "Health-score портфеля задач: оценка состояния и драйверов риска (read-only)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args projectHealthArgs) (*mcp.CallToolResult, any, error) {
		return mcpsafe.SafeToolInvoke("mcp-bitrix24", "b24_analyze_project_health", func() (*mcp.CallToolResult, any, error) {
			if !heavyAnalyticsEnabled {
				msg := "b24_analyze_project_health отключен feature-flag `DisableHeavyAnalytics`."
				return mcpresult.Text(msg), nil, nil
			}

			ctx, rid := withRequestID(ctx)
			log.Printf("[b24-mcp] tool=b24_analyze_project_health req_id=%s", rid)
			logToolArgs("b24_analyze_project_health", args)
			if err := validatePortfolioArgs(args.Start, args.Limit, ""); err != nil {
				return nil, nil, err
			}

			result, err := runProjectHealth(ctx, client, args.Filter, args.Order, args.Start, args.Limit, args.IncludeComments)
			if err != nil {
				return nil, nil, err
			}

			logToolResult("b24_analyze_project_health", result)
			return mcpresult.TextAndJSON("b24_analyze_project_health", result), nil, nil
		})
	})

	return srv, nil
}

func extractTask(resp map[string]any) (map[string]any, error) {
	result, ok := resp["result"]
	if !ok {
		return nil, fmt.Errorf("bitrix response has no result field")
	}

	asMap, ok := result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("bitrix result is not an object")
	}

	taskRaw, ok := asMap["task"]
	if !ok {
		return nil, fmt.Errorf("bitrix result.task is отсутствует")
	}

	task, ok := taskRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("bitrix result.task has unexpected type")
	}
	return task, nil
}

func extractComments(resp map[string]any) []map[string]any {
	result, ok := resp["result"]
	if !ok {
		return nil
	}

	switch typed := result.(type) {
	case []any:
		return toMapSlice(typed)
	case map[string]any:
		if comments, ok := typed["comments"]; ok {
			if out := collectCommentsFromAny(comments); len(out) > 0 {
				return out
			}
		}

		if items, ok := typed["items"]; ok {
			if out := collectCommentsFromAny(items); len(out) > 0 {
				return out
			}
		}

		if out := collectCommentsFromAny(typed); len(out) > 0 {
			return out
		}
	}

	return nil
}

func toMapSlice(items []any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		asMap, ok := item.(map[string]any)
		if ok {
			out = append(out, asMap)
		}
	}

	return out
}

func collectCommentsFromAny(v any) []map[string]any {
	switch typed := v.(type) {
	case []any:
		return toMapSlice(typed)
	case map[string]any:
		direct := map[string]any{"AUTHOR_ID": nil, "POST_MESSAGE": nil}
		if hasAnyKey(typed, direct) {
			return []map[string]any{typed}
		}

		out := make([]map[string]any, 0, len(typed))
		for _, vv := range typed {
			switch nested := vv.(type) {
			case map[string]any:
				out = append(out, nested)
			case []any:
				out = append(out, toMapSlice(nested)...)
			}
		}

		return out
	default:
		return nil
	}
}

func commentsTotalFromResponse(resp map[string]any) int {
	result, ok := resp["result"]
	if !ok {
		return 0
	}

	comments := collectCommentsFromAny(result)
	return len(comments)
}

func commentParseWarnings(total, parsed int) []string {
	var warnings []string
	if total > 0 && parsed == 0 {
		warnings = append(warnings, "all_comments_skipped_after_parse")
	}

	if total > parsed {
		warnings = append(warnings, fmt.Sprintf("partial_parse: parsed=%d total=%d", parsed, total))
	}

	return warnings
}

func hasAnyKey(m map[string]any, keys map[string]any) bool {
	for key := range keys {
		if _, ok := m[key]; ok {
			return true
		}
	}

	return false
}

func extractTaskIDFromListFilter(filter map[string]any) (int, bool) {
	if len(filter) == 0 {
		return 0, false
	}

	for _, key := range []string{"ID", "id", "=ID", "=id", "TASK_ID", "task_id", "taskId"} {
		raw, ok := filter[key]
		if !ok {
			continue
		}

		taskID, err := parseTaskID(raw)
		if err != nil || taskID <= 0 {
			return 0, false
		}

		return taskID, true
	}

	return 0, false
}

func parseTaskID(v any) (int, error) {
	switch typed := v.(type) {
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case float64:
		return int(typed), nil
	case string:
		s := strings.TrimSpace(typed)
		if s == "" {
			return 0, fmt.Errorf("task_id is required")
		}

		n, err := strconv.Atoi(s)
		if err != nil {
			return 0, fmt.Errorf("task_id должно быть integer-like, got %q", typed)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("task_id has unsupported type %T", v)
	}
}

var errTaskIDMustBePositive = fmt.Errorf("task_id должно быть > 0")

func parsePositiveTaskID(v any) (int, error) {
	taskID, err := parseTaskID(v)
	if err != nil {
		return 0, err
	}
	if taskID <= 0 {
		return 0, errTaskIDMustBePositive
	}
	return taskID, nil
}

func parseOptionalTaskID(v any) (*int, error) {
	if v == nil {
		return nil, nil
	}

	taskID, err := parseTaskID(v)
	if err != nil {
		return nil, err
	}

	return &taskID, nil
}

func logToolArgs(tool string, args any) {
	raw, err := json.Marshal(args)
	if err != nil {
		log.Printf("[b24-mcp] tool=%s args_marshal_err=%v", tool, err)
		return
	}

	log.Printf("[b24-mcp] tool=%s args=%s", tool, truncateForLog(prettyJSONString(raw), 3000))
}

func logToolResult(tool string, result any) {
	raw, err := json.Marshal(result)
	if err != nil {
		log.Printf("[b24-mcp] tool=%s result_marshal_err=%v", tool, err)
		return
	}

	log.Printf("[b24-mcp] tool=%s result=%s", tool, truncateForLog(prettyJSONString(raw), 3000))
}

type ctxKey string

const requestIDKey ctxKey = "b24_request_id"

var requestSeq uint64

func withRequestID(ctx context.Context) (context.Context, string) {
	if existing, ok := requestIDFromContext(ctx); ok && existing != "" {
		return ctx, existing
	}

	seq := atomic.AddUint64(&requestSeq, 1)
	rid := fmt.Sprintf("b24-%d-%d", time.Now().UnixMilli(), seq)
	return context.WithValue(ctx, requestIDKey, rid), rid
}

func requestIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}

	v := ctx.Value(requestIDKey)
	s, ok := v.(string)
	return s, ok
}

var (
	toolCallsTotal     uint64
	toolErrorsTotal    uint64
	toolLatencyUSum    uint64
	httpCallsTotal     uint64
	httpErrorsTotal    uint64
	softSkipsTotal     uint64
	telemetryStartOnce sync.Once
)

func initTelemetryReporter() {
	telemetryStartOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(60 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				log.Printf(
					"[b24-mcp] metrics tool_calls=%d tool_errors=%d avg_tool_latency_ms=%.2f http_calls=%d http_errors=%d soft_skips=%d",
					atomic.LoadUint64(&toolCallsTotal),
					atomic.LoadUint64(&toolErrorsTotal),
					avgToolLatencyMS(),
					atomic.LoadUint64(&httpCallsTotal),
					atomic.LoadUint64(&httpErrorsTotal),
					atomic.LoadUint64(&softSkipsTotal),
				)
			}
		}()
	})
}

func avgToolLatencyMS() float64 {
	calls := atomic.LoadUint64(&toolCallsTotal)
	if calls == 0 {
		return 0
	}

	return float64(atomic.LoadUint64(&toolLatencyUSum)) / float64(calls) / 1000.0
}

func incHTTPCall() {
	atomic.AddUint64(&httpCallsTotal, 1)
}

func incHTTPError() {
	atomic.AddUint64(&httpErrorsTotal, 1)
}

func incSoftSkip() {
	atomic.AddUint64(&softSkipsTotal, 1)
}

func validateOptionalNonNegativeInt(field string, value *int) error {
	if value == nil {
		return nil
	}

	if *value < 0 {
		return fmt.Errorf("%s должно быть >= 0", field)
	}

	return nil
}

func validateOptionalIntRange(field string, value *int, min, max int) error {
	if value == nil {
		return nil
	}
	if *value < min || *value > max {
		return fmt.Errorf("%s должно быть in range [%d..%d]", field, min, max)
	}
	return nil
}

func validateOptionalEnum(field, value string, allowed ...string) error {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return nil
	}

	for _, item := range allowed {
		if value == strings.ToLower(strings.TrimSpace(item)) {
			return nil
		}
	}

	return fmt.Errorf("%s должно быть one of: %s", field, strings.Join(allowed, ", "))
}

func validateListTasksArgs(start *int, filter map[string]any) error {
	if err := validateOptionalNonNegativeInt("start", start); err != nil {
		return err
	}

	return validateListFilterTaskIDConsistency(filter)
}

func validateListFilterTaskIDConsistency(filter map[string]any) error {
	if len(filter) == 0 {
		return nil
	}

	values := make(map[string]int)
	for _, key := range []string{"ID", "id", "=ID", "=id", "TASK_ID", "task_id", "taskId"} {
		raw, ok := filter[key]
		if !ok {
			continue
		}

		taskID, err := parseTaskID(raw)
		if err != nil || taskID <= 0 {
			continue
		}
		values[key] = taskID
	}

	if len(values) <= 1 {
		return nil
	}

	var expected int
	for _, v := range values {
		expected = v
		break
	}

	for key, v := range values {
		if v != expected {
			return fmt.Errorf("conflicting task id filters: %q=%d conflicts with other id fields", key, v)
		}
	}

	return nil
}

func validateAnalyzeTasksByQueryArgs(start, limit *int) error {
	if err := validateOptionalNonNegativeInt("start", start); err != nil {
		return err
	}

	return validateOptionalIntRange("limit", limit, 1, 50)
}

func validatePortfolioArgs(start, limit *int, groupBy string) error {
	if err := validateOptionalNonNegativeInt("start", start); err != nil {
		return err
	}

	if err := validateOptionalIntRange("limit", limit, 1, 50); err != nil {
		return err
	}

	return validateOptionalEnum("group_by", groupBy, "responsible", "creator", "status")
}

func validateExecutiveSummaryArgs(start, limit, periodDays *int) error {
	if err := validateOptionalNonNegativeInt("start", start); err != nil {
		return err
	}

	if err := validateOptionalIntRange("limit", limit, 1, 50); err != nil {
		return err
	}

	return validateOptionalIntRange("period_days", periodDays, 1, 30)
}

func validateSLAArgs(start, limit, soonHoursThreshold *int) error {
	if err := validateOptionalNonNegativeInt("start", start); err != nil {
		return err
	}

	if err := validateOptionalIntRange("limit", limit, 1, 50); err != nil {
		return err
	}

	return validateOptionalIntRange("soon_hours_threshold", soonHoursThreshold, 1, 168)
}

func validateWorkloadArgs(start, limit, overloadTasks *int) error {
	if err := validateOptionalNonNegativeInt("start", start); err != nil {
		return err
	}

	if err := validateOptionalIntRange("limit", limit, 1, 50); err != nil {
		return err
	}

	return validateOptionalIntRange("overload_tasks", overloadTasks, 1, 100)
}

func validateStatusTrendsArgs(start, limit, periodDays *int) error {
	if err := validateOptionalNonNegativeInt("start", start); err != nil {
		return err
	}

	if err := validateOptionalIntRange("limit", limit, 1, 50); err != nil {
		return err
	}

	return validateOptionalIntRange("period_days", periodDays, 1, 30)
}

func validateResponsiblePerformanceArgs(start, limit *int, responsibleID string) error {
	if err := validateOptionalNonNegativeInt("start", start); err != nil {
		return err
	}

	if err := validateOptionalIntRange("limit", limit, 1, 50); err != nil {
		return err
	}

	if strings.TrimSpace(responsibleID) == "" {
		return fmt.Errorf("responsible_id is required")
	}

	return nil
}
