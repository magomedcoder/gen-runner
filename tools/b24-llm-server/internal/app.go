package internal

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/service"
)

type App struct {
	llm        *service.LLMRunnerService
	model      string
	runnerAddr string
	host       string
}

func NewFromEnv() (*App, error) {
	runnerAddr := strings.TrimSpace(os.Getenv("GEN_RUNNER_ADDR"))
	model := strings.TrimSpace(os.Getenv("GEN_MODEL"))
	host := strings.TrimSpace(os.Getenv("B24_LLM_SERVER_HOST"))

	if runnerAddr == "" {
		runnerAddr = "0.0.0.0:50052"
	}

	if model == "" {
		model = "Qwen3-8B-Q8_0"
	}
	
	if host == "" {
		host = "127.0.0.1:8001"
	}

	llmSvc, err := service.NewLLMRunnerService(runnerAddr, model)
	if err != nil {
		return nil, fmt.Errorf("инициализация llm runner: %w", err)
	}

	return &App{
		llm:        llmSvc,
		model:      model,
		runnerAddr: runnerAddr,
		host:       host,
	}, nil
}

func (a *App) Close() {
	if a == nil || a.llm == nil {
		return
	}
	_ = a.llm.Close()
}

func (a *App) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/b24/v1/health", a.handleHealth)
	mux.HandleFunc("/b24/v1/analyze", a.handleAnalyze)

	log.Printf("b24-llm-server запущен на %s, runner=%s, model=%s", a.host, a.runnerAddr, a.model)
	return http.ListenAndServe(a.host, mux)
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}

	ok, err := a.llm.CheckConnection(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": ok})
}

func (a *App) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}

	var req AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}

	if strings.TrimSpace(req.Prompt) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt_required"})
		return
	}

	messages := []*domain.Message{
		{
			Role: domain.MessageRoleSystem,
			Content: strings.TrimSpace(
				"Ты аналитик задач Bitrix24. Отвечай структурировано и по делу. Используй контекст задачи и комментариев, отмечай риски, шаги и уточняющие вопросы.",
			),
		},
		{
			Role:    domain.MessageRoleUser,
			Content: buildTaskContext(req),
		},
	}

	for _, m := range req.History {
		role := strings.TrimSpace(m.Role)
		if role != "user" && role != "assistant" {
			continue
		}

		if strings.TrimSpace(m.Content) == "" {
			continue
		}

		messages = append(messages, &domain.Message{
			Role: domain.FromProtoRole(role),
			Content: m.Content,
		})
	}

	messages = append(messages, &domain.Message{
		Role: domain.MessageRoleUser,
		Content: req.Prompt,
	})

	ch, err := a.llm.SendMessage(r.Context(), 0, a.model, messages, nil, 0, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "llm_error"})
		return
	}

	var out strings.Builder
	for chunk := range ch {
		if chunk.Content != "" {
			out.WriteString(chunk.Content)
		}
	}

	writeJSON(w, http.StatusOK, AnalyzeResponse{
		Message: strings.TrimSpace(out.String()),
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
