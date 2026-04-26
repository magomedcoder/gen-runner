package bitrix24mock

import (
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	tasks    []map[string]any
	comments map[int][]map[string]any
}

func NewServer() *Server {
	now := time.Now().UTC()
	task1 := map[string]any{
		"ID":                   "1001",
		"TITLE":                "Подготовить коммерческое предложение",
		"CREATED_DATE":         now.Add(-48 * time.Hour).Format(time.RFC3339),
		"DEADLINE":             now.Add(24 * time.Hour).Format(time.RFC3339),
		"STATUS":               "3",
		"CREATED_BY":           "17",
		"RESPONSIBLE_ID":       "21",
		"UF_AUTO_428428258665": "CN-0000-000",
		"UF_AUTO_123456789012": "DEMO",
	}
	task2 := map[string]any{
		"ID":                   "1002",
		"TITLE":                "Согласовать план внедрения",
		"CREATED_DATE":         now.Add(-96 * time.Hour).Format(time.RFC3339),
		"DEADLINE":             now.Add(-10 * time.Hour).Format(time.RFC3339),
		"STATUS":               "2",
		"CREATED_BY":           "18",
		"RESPONSIBLE_ID":       "22",
		"UF_AUTO_428428258665": "CN-0000-000",
		"UF_AUTO_123456789012": "DEMO",
	}

	return &Server{
		tasks: []map[string]any{task1, task2},
		comments: map[int][]map[string]any{
			1001: {
				{
					"ID":           "9001",
					"TASK_ID":      "1001",
					"POST_DATE":    now.Add(-36 * time.Hour).Format(time.RFC3339),
					"AUTHOR_ID":    "17",
					"POST_MESSAGE": "Черновик КП подготовлен, жду проверку.",
				},
				{
					"ID":           "9002",
					"TASK_ID":      "1001",
					"POST_DATE":    now.Add(-12 * time.Hour).Format(time.RFC3339),
					"AUTHOR_ID":    "21",
					"POST_MESSAGE": "Проверил, добавьте блок по SLA.",
				},
			},
			1002: {
				{
					"ID":           "9003",
					"TASK_ID":      "1002",
					"POST_DATE":    now.Add(-60 * time.Hour).Format(time.RFC3339),
					"AUTHOR_ID":    "18",
					"POST_MESSAGE": "Ожидаем финальный апдейт от заказчика.",
				},
			},
		},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleMethod)
	return mux
}

func (s *Server) handleMethod(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[b24-mock] panic recovered: %v", rec)
			writeError(w, http.StatusInternalServerError, "internal mock server error")
		}
	}()

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST is supported")
		return
	}

	method := extractMethod(r.URL.Path)
	if method == "" {
		writeError(w, http.StatusBadRequest, "method is empty in URL path")
		return
	}
	log.Printf("[b24-mock] request method=%q path=%s remote=%s", method, r.URL.Path, r.RemoteAddr)

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[b24-mock] method=%q invalid_json err=%v", method, err)
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	switch method {
	case "tasks.task.list":
		s.handleTaskList(w, req)
	case "tasks.task.get":
		s.handleTaskGet(w, req)
	case "tasks.task.commentitem.getlist":
		s.handleCommentList(w, req)
	default:
		log.Printf("[b24-mock] method=%q not_implemented", method)
		writeJSON(w, http.StatusOK, map[string]any{
			"error":             "ERROR_METHOD_NOT_FOUND",
			"error_description": fmt.Sprintf("mock: method %s is not implemented", method),
		})
	}
}

func (s *Server) handleTaskList(w http.ResponseWriter, req map[string]any) {
	filter, _ := req["filter"].(map[string]any)
	selectFields := toStringSlice(req["select"])
	order, _ := req["order"].(map[string]any)

	filtered := make([]map[string]any, 0, len(s.tasks))
	for _, task := range s.tasks {
		if matchesFilter(task, filter) {
			filtered = append(filtered, cloneMap(task))
		}
	}

	applyOrder(filtered, order)
	for i := range filtered {
		filtered[i] = selectTaskFields(filtered[i], selectFields)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"result": map[string]any{
			"tasks": filtered,
			"total": strconv.Itoa(len(filtered)),
		},
	})
	log.Printf("[b24-mock] tasks.task.list ok total=%d", len(filtered))
}

