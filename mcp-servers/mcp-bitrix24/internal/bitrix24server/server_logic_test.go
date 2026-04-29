package bitrix24server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/magomedcoder/gen/mcp-servers/mcp-bitrix24/internal/bitrix24mock"
	"golang.org/x/text/encoding/charmap"
)

func TestExtractTaskIDFromListFilter(t *testing.T) {
	taskID, ok := extractTaskIDFromListFilter(map[string]any{"ID": "100"})
	if !ok || taskID != 100 {
		t.Fatalf("expected task id 100, got ok=%v id=%d", ok, taskID)
	}

	taskID, ok = extractTaskIDFromListFilter(map[string]any{"=ID": 42})
	if !ok || taskID != 42 {
		t.Fatalf("expected task id 42 from =ID, got ok=%v id=%d", ok, taskID)
	}

	taskID, ok = extractTaskIDFromListFilter(map[string]any{"taskId": " 777 "})
	if !ok || taskID != 777 {
		t.Fatalf("expected task id 777 from taskId, got ok=%v id=%d", ok, taskID)
	}

	if _, ok := extractTaskIDFromListFilter(map[string]any{"ID": "abc"}); ok {
		t.Fatalf("did not expect task-id detection for invalid ID value")
	}

	if _, ok := extractTaskIDFromListFilter(map[string]any{"RESPONSIBLE_ID": 7}); ok {
		t.Fatalf("did not expect task-id detection for non-id filter")
	}
}

func TestExtractComments_FromMapPayloadShapes(t *testing.T) {
	respWithItemsMap := map[string]any{
		"result": map[string]any{
			"items": map[string]any{
				"9001": map[string]any{
					"ID":           "9001",
					"AUTHOR_ID":    "17",
					"POST_MESSAGE": "msg1",
				},
			},
		},
	}
	comments := extractComments(respWithItemsMap)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment from items map, got %d", len(comments))
	}

	respWithCommentsObject := map[string]any{
		"result": map[string]any{
			"comments": map[string]any{
				"ID":           "9002",
				"AUTHOR_ID":    "21",
				"POST_MESSAGE": "msg2",
			},
		},
	}
	comments = extractComments(respWithCommentsObject)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment from single comment object, got %d", len(comments))
	}

	respWithCommentsMapByID := map[string]any{
		"result": map[string]any{
			"comments": map[string]any{
				"9003": map[string]any{
					"ID":           "9003",
					"AUTHOR_ID":    "5",
					"POST_MESSAGE": "msg3",
				},
				"9004": map[string]any{
					"ID":           "9004",
					"AUTHOR_ID":    "8",
					"POST_MESSAGE": "msg4",
				},
			},
		},
	}
	comments = extractComments(respWithCommentsMapByID)
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments from comments map by id, got %d", len(comments))
	}
}

func TestExtractComments_FromNestedArraysInsideMap(t *testing.T) {
	resp := map[string]any{
		"result": map[string]any{
			"comments": map[string]any{
				"group_a": []any{
					map[string]any{"ID": "1", "AUTHOR_ID": "7", "POST_MESSAGE": "m1"},
				},
				"group_b": []any{
					map[string]any{"ID": "2", "AUTHOR_ID": "8", "POST_MESSAGE": "m2"},
				},
			},
		},
	}

	comments := extractComments(resp)
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments from nested arrays in map, got %d", len(comments))
	}
}

func TestCommentDiagnostics(t *testing.T) {
	resp := map[string]any{
		"result": map[string]any{
			"comments": map[string]any{
				"1": map[string]any{"ID": "1", "AUTHOR_ID": "7", "POST_MESSAGE": "ok"},
			},
		},
	}

	total := commentsTotalFromResponse(resp)
	if total != 1 {
		t.Fatalf("expected total=1, got %d", total)
	}

	warns := commentParseWarnings(2, 1)
	if len(warns) == 0 {
		t.Fatalf("expected warnings for partial parse")
	}
}

