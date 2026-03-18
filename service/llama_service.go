//go:build llama

package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/magomedcoder/llm-runner/domain"
	llama "github.com/magomedcoder/llm-runner/llama"
)

const defaultChunkSize = 128

type LlamaService struct {
	modelsDir        string
	currentModelName string
	chunkSize        int
	predictOpts      []llama.PredictOption
	mu               sync.RWMutex
	model            *llama.LLama
	maxContextTokens int
	enableEmbeddings bool
}

type LlamaOption func(*LlamaService)

func WithChunkSize(n int) LlamaOption {
	return func(s *LlamaService) {
		if n > 0 {
			s.chunkSize = n
		}
	}
}

func WithPredictOptions(opts ...llama.PredictOption) LlamaOption {
	return func(s *LlamaService) {
		s.predictOpts = opts
	}
}

func WithMaxContextTokens(n int) LlamaOption {
	return func(s *LlamaService) {
		if n > 0 {
			s.maxContextTokens = n
		}
	}
}

func WithEmbeddings(enable bool) LlamaOption {
	return func(s *LlamaService) {
		s.enableEmbeddings = enable
	}
}

func NewLlamaService(modelPath string, opts ...LlamaOption) *LlamaService {
	modelsDir := modelPath
	if modelPath != "" {
		if info, err := os.Stat(modelPath); err == nil && !info.IsDir() {
			modelsDir = filepath.Dir(modelPath)
		}
	}

	s := &LlamaService{
		modelsDir: modelsDir,
		chunkSize: defaultChunkSize,
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.chunkSize <= 0 {
		s.chunkSize = defaultChunkSize
	}

	return s
}

func (s *LlamaService) ensureModel(modelName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.modelsDir == "" {
		return fmt.Errorf("llama: путь к папке с моделями не задан")
	}

	if modelName == "" {
		return fmt.Errorf("llama: укажите модель (доступные: %s)", strings.Join(s.modelNamesLocked(), ", "))
	}

	fullPath := filepath.Join(s.modelsDir, modelName)
	if s.model != nil && s.currentModelName == modelName {
		return nil
	}

	if s.model != nil {
		s.model.Free()
		s.model = nil
		s.currentModelName = ""
	}

	var modelOpts []llama.ModelOption
	if s.enableEmbeddings {
		modelOpts = append(modelOpts, llama.EnableEmbeddings)
	}

	m, err := llama.New(fullPath, modelOpts...)
	if err != nil {
		return fmt.Errorf("llama: не удалось загрузить модель %q: %w", modelName, err)
	}

	s.model = m
	s.currentModelName = modelName
	return nil
}

func (s *LlamaService) modelNamesLocked() []string {
	if s.modelsDir == "" {
		return nil
	}

	entries, err := os.ReadDir(s.modelsDir)
	if err != nil {
		return nil
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".gguf" {
			names = append(names, e.Name())
		}
	}

	sort.Strings(names)

	return names
}

func (s *LlamaService) CheckConnection(ctx context.Context) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := s.modelNamesLocked()
	if len(names) == 0 {
		return false, fmt.Errorf("llama: нет моделей в папке %q", s.modelsDir)
	}
	return true, nil
}

func (s *LlamaService) GetModels(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.modelNamesLocked(), nil
}

func (s *LlamaService) SendMessage(ctx context.Context, model string, messages []*domain.AIChatMessage, stopSequences []string, genParams *domain.GenerationParams) (chan string, error) {
	prompt := buildPrompt(messages, genParams)
	if s.maxContextTokens > 0 {
		approxTokens := len(prompt)/4 + 1
		if approxTokens > s.maxContextTokens {
			return nil, fmt.Errorf("llama: контекст слишком велик (≈%d токенов, лимит %d)", approxTokens, s.maxContextTokens)
		}
	}

	if err := s.ensureModel(model); err != nil {
		return nil, err
	}

	out := make(chan string, 32)
	go func() {
		defer close(out)
		opts := make([]llama.PredictOption, 0, len(s.predictOpts)+6)
		opts = append(opts, s.predictOpts...)
		if genParams != nil {
			if genParams.Temperature != nil {
				opts = append(opts, llama.SetTemperature(*genParams.Temperature))
			}

			if genParams.MaxTokens != nil && *genParams.MaxTokens > 0 {
				opts = append(opts, llama.SetTokens(int(*genParams.MaxTokens)))
			}

			if genParams.TopK != nil && *genParams.TopK > 0 {
				opts = append(opts, llama.SetTopK(int(*genParams.TopK)))
			}

			if genParams.TopP != nil {
				opts = append(opts, llama.SetTopP(*genParams.TopP))
			}

			if genParams.ResponseFormat != nil && genParams.ResponseFormat.Type == "json_object" {
				grammar := DefaultJSONObjectGrammar
				if genParams.ResponseFormat.Schema != nil && *genParams.ResponseFormat.Schema != "" {
					grammar = *genParams.ResponseFormat.Schema
				}

				if grammar != "" {
					opts = append(opts, llama.WithGrammar(grammar))
				}
			}
		}

		if len(stopSequences) > 0 {
			opts = append(opts, llama.SetStopWords(stopSequences...))
		}

		opts = append(opts, llama.SetTokenCallback(func(token string) bool {
			select {
			case <-ctx.Done():
				return false
			default:
				if token != "" {
					select {
					case <-ctx.Done():
						return false
					case out <- token:
					}
				}
				return true
			}
		}))
		s.mu.Lock()
		_, err := s.model.Predict(prompt, opts...)
		s.mu.Unlock()
		if err != nil {
			return
		}
	}()
	return out, nil
}

func (s *LlamaService) Embed(ctx context.Context, model string, text string) ([]float32, error) {
	if err := s.ensureModel(model); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.model == nil {
		return nil, fmt.Errorf("llama: модель не загружена")
	}

	return s.model.Embeddings(text, s.predictOpts...)
}

func buildPrompt(messages []*domain.AIChatMessage, genParams *domain.GenerationParams) string {
	n := len("Assistant: ")
	for _, m := range messages {
		if m != nil {
			n += len(m.Content) + 24
		}
	}
	if genParams != nil && len(genParams.Tools) > 0 {
		for _, t := range genParams.Tools {
			n += len(t.Name) + len(t.Description) + len(t.ParametersJSON) + 48
		}
		n += 96
	}
	var b strings.Builder
	b.Grow(n)
	for _, m := range messages {
		var role string
		switch m.Role {
		case domain.AIChatMessageRoleSystem:
			role = "System"
		case domain.AIChatMessageRoleAssistant:
			role = "Assistant"
		default:
			role = "User"
		}
		b.WriteString(role)
		b.WriteString(": ")
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	if genParams != nil && len(genParams.Tools) > 0 {
		b.WriteString(buildToolsPrompt(genParams.Tools))
	}
	b.WriteString("Assistant: ")

	return b.String()
}

func buildToolsPrompt(tools []domain.Tool) string {
	var b strings.Builder
	b.WriteString("\nTools:\n")
	for _, t := range tools {
		b.WriteString("- ")
		b.WriteString(t.Name)
		if t.Description != "" {
			b.WriteString(": ")
			b.WriteString(t.Description)
		}
		if t.ParametersJSON != "" {
			b.WriteString(" (params: ")
			b.WriteString(t.ParametersJSON)
			b.WriteString(")")
		}
		b.WriteString("\n")
	}
	b.WriteString("\nReply with JSON: {\"name\": \"tool_name\", \"arguments\": {...}}\n\n")

	return b.String()
}