func (s *Server) handleTaskGet(w http.ResponseWriter, req map[string]any) {
	taskID := toInt(req["taskId"])
	if taskID <= 0 {
		log.Printf("[b24-mock] tasks.task.get missing taskId")
		writeJSON(w, http.StatusOK, map[string]any{
			"error":             "ERROR_REQUIRED_PARAMETER",
			"error_description": "taskId is required",
		})
		return
	}

	selectFields := toStringSlice(req["select"])
	for _, task := range s.tasks {
		if toInt(task["ID"]) == taskID {
			log.Printf("[b24-mock] tasks.task.get ok taskId=%d", taskID)
			writeJSON(w, http.StatusOK, map[string]any{
				"result": map[string]any{
					"task": selectTaskFields(cloneMap(task), selectFields),
				},
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"error":             "ERROR_TASK_NOT_FOUND",
		"error_description": "task not found in mock storage",
	})
	log.Printf("[b24-mock] tasks.task.get not_found taskId=%d", taskID)
}

func (s *Server) handleCommentList(w http.ResponseWriter, req map[string]any) {
	taskID := toInt(req["taskId"])
	if taskID <= 0 {
		log.Printf("[b24-mock] tasks.task.commentitem.getlist missing taskId")
		writeJSON(w, http.StatusOK, map[string]any{
			"error":             "ERROR_REQUIRED_PARAMETER",
			"error_description": "taskId is required",
		})
		return
	}

	order, _ := req["order"].(map[string]any)
	selectFields := toStringSlice(req["select"])

	items := make([]map[string]any, 0, len(s.comments[taskID]))
	for _, comment := range s.comments[taskID] {
		items = append(items, cloneMap(comment))
	}

	applyOrder(items, order)
	for i := range items {
		items[i] = selectTaskFields(items[i], selectFields)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"result": items,
	})
	log.Printf("[b24-mock] tasks.task.commentitem.getlist ok taskId=%d total=%d", taskID, len(items))
}

func extractMethod(path string) string {
	path = strings.Trim(path, "/")
	if path == "" {
		return ""
	}

	parts := strings.Split(path, "/")
	method := parts[len(parts)-1]
	method = strings.TrimSuffix(method, ".json")
	return method
}

func matchesFilter(task map[string]any, filter map[string]any) bool {
	if len(filter) == 0 {
		return true
	}
	for key, expected := range filter {
		actual, ok := task[key]
		if !ok {
			return false
		}
		if fmt.Sprint(actual) != fmt.Sprint(expected) {
			return false
		}
	}

	return true
}

func applyOrder(items []map[string]any, order map[string]any) {
	if len(items) <= 1 || len(order) == 0 {
		return
	}
	var field string
	var direction string
	for k, v := range order {
		field = strings.TrimSpace(k)
		direction = strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
		break
	}
	if field == "" {
		return
	}
	desc := direction == "desc"

	sort.SliceStable(items, func(i, j int) bool {
		ai := fmt.Sprint(items[i][field])
		aj := fmt.Sprint(items[j][field])
		if desc {
			return ai > aj
		}

		return ai < aj
	})
}

func selectTaskFields(item map[string]any, fields []string) map[string]any {
	if len(fields) == 0 {
		return item
	}

	selected := make(map[string]any, len(fields))
	for _, field := range fields {
		if v, ok := item[field]; ok {
			selected[field] = v
		}
	}

	return selected
}

func toStringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}

	out := make([]string, 0, len(raw))
	for _, item := range raw {
		out = append(out, fmt.Sprint(item))
	}

	return out
}

func toInt(v any) int {
	switch typed := v.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(typed))
		return n
	default:
		return 0
	}
}

func cloneMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	maps.Copy(dst, src)

	return dst
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error":             "MOCK_HTTP_ERROR",
		"error_description": message,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