func TestValidateAnalyzeToolsArgs(t *testing.T) {
	tests := []struct {
		name    string
		run     func() error
		wantErr bool
	}{
		{"list-valid", func() error {
			return validateListTasksArgs(new(0), nil)
		}, false},
		{"list-invalid", func() error {
			return validateListTasksArgs(new(-1), nil)
		}, true},
		{"list-valid-consistent-task-id-filter", func() error {
			return validateListTasksArgs(new(0), map[string]any{"ID": "123", "taskId": 123})
		}, false},
		{"list-invalid-conflicting-task-id-filter", func() error {
			return validateListTasksArgs(new(0), map[string]any{"ID": "123", "taskId": 124})
		}, true},
		{"query-valid", func() error {
			return validateAnalyzeTasksByQueryArgs(new(0), new(20))
		}, false},
		{"query-invalid-limit", func() error {
			return validateAnalyzeTasksByQueryArgs(new(0), new(51))
		}, true},
		{"portfolio-valid", func() error {
			return validatePortfolioArgs(new(0), new(30), "responsible")
		}, false},
		{"portfolio-invalid-group", func() error {
			return validatePortfolioArgs(new(0), new(30), "team")
		}, true},
		{"exec-valid", func() error {
			return validateExecutiveSummaryArgs(new(0), new(40), new(7))
		}, false},
		{"exec-invalid-period", func() error {
			return validateExecutiveSummaryArgs(new(0), new(40), new(31))
		}, true},
		{"sla-valid", func() error {
			return validateSLAArgs(new(0), new(40), new(24))
		}, false},
		{"sla-invalid-threshold", func() error {
			return validateSLAArgs(new(0), new(40), new(169))
		}, true},
		{"workload-valid", func() error {
			return validateWorkloadArgs(new(0), new(40), new(12))
		}, false},
		{"workload-invalid-overload", func() error {
			return validateWorkloadArgs(new(0), new(40), new(101))
		}, true},
		{"status-valid", func() error {
			return validateStatusTrendsArgs(new(0), new(50), new(7))
		}, false},
		{"status-invalid-limit", func() error {
			return validateStatusTrendsArgs(new(0), new(0), new(7))
		}, true},
		{"responsible-performance-valid", func() error {
			return validateResponsiblePerformanceArgs(new(0), new(20), "21")
		}, false},
		{"responsible-performance-invalid-empty", func() error {
			return validateResponsiblePerformanceArgs(new(0), new(20), " ")
		}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if tc.wantErr && err == nil {
				t.Fatalf("expected error")
			}

			if !tc.wantErr && err != nil {
				t.Fatalf("неожиданный error: %v", err)
			}
		})
	}
}

func TestValidateOptionalNonNegativeInt(t *testing.T) {
	neg := -1
	if err := validateOptionalNonNegativeInt("start", &neg); err == nil {
		t.Fatalf("expected error for negative value")
	}

	zero := 0
	if err := validateOptionalNonNegativeInt("start", &zero); err != nil {
		t.Fatalf("неожиданный error for zero: %v", err)
	}

	if err := validateOptionalNonNegativeInt("start", nil); err != nil {
		t.Fatalf("неожиданный error for nil: %v", err)
	}
}

func TestValidateOptionalIntRange(t *testing.T) {
	inside := 10
	if err := validateOptionalIntRange("limit", &inside, 1, 50); err != nil {
		t.Fatalf("неожиданный error for inside value: %v", err)
	}

	tooLow := 0
	if err := validateOptionalIntRange("limit", &tooLow, 1, 50); err == nil {
		t.Fatalf("expected error for low value")
	}

	tooHigh := 51
	if err := validateOptionalIntRange("limit", &tooHigh, 1, 50); err == nil {
		t.Fatalf("expected error for high value")
	}
}

func TestValidateOptionalEnum(t *testing.T) {
	if err := validateOptionalEnum("group_by", "responsible", "responsible", "creator", "status"); err != nil {
		t.Fatalf("неожиданный error for allowed enum: %v", err)
	}

	if err := validateOptionalEnum("group_by", "STATUS", "responsible", "creator", "status"); err != nil {
		t.Fatalf("неожиданный error for case-insensitive enum: %v", err)
	}

	if err := validateOptionalEnum("group_by", "", "responsible", "creator", "status"); err != nil {
		t.Fatalf("неожиданный error for empty enum: %v", err)
	}

	if err := validateOptionalEnum("group_by", "team", "responsible", "creator", "status"); err == nil {
		t.Fatalf("expected error for unknown enum")
	}
}

func TestWithRequestID_AssignsAndReuses(t *testing.T) {
	ctx := context.Background()
	ctx1, id1 := withRequestID(ctx)
	if id1 == "" {
		t.Fatalf("expected non-empty request id")
	}

	got, ok := requestIDFromContext(ctx1)
	if !ok || got == "" {
		t.Fatalf("expected request id in context")
	}

	if got != id1 {
		t.Fatalf("неожиданный id from context: %q != %q", got, id1)
	}

	ctx2, id2 := withRequestID(ctx1)
	if id2 != id1 {
		t.Fatalf("expected request id reuse, got %q want %q", id2, id1)
	}

	if ctx2 != ctx1 {
		t.Fatalf("expected same context when id already exists")
	}
}

func TestMaskJSONForLog_MasksSensitiveFields(t *testing.T) {
	raw := []byte(`{"auth":"abc","nested":{"token":"secret","ok":"value"},"password":"123"}`)
	masked := string(maskJSONForLog(raw))
	if strings.Contains(masked, `"abc"`) || strings.Contains(masked, `"secret"`) || strings.Contains(masked, `"123"`) {
		t.Fatalf("sensitive values were not masked: %s", masked)
	}

	if !strings.Contains(masked, "***MASKED***") {
		t.Fatalf("expected masked marker in output: %s", masked)
	}
}

func TestBitrixErrorHint_TaskCommentActionFailed(t *testing.T) {
	hint := bitrixErrorHint("task.commentitem.getlist", "ERROR_CORE", "TASKS_ERROR_EXCEPTION_#8; Action failed", 400)
	if !strings.Contains(strings.ToLower(hint), "include_comments") {
		t.Fatalf("expected include_comments hint, got: %s", hint)
	}
}

func TestWrapBitrixError_ContainsMethodAndHint(t *testing.T) {
	raw := []byte(`{"error":"ERROR_CORE","error_description":"TASKS_ERROR_EXCEPTION_#256; expected to be of type integer"}`)
	err := wrapBitrixError("task.commentitem.getlist", 400, raw, nil)
	if err == nil {
		t.Fatalf("expected error")
	}

	msg := err.Error()
	if !strings.Contains(msg, "task.commentitem.getlist") {
		t.Fatalf("expected method name in error, got: %s", msg)
	}

	if !strings.Contains(strings.ToLower(msg), "проверьте типы") {
		t.Fatalf("expected actionable hint in error, got: %s", msg)
	}
}

func TestBuildAnalyticsContextForTaskList_WithMock(t *testing.T) {
	mock := bitrix24mock.NewServer()
	srv := httptest.NewServer(mock.Handler())
	defer srv.Close()

	client, err := newBitrixClient(srv.URL, 2*time.Second, "info", 0, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("newBitrixClient: %v", err)
	}

	ac, err := buildAnalyticsContextForTaskList(context.Background(), client, nil, nil, nil, 10, false)
	if err != nil {
		t.Fatalf("buildAnalyticsContextForTaskList: %v", err)
	}

	if ac == nil || len(ac.Items) == 0 {
		t.Fatalf("expected non-empty analytics context")
	}
}

func TestDetectBlockerSignals_FindsKeywordsAndSortsByAge(t *testing.T) {
	now := time.Date(2026, 4, 28, 15, 0, 0, 0, time.UTC)
	comments := []CommentSnapshot{
		{
			ID:        1,
			TaskID:    1001,
			AuthorID:  "7",
			CreatedAt: "2026-04-28T14:00:00Z",
			Message:   "Жду response от клиента",
		},
		{
			ID:        2,
			TaskID:    1001,
			AuthorID:  "8",
			CreatedAt: "2026-04-26T10:00:00Z",
			Message:   "Blocked by dependency",
		},
		{
			ID:        3,
			TaskID:    1001,
			AuthorID:  "9",
			CreatedAt: "2026-04-28T13:00:00Z",
			Message:   "обычный апдейт",
		},
	}

	signals := detectBlockerSignals(comments, now)
	if len(signals) != 2 {
		t.Fatalf("expected 2 blocker signals, got %d", len(signals))
	}

	if signals[0].CommentID != 2 {
		t.Fatalf("expected oldest blocker first, got comment_id=%d", signals[0].CommentID)
	}
}

func TestBuildExecutionDriftReport_HighDriftOnOverrunAndSilence(t *testing.T) {
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	task := TaskSnapshot{ID: 1001, TimeEstimate: 100, TimeSpent: 150}
	comments := []CommentSnapshot{
		{
			CreatedAt: "2026-04-20T12:00:00Z",
			Message:   "old",
		},
	}
	report := buildExecutionDriftReport(task, comments, now)
	if report.DriftLevel != "high" || report.OverrunSeconds <= 0 {
		t.Fatalf("неожиданный report: %+v", report)
	}
}

func TestNormalizeTaskSnapshot(t *testing.T) {
	task := map[string]any{"ID": "1001", "TITLE": "Demo task", "STATUS": "3", "RESPONSIBLE_ID": "21", "TIME_ESTIMATE": "3600", "TIME_SPENT_IN_LOGS": "1200"}
	s := normalizeTaskSnapshot(task)
	if s.ID != 1001 || s.Title != "Demo task" || s.ResponsibleID != "21" {
		t.Fatalf("неожиданный snapshot: %+v", s)
	}
}

func TestBuildTaskTimeline_SortsByDateDesc(t *testing.T) {
	task := TaskSnapshot{ID: 1001, Title: "Demo", StatusCode: 3, StatusLabel: "Выполняется", CreatedAt: "2026-04-28T10:00:00+03:00", ChangedAt: "2026-04-28T12:00:00+03:00"}
	comments := []CommentSnapshot{
		{
			ID:        1,
			TaskID:    1001,
			CreatedAt: "2026-04-28T13:00:00+03:00",
			Message:   "latest",
		},
		{
			ID:        2,
			TaskID:    1001,
			CreatedAt: "2026-04-28T11:00:00+03:00",
			Message:   "old",
		},
	}
	events := buildTaskTimeline(task, comments)
	if len(events) < 3 || events[0].Details != "latest" {
		t.Fatalf("неожиданный first event: %+v", events)
	}
}

func TestRunAnalyticsQuery_CommentErrorDoesNotFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/tasks.task.list"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":{"tasks":[{"ID":"101","TITLE":"Тестовая задача","STATUS":"3","CHANGED_DATE":"2026-04-27T10:00:00+00:00"}]}}`))
		case strings.HasSuffix(r.URL.Path, "/task.commentitem.getlist"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"ERROR_CORE","error_description":"TASKS_ERROR_EXCEPTION_#8; Action failed"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"NOT_FOUND"}`))
		}
	}))
	defer srv.Close()

	client, err := newBitrixClient(srv.URL, time.Second, "info", 0, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("newBitrixClient: %v", err)
	}

	includeComments := true
	limit := 20
	result, err := runAnalyticsQuery(context.Background(), client, "общий аналитический обзор", nil, nil, nil, nil, &limit, &includeComments)
	if err != nil {
		t.Fatalf("runAnalyticsQuery returned error: %v", err)
	}

	if !strings.Contains(result, "Найдено задач: 1") {
		t.Fatalf("неожиданный analytics output: %s", result)
	}
}

func TestLoadTaskList_ByIDUsesTaskGet(t *testing.T) {
	var listCalls, getCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/tasks.task.get"):
			getCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":{"task":{"ID":"101","TITLE":"Точечная задача","STATUS":"3"}}}`))
		case strings.HasSuffix(r.URL.Path, "/tasks.task.list"):
			listCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":{"tasks":[{"ID":"999","TITLE":"Лишняя задача","STATUS":"2"}]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"NOT_FOUND"}`))
		}
	}))
	defer srv.Close()

	client, err := newBitrixClient(srv.URL, time.Second, "info", 0, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("newBitrixClient: %v", err)
	}

	tasks, err := loadTaskList(context.Background(), client, map[string]any{"task_id": "101"}, nil, nil, 20)
	if err != nil {
		t.Fatalf("loadTaskList: %v", err)
	}

	if len(tasks) != 1 {
		t.Fatalf("expected exactly one task, got %d", len(tasks))
	}

	if numberLike(field(tasks[0], "ID", "id")) != 101 {
		t.Fatalf("unexpected task payload: %+v", tasks[0])
	}

	if getCalls != 1 {
		t.Fatalf("expected tasks.task.get call, got %d", getCalls)
	}

	if listCalls != 0 {
		t.Fatalf("expected no tasks.task.list call, got %d", listCalls)
	}
}

func TestBuildAnalyticsContextForTaskList_ByIDUsesTaskGet(t *testing.T) {
	var listCalls, getCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/tasks.task.get"):
			getCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":{"task":{"ID":"202","TITLE":"Контекстная задача","STATUS":"2"}}}`))
		case strings.HasSuffix(r.URL.Path, "/tasks.task.list"):
			listCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":{"tasks":[{"ID":"303","TITLE":"Лишний список","STATUS":"3"}]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"NOT_FOUND"}`))
		}
	}))
	defer srv.Close()

	client, err := newBitrixClient(srv.URL, time.Second, "info", 0, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("newBitrixClient: %v", err)
	}

	ac, err := buildAnalyticsContextForTaskList(context.Background(), client, map[string]any{"TASK_ID": 202}, nil, nil, 10, false)
	if err != nil {
		t.Fatalf("buildAnalyticsContextForTaskList: %v", err)
	}

	if ac == nil || len(ac.Items) != 1 || ac.Items[0].TaskID != 202 {
		t.Fatalf("unexpected analytics context: %+v", ac)
	}
	if getCalls != 1 {
		t.Fatalf("expected tasks.task.get call, got %d", getCalls)
	}
	if listCalls != 0 {
		t.Fatalf("expected no tasks.task.list call, got %d", listCalls)
	}
}

func TestBitrixClientCall_DecodesWindows1251JSON(t *testing.T) {
	cp1251Body, err := charmap.Windows1251.NewEncoder().Bytes([]byte(`{"result":{"task":{"TITLE":"Привет"}}}`))
	if err != nil {
		t.Fatalf("encode cp1251: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=windows-1251")
		_, _ = w.Write(cp1251Body)
	}))
	defer srv.Close()

	client, err := newBitrixClient(srv.URL, time.Second, "debug", 0, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("newBitrixClient: %v", err)
	}

	resp, err := client.call(context.Background(), "tasks.task.get", map[string]any{"taskId": 1001})
	if err != nil || nestedTaskTitle(resp) != "Привет" {
		t.Fatalf("неожиданный response: err=%v title=%q", err, nestedTaskTitle(resp))
	}
}

func nestedTaskTitle(resp map[string]any) string {
	result, _ := resp["result"].(map[string]any)
	task, _ := result["task"].(map[string]any)
	title, _ := task["TITLE"].(string)
	return title
}

func TestCallTaskCommentItemGetList_UsesStablePayloadOrder(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[]}`))
	}))
	defer srv.Close()

	client, err := newBitrixClient(srv.URL, time.Second, "debug", 0, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("newBitrixClient: %v", err)
	}

	_, err = client.callTaskCommentItemGetList(context.Background(), 1001, map[string]any{"POST_DATE": "desc"}, map[string]any{"AUTHOR_ID": 7})
	if err != nil {
		t.Fatalf("callTaskCommentItemGetList: %v", err)
	}

	idxTask := strings.Index(gotBody, `"TASKID"`)
	idxOrder := strings.Index(gotBody, `"ORDER"`)
	idxFilter := strings.Index(gotBody, `"FILTER"`)
	if !(idxTask != -1 && idxOrder != -1 && idxFilter != -1 && idxTask < idxOrder && idxOrder < idxFilter) {
		t.Fatalf("неожиданный key order: %s", gotBody)
	}
}

func TestBitrixClientCall_BlocksWriteMethodsByReadOnlyPolicy(t *testing.T) {
	var hitCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hitCount, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"ok":true}}`))
	}))
	defer srv.Close()

	client, err := newBitrixClient(srv.URL, time.Second, "debug", 0, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("newBitrixClient: %v", err)
	}

	_, err = client.call(context.Background(), "tasks.task.update", map[string]any{"taskId": 1001})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "read-only policy") {
		t.Fatalf("expected read-only policy error, got: %v", err)
	}

	if atomic.LoadInt32(&hitCount) != 0 {
		t.Fatalf("write method должно быть blocked before HTTP call")
	}
}

func TestContract_LoadTaskList_WithMockServer(t *testing.T) {
	mock := bitrix24mock.NewServer()
	srv := httptest.NewServer(mock.Handler())
	defer srv.Close()

	client, err := newBitrixClient(srv.URL, 2*time.Second, "info", 0, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("newBitrixClient: %v", err)
	}

	tasks, err := loadTaskList(context.Background(), client, nil, nil, nil, 50)
	if err != nil || len(tasks) == 0 {
		t.Fatalf("loadTaskList не удалось: err=%v len=%d", err, len(tasks))
	}
}

func TestContract_LoadTaskComments_WithMockServer(t *testing.T) {
	mock := bitrix24mock.NewServer()
	srv := httptest.NewServer(mock.Handler())
	defer srv.Close()

	client, err := newBitrixClient(srv.URL, 2*time.Second, "info", 0, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("newBitrixClient: %v", err)
	}

	comments, err := loadTaskComments(context.Background(), client, 1001)
	if err != nil || len(comments) == 0 {
		t.Fatalf("loadTaskComments не удалось: err=%v len=%d", err, len(comments))
	}
}

func TestContract_RunExecutiveSummary_WithMockServer(t *testing.T) {
	mock := bitrix24mock.NewServer()
	srv := httptest.NewServer(mock.Handler())
	defer srv.Close()

	client, err := newBitrixClient(srv.URL, 2*time.Second, "info", 0, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("newBitrixClient: %v", err)
	}

	report, err := runExecutiveSummary(context.Background(), client, nil, nil, nil, nil, nil, nil)
	if err != nil || !strings.Contains(report, "Executive summary") || !strings.Contains(report, "=== Вывод ===") {
		t.Fatalf("неожиданный report: err=%v report=%s", err, report)
	}
}

func TestRunProjectHealth_WithMockServer(t *testing.T) {
	mock := bitrix24mock.NewServer()
	srv := httptest.NewServer(mock.Handler())
	defer srv.Close()

	client, err := newBitrixClient(srv.URL, 2*time.Second, "info", 0, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("newBitrixClient: %v", err)
	}

	report, err := runProjectHealth(context.Background(), client, nil, nil, nil, nil, nil)
	if err != nil || !strings.Contains(report, "Project health summary") || !strings.Contains(report, "Health score:") {
		t.Fatalf("неожиданный report: err=%v report=%s", err, report)
	}
}

func TestRunResponsiblePerformance_WithMockServer(t *testing.T) {
	mock := bitrix24mock.NewServer()
	srv := httptest.NewServer(mock.Handler())
	defer srv.Close()

	client, err := newBitrixClient(srv.URL, 2*time.Second, "info", 0, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("newBitrixClient: %v", err)
	}

	report, err := runResponsiblePerformance(context.Background(), client, "21", nil, nil, nil, nil, nil)
	if err != nil || !strings.Contains(report, "Performance по responseственному 21") || !strings.Contains(report, "=== Вывод ===") {
		t.Fatalf("неожиданный report: err=%v report=%s", err, report)
	}
}

func TestRunResponsiblePerformance_DoesNotMutateInputFilter(t *testing.T) {
	mock := bitrix24mock.NewServer()
	srv := httptest.NewServer(mock.Handler())
	defer srv.Close()

	client, err := newBitrixClient(srv.URL, 2*time.Second, "info", 0, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("newBitrixClient: %v", err)
	}

	inputFilter := map[string]any{"STATUS": 3}
	_, err = runResponsiblePerformance(context.Background(), client, "21", inputFilter, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("runResponsiblePerformance: %v", err)
	}

	if _, exists := inputFilter["RESPONSIBLE_ID"]; exists {
		t.Fatalf("input filter must not be mutated, got: %#v", inputFilter)
	}
}
