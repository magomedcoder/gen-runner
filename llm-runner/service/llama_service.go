//go:build llama

package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/magomedcoder/gen/llm-runner/domain"
	"github.com/magomedcoder/gen/llm-runner/llama"
	"github.com/magomedcoder/gen/llm-runner/template"
	"github.com/magomedcoder/gen/pkg/logger"
)

type LlamaService struct {
	modelsDir              string
	currentModelName       string
	lastModelRef           string
	currentDraftName       string
	lastDraftRef           string
	mu                     sync.RWMutex
	model                  *llama.Model
	draftModel             *llama.Model
	maxContextTokens       int
	llamaNCtx              int
	enableEmbeddings       bool
	gpuLayers              int
	mlock                  bool
	mmap                   *bool
	mainGPU                string
	tensorSplit            string
	silentLoading          bool
	progressCallback       bool
	threads                int
	threadsBatch           int
	batchSize              int
	f16Memory              bool
	kvCacheType            string
	flashAttn              string
	prefixCaching          *bool
	parallel               int
	topNSigma              *float32
	frequencyPenalty       *float32
	presencePenalty        *float32
	ignoreEOS              *bool
	dryMultiplier          *float32
	dryBase                *float32
	dryAllowedLength       *int
	dryPenaltyLastN        *int
	drySeqBreakers         []string
	xtcProbability         *float32
	xtcThreshold           *float32
	mirostat               *int
	mirostatTau            *float32
	mirostatEta            *float32
	typicalP               *float32
	minKeep                *int
	dynatempRange          *float32
	dynatempExponent       *float32
	nPrev                  *int
	nProbs                 *int
	debugGeneration        bool
	speculativeEnabled     bool
	speculativeDraftModel  string
	speculativeDraftTokens int
	tokenPipelineEnabled   bool
	chatAPIEnabled         bool
	chatStreamBufferSize   int
	chatReasoningFormat    string
	chatEnableThinking     *bool
	chatReasoningBudget    *int
	reinitLlamaLogging     bool
	logModelStats          bool
}

type LlamaOption func(*LlamaService)

func WithMaxContextTokens(n int) LlamaOption {
	return func(s *LlamaService) {
		if n > 0 {
			s.maxContextTokens = n
		}
	}
}

func WithLlamaNCtx(n int) LlamaOption {
	return func(s *LlamaService) {
		if n > 0 {
			s.llamaNCtx = n
		}
	}
}

func WithEmbeddings(enable bool) LlamaOption {
	return func(s *LlamaService) {
		s.enableEmbeddings = enable
	}
}

func WithGPULayers(n int) LlamaOption {
	return func(s *LlamaService) {
		s.gpuLayers = n
	}
}

func WithMLock(enable bool) LlamaOption {
	return func(s *LlamaService) {
		s.mlock = enable
	}
}

func WithMMap(enable bool) LlamaOption {
	return func(s *LlamaService) {
		s.mmap = &enable
	}
}

func WithMainGPU(v string) LlamaOption {
	return func(s *LlamaService) {
		s.mainGPU = strings.TrimSpace(v)
	}
}

func WithTensorSplit(v string) LlamaOption {
	return func(s *LlamaService) {
		s.tensorSplit = strings.TrimSpace(v)
	}
}

func WithSilentLoading(enable bool) LlamaOption {
	return func(s *LlamaService) {
		s.silentLoading = enable
	}
}

func WithProgressCallback(enable bool) LlamaOption {
	return func(s *LlamaService) {
		s.progressCallback = enable
	}
}

func WithThreads(n int) LlamaOption {
	return func(s *LlamaService) {
		if n > 0 {
			s.threads = n
		}
	}
}

func WithThreadsBatch(n int) LlamaOption {
	return func(s *LlamaService) {
		if n > 0 {
			s.threadsBatch = n
		}
	}
}

func WithBatchSize(n int) LlamaOption {
	return func(s *LlamaService) {
		if n > 0 {
			s.batchSize = n
		}
	}
}

func WithF16Memory(enable bool) LlamaOption {
	return func(s *LlamaService) {
		s.f16Memory = enable
	}
}

func WithKVCacheType(v string) LlamaOption {
	return func(s *LlamaService) {
		s.kvCacheType = strings.TrimSpace(v)
	}
}

func WithFlashAttn(v string) LlamaOption {
	return func(s *LlamaService) {
		s.flashAttn = strings.TrimSpace(v)
	}
}

func WithPrefixCaching(enable bool) LlamaOption {
	return func(s *LlamaService) {
		s.prefixCaching = &enable
	}
}

func WithParallel(n int) LlamaOption {
	return func(s *LlamaService) {
		if n > 0 {
			s.parallel = n
		}
	}
}

func WithTopNSigma(v float32) LlamaOption {
	return func(s *LlamaService) { s.topNSigma = &v }
}

func WithFrequencyPenalty(v float32) LlamaOption {
	return func(s *LlamaService) { s.frequencyPenalty = &v }
}

func WithPresencePenalty(v float32) LlamaOption {
	return func(s *LlamaService) { s.presencePenalty = &v }
}

func WithIgnoreEOS(v bool) LlamaOption {
	return func(s *LlamaService) { s.ignoreEOS = &v }
}

func WithDRYMultiplier(v float32) LlamaOption {
	return func(s *LlamaService) { s.dryMultiplier = &v }
}

func WithDRYBase(v float32) LlamaOption {
	return func(s *LlamaService) { s.dryBase = &v }
}

func WithDRYAllowedLength(v int) LlamaOption {
	return func(s *LlamaService) { s.dryAllowedLength = &v }
}

func WithDRYPenaltyLastN(v int) LlamaOption {
	return func(s *LlamaService) { s.dryPenaltyLastN = &v }
}

func WithDRYSequenceBreakers(v []string) LlamaOption {
	return func(s *LlamaService) { s.drySeqBreakers = append([]string(nil), v...) }
}

func WithXTC(probability, threshold float32) LlamaOption {
	return func(s *LlamaService) {
		s.xtcProbability = &probability
		s.xtcThreshold = &threshold
	}
}

func WithMirostat(version int) LlamaOption {
	return func(s *LlamaService) { s.mirostat = &version }
}

func WithMirostatTau(v float32) LlamaOption {
	return func(s *LlamaService) { s.mirostatTau = &v }
}

func WithMirostatEta(v float32) LlamaOption {
	return func(s *LlamaService) { s.mirostatEta = &v }
}

func WithTypicalP(v float32) LlamaOption {
	return func(s *LlamaService) { s.typicalP = &v }
}

func WithMinKeep(v int) LlamaOption {
	return func(s *LlamaService) { s.minKeep = &v }
}

func WithDynamicTemperature(tempRange, exponent float32) LlamaOption {
	return func(s *LlamaService) {
		s.dynatempRange = &tempRange
		s.dynatempExponent = &exponent
	}
}

func WithNPrev(v int) LlamaOption {
	return func(s *LlamaService) { s.nPrev = &v }
}

func WithNProbs(v int) LlamaOption {
	return func(s *LlamaService) { s.nProbs = &v }
}

func WithDebugGeneration(enable bool) LlamaOption {
	return func(s *LlamaService) { s.debugGeneration = enable }
}

func WithSpeculativeEnabled(enable bool) LlamaOption {
	return func(s *LlamaService) { s.speculativeEnabled = enable }
}

func WithSpeculativeDraftModel(v string) LlamaOption {
	return func(s *LlamaService) { s.speculativeDraftModel = strings.TrimSpace(v) }
}

func WithSpeculativeDraftTokens(v int) LlamaOption {
	return func(s *LlamaService) {
		if v > 0 {
			s.speculativeDraftTokens = v
		}
	}
}

func WithTokenPipelineEnabled(enable bool) LlamaOption {
	return func(s *LlamaService) { s.tokenPipelineEnabled = enable }
}

func WithChatAPIEnabled(enable bool) LlamaOption {
	return func(s *LlamaService) { s.chatAPIEnabled = enable }
}

func WithChatStreamBufferSize(v int) LlamaOption {
	return func(s *LlamaService) {
		if v > 0 {
			s.chatStreamBufferSize = v
		}
	}
}

func WithChatReasoningFormat(v string) LlamaOption {
	return func(s *LlamaService) { s.chatReasoningFormat = strings.TrimSpace(v) }
}

func WithChatEnableThinking(v bool) LlamaOption {
	return func(s *LlamaService) { s.chatEnableThinking = &v }
}

func WithChatReasoningBudget(v int) LlamaOption {
	return func(s *LlamaService) { s.chatReasoningBudget = &v }
}

func WithReinitLlamaLogging(enable bool) LlamaOption {
	return func(s *LlamaService) { s.reinitLlamaLogging = enable }
}

func WithLogModelStats(enable bool) LlamaOption {
	return func(s *LlamaService) { s.logModelStats = enable }
}

func mapChatReasoningFormat(v string) llama.ReasoningFormat {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "auto":
		return llama.ReasoningFormatAuto
	case "deepseek-legacy":
		return llama.ReasoningFormatDeepSeekLegacy
	case "deepseek":
		return llama.ReasoningFormatDeepSeek
	default:
		return llama.ReasoningFormatNone
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
		modelsDir:              modelsDir,
		llamaNCtx:              4096,
		speculativeDraftTokens: 16,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *LlamaService) applyModelChatTemplate(norm []*domain.AIChatMessage, addGen bool, chatTemplateOverride string) (string, error) {
	if s.model == nil {
		return "", fmt.Errorf("llama: модель не загружена")
	}

	messages := make([]llama.ChatMessage, 0, len(norm))
	for _, m := range norm {
		messages = append(messages, llama.ChatMessage{
			Role:    ChatRoleString(m.Role),
			Content: FormatContentForBuiltinChatTemplate(m),
		})
	}

	if !addGen {
		return "", fmt.Errorf("llama: addGen=false не поддерживается для llama")
	}

	return s.model.FormatChatPrompt(messages, llama.ChatOptions{
		ChatTemplate: strings.TrimSpace(chatTemplateOverride),
	})
}

func (s *LlamaService) resolveChatPrompt(norm []*domain.AIChatMessage, genParams *domain.GenerationParams, chatTemplateOverride string) (prompt string, presetStops []string, err error) {
	jinja := strings.TrimSpace(s.model.ChatTemplate())
	var matched *template.MatchedPreset
	if jinja != "" {
		if p, e := template.Named(jinja); e == nil {
			matched = p
			presetStops = template.PresetStopSequences(p)
		}
	}

	if p, e := s.applyModelChatTemplate(norm, true, chatTemplateOverride); e == nil && p != "" {
		return p, presetStops, nil
	}

	if matched != nil && strings.TrimSpace(chatTemplateOverride) == "" {
		text, e := RenderMatchedPreset(matched, norm, genParams)
		if e != nil {
			return "", presetStops, fmt.Errorf("llama: пресет %q: %w", matched.Name, e)
		}

		if strings.TrimSpace(text) != "" {
			return text, presetStops, nil
		}
	}

	return fallbackPlainChatPrompt(norm, genParams), presetStops, nil
}

func (s *LlamaService) ensureModel(modelName string) (*ModelYAML, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.modelsDir == "" {
		return nil, "", fmt.Errorf("llama: путь к папке с моделями не задан")
	}

	if strings.TrimSpace(modelName) == "" {
		return nil, "", fmt.Errorf("llama: укажите модель (доступные: %s)", strings.Join(s.modelDisplayNamesLocked(), ", "))
	}

	resolved, yamlCfg, err := ResolveModelForInference(s.modelsDir, modelName)
	if err != nil {
		return nil, "", fmt.Errorf("llama: %w (доступные: %s)", err, strings.Join(s.modelDisplayNamesLocked(), ", "))
	}

	fullPath := filepath.Join(s.modelsDir, resolved)
	ref := strings.TrimSpace(modelName)
	if s.model != nil && s.currentModelName == resolved && s.lastModelRef == ref {
		return yamlCfg, "", nil
	}

	if s.model != nil {
		_ = s.model.Close()
		s.model = nil
		s.currentModelName = ""
		s.lastModelRef = ""
	}

	var modelOpts []llama.ModelOption
	if s.mmap == nil {
		modelOpts = append(modelOpts, llama.WithMMap(true))
	} else {
		modelOpts = append(modelOpts, llama.WithMMap(*s.mmap))
	}
	if s.gpuLayers != 0 {
		modelOpts = append(modelOpts, llama.WithGPULayers(s.gpuLayers))
	}
	if s.mlock {
		modelOpts = append(modelOpts, llama.WithMLock())
	}
	if strings.TrimSpace(s.mainGPU) != "" {
		modelOpts = append(modelOpts, llama.WithMainGPU(s.mainGPU))
	}
	if strings.TrimSpace(s.tensorSplit) != "" {
		modelOpts = append(modelOpts, llama.WithTensorSplit(s.tensorSplit))
	}
	if s.silentLoading {
		modelOpts = append(modelOpts, llama.WithSilentLoading())
	}
	if s.progressCallback {
		modelOpts = append(modelOpts, llama.WithProgressCallback(func(progress float32) bool {
			logger.I("llama: загрузка модели %.1f%%", progress*100)
			return true
		}))
	}

	var mmProjAbs string
	if yamlCfg != nil {
		if r := strings.TrimSpace(yamlCfg.MMProj); r != "" {
			if filepath.IsAbs(r) {
				mmProjAbs = filepath.Clean(r)
			} else {
				mmProjAbs = filepath.Join(s.modelsDir, filepath.Clean(r))
			}

			st, e := os.Stat(mmProjAbs)
			if e != nil {
				return nil, "", fmt.Errorf("llama: mmproj %q: %w", mmProjAbs, e)
			}

			if st.IsDir() {
				return nil, "", fmt.Errorf("llama: mmproj %q: ожидался файл .gguf", mmProjAbs)
			}

			modelOpts = append(modelOpts, llama.WithMMProj(mmProjAbs))
		}
	}

	if s.reinitLlamaLogging {
		llama.InitLogging()
	}
	m, err := llama.LoadModel(fullPath, modelOpts...)
	if err != nil {
		return nil, "", fmt.Errorf("llama: не удалось загрузить модель %q: %w", DisplayModelName(resolved), err)
	}

	s.model = m
	s.currentModelName = resolved
	s.lastModelRef = ref
	if s.gpuLayers != 0 {
		logger.I("llama: модель %q - gpu_layers=%d (llama), проверьте offload в логах llama.cpp", DisplayModelName(resolved), s.gpuLayers)
		logger.I("llama: проверьте nvidia-smi: при загрузке модели растёт память GPU, при генерации видна утилизация")
	}
	if s.logModelStats {
		if stats, stErr := m.Stats(); stErr == nil && stats != nil {
			logger.I("llama: model stats\n%s", stats.String())
		} else if stErr != nil {
			logger.W("llama: не удалось получить model stats: %v", stErr)
		}
	}

	return yamlCfg, "", nil
}

func (s *LlamaService) ensureDraftModel() (*llama.Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.speculativeEnabled {
		return nil, nil
	}
	ref := strings.TrimSpace(s.speculativeDraftModel)
	if ref == "" {
		return nil, fmt.Errorf("llama: speculative decoding включен, но speculative_draft_model не задан")
	}
	if s.modelsDir == "" {
		return nil, fmt.Errorf("llama: путь к папке с моделями не задан")
	}

	resolved, _, err := ResolveModelForInference(s.modelsDir, ref)
	if err != nil {
		return nil, fmt.Errorf("llama: speculative draft model: %w", err)
	}
	if s.draftModel != nil && s.currentDraftName == resolved && s.lastDraftRef == ref {
		return s.draftModel, nil
	}

	if s.draftModel != nil {
		_ = s.draftModel.Close()
		s.draftModel = nil
		s.currentDraftName = ""
		s.lastDraftRef = ""
	}

	fullPath := filepath.Join(s.modelsDir, resolved)
	var modelOpts []llama.ModelOption
	if s.mmap == nil {
		modelOpts = append(modelOpts, llama.WithMMap(true))
	} else {
		modelOpts = append(modelOpts, llama.WithMMap(*s.mmap))
	}
	if s.gpuLayers != 0 {
		modelOpts = append(modelOpts, llama.WithGPULayers(s.gpuLayers))
	}
	if s.mlock {
		modelOpts = append(modelOpts, llama.WithMLock())
	}
	if strings.TrimSpace(s.mainGPU) != "" {
		modelOpts = append(modelOpts, llama.WithMainGPU(s.mainGPU))
	}
	if strings.TrimSpace(s.tensorSplit) != "" {
		modelOpts = append(modelOpts, llama.WithTensorSplit(s.tensorSplit))
	}
	if s.silentLoading {
		modelOpts = append(modelOpts, llama.WithSilentLoading())
	}
	if s.progressCallback {
		modelOpts = append(modelOpts, llama.WithProgressCallback(func(progress float32) bool {
			logger.I("llama: загрузка draft-модели %.1f%%", progress*100)
			return true
		}))
	}

	dm, err := llama.LoadModel(fullPath, modelOpts...)
	if err != nil {
		return nil, fmt.Errorf("llama: не удалось загрузить draft-модель %q: %w", DisplayModelName(resolved), err)
	}
	s.draftModel = dm
	s.currentDraftName = resolved
	s.lastDraftRef = ref
	return s.draftModel, nil
}

func (s *LlamaService) modelDisplayNamesLocked() []string {
	if s.modelsDir == "" {
		return nil
	}

	names, err := SortedDisplayModelNames(s.modelsDir)
	if err != nil {
		return nil
	}

	return names
}

func (s *LlamaService) WarmDefaultModel(ctx context.Context, model string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	_, _, err := s.ensureModel(strings.TrimSpace(model))
	return err
}

func (s *LlamaService) CheckConnection(ctx context.Context) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	files, err := ListGGUFBasenames(s.modelsDir)
	if err != nil || len(files) == 0 {
		return false, fmt.Errorf("llama: нет моделей в папке %q", s.modelsDir)
	}

	return true, nil
}

func (s *LlamaService) GetModels(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.modelDisplayNamesLocked()
	if out == nil {
		return []string{}, nil
	}

	return out, nil
}

func (s *LlamaService) GetLoadedModel(ctx context.Context) (loaded bool, ggufBasename, displayName string, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.model == nil || strings.TrimSpace(s.currentModelName) == "" {
		return false, "", "", nil
	}

	return true, s.currentModelName, DisplayModelName(s.currentModelName), nil
}

func (s *LlamaService) UnloadModel(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.model != nil {
		_ = s.model.Close()
		s.model = nil
		s.currentModelName = ""
		s.lastModelRef = ""
	}
	if s.draftModel != nil {
		_ = s.draftModel.Close()
		s.draftModel = nil
		s.currentDraftName = ""
		s.lastDraftRef = ""
	}

	return nil
}

func (s *LlamaService) SendMessage(ctx context.Context, model string, messages []*domain.AIChatMessage, stopSequences []string, genParams *domain.GenerationParams) (chan domain.TextStreamChunk, error) {
	yamlCfg, _, err := s.ensureModel(model)
	if err != nil {
		return nil, err
	}

	norm := NormalizeChatMessages(messages)
	norm = ApplyModelYAMLSystem(norm, yamlCfg)
	norm = ApplyModelYAMLMessages(norm, yamlCfg)
	if len(norm) == 0 {
		return nil, fmt.Errorf("llama: пустой список сообщений (нет текста content)")
	}

	wantVision := MessagesHaveVisionAttachments(norm)
	if wantVision {
		s.mu.RLock()
		mr := s.model
		s.mu.RUnlock()
		if mr == nil || !mr.HasMTMD() {
			return nil, fmt.Errorf("llama: в сообщении есть изображение, но не загружен mmproj: укажите в YAML модели поле mmproj (путь к .gguf проектору рядом с весами)")
		}
	}

	genMerged := MergeGenParams(genParams, yamlCfg)
	chatTmpl := ""
	if yamlCfg != nil {
		chatTmpl = yamlCfg.Template
	}

	var prompt string
	var presetStops []string
	if !wantVision {
		prompt, presetStops, err = s.resolveChatPrompt(norm, genMerged, chatTmpl)
		if err != nil {
			return nil, err
		}
		prompt = applyResponseFormatPrompt(prompt, genMerged)
	}

	stopForPredict := MergeStopSequences(stopSequences, presetStops)
	if yamlCfg != nil && len(yamlCfg.Stop) > 0 {
		stopForPredict = MergeStopSequences(stopForPredict, yamlCfg.Stop)
	}
	if !wantVision {
		stopForPredict = MergeStopSequences(stopForPredict, inferStopSequencesFromPrompt(prompt))
	}

	contextOpts := make([]llama.ContextOption, 0, 4)
	nCtx := s.llamaNCtx
	if nCtx <= 0 {
		nCtx = 4096
	}
	if yamlCfg != nil && yamlCfg.NumCtx != nil && *yamlCfg.NumCtx > 0 {
		nCtx = *yamlCfg.NumCtx
	}
	contextOpts = append(contextOpts, llama.WithContext(nCtx))
	if s.threads > 0 {
		contextOpts = append(contextOpts, llama.WithThreads(s.threads))
	}
	if s.threadsBatch > 0 {
		contextOpts = append(contextOpts, llama.WithThreadsBatch(s.threadsBatch))
	}
	if s.batchSize > 0 {
		contextOpts = append(contextOpts, llama.WithBatch(s.batchSize))
	}
	if s.f16Memory {
		contextOpts = append(contextOpts, llama.WithF16Memory())
	}
	if strings.TrimSpace(s.kvCacheType) != "" {
		contextOpts = append(contextOpts, llama.WithKVCacheType(s.kvCacheType))
	}
	if strings.TrimSpace(s.flashAttn) != "" {
		contextOpts = append(contextOpts, llama.WithFlashAttn(s.flashAttn))
	}
	if s.prefixCaching != nil {
		contextOpts = append(contextOpts, llama.WithPrefixCaching(*s.prefixCaching))
	}
	if s.parallel > 0 {
		contextOpts = append(contextOpts, llama.WithParallel(s.parallel))
	}

	s.mu.RLock()
	modelRef := s.model
	s.mu.RUnlock()
	if modelRef == nil {
		return nil, fmt.Errorf("llama: модель не загружена")
	}
	var draftModelRef *llama.Model
	if !wantVision {
		draftModelRef, err = s.ensureDraftModel()
		if err != nil {
			return nil, err
		}
	}

	genCtx, err := modelRef.NewContext(contextOpts...)
	if err != nil {
		return nil, err
	}

	var promptTokens []int32
	if !wantVision {
		promptTokens, err = genCtx.Tokenize(prompt)
		if err != nil {
			_ = genCtx.Close()
			return nil, fmt.Errorf("llama: tokenize prompt: %w", err)
		}

		promptTokenCount := len(promptTokens)
		if promptTokenCount > nCtx {
			_ = genCtx.Close()
			return nil, fmt.Errorf("llama: контекст слишком велик (%d токенов, n_ctx=%d)", promptTokenCount, nCtx)
		}

		if s.maxContextTokens > 0 && promptTokenCount > s.maxContextTokens {
			_ = genCtx.Close()
			return nil, fmt.Errorf("llama: контекст слишком велик (%d токенов, лимит %d)", promptTokenCount, s.maxContextTokens)
		}
	}

	useSpeculative := s.speculativeEnabled && draftModelRef != nil && !wantVision
	useTokenPipeline := s.tokenPipelineEnabled && !useSpeculative && !wantVision
	useChatAPI := s.chatAPIEnabled && !useSpeculative && !useTokenPipeline && !wantVision
	if useChatAPI && requiresGeneratePipeline(genMerged, yamlCfg, s) {
		useChatAPI = false
	}
	chatMessages := toLlamaChatMessages(norm)
	chatMessages = applyResponseFormatToChatMessages(chatMessages, genMerged)

	out := make(chan domain.TextStreamChunk, 32)
	go func() {
		defer close(out)
		defer genCtx.Close()
		var draftCtx *llama.Context
		if useSpeculative {
			draftCtx, err = draftModelRef.NewContext(contextOpts...)
			if err != nil {
				return
			}
			defer draftCtx.Close()
		}
		generateOpts := make([]llama.GenerateOption, 0, 16)
		if genMerged != nil {
			if genMerged.Temperature != nil {
				generateOpts = append(generateOpts, llama.WithTemperature(*genMerged.Temperature))
			}

			if genMerged.MaxTokens != nil && *genMerged.MaxTokens > 0 {
				generateOpts = append(generateOpts, llama.WithMaxTokens(int(*genMerged.MaxTokens)))
			}

			if genMerged.TopK != nil && *genMerged.TopK > 0 {
				generateOpts = append(generateOpts, llama.WithTopK(int(*genMerged.TopK)))
			}

			if genMerged.TopP != nil {
				generateOpts = append(generateOpts, llama.WithTopP(*genMerged.TopP))
			}

			if genMerged.MinP != nil {
				generateOpts = append(generateOpts, llama.WithMinP(*genMerged.MinP))
			}

			if yamlCfg != nil && yamlCfg.Parameter != nil {
				p := yamlCfg.Parameter
				if p.RepeatLastN != nil {
					generateOpts = append(generateOpts, llama.WithPenaltyLastN(*p.RepeatLastN))
				}

				if p.RepeatPenalty != nil {
					generateOpts = append(generateOpts, llama.WithRepeatPenalty(float32(*p.RepeatPenalty)))
				}

				if p.Seed != nil {
					generateOpts = append(generateOpts, llama.WithSeed(*p.Seed))
				}
			}
		}

		if len(stopForPredict) > 0 {
			generateOpts = append(generateOpts, llama.WithStopWords(stopForPredict...))
		}
		if s.topNSigma != nil {
			generateOpts = append(generateOpts, llama.WithTopNSigma(*s.topNSigma))
		}
		if s.frequencyPenalty != nil {
			generateOpts = append(generateOpts, llama.WithFrequencyPenalty(*s.frequencyPenalty))
		}
		if s.presencePenalty != nil {
			generateOpts = append(generateOpts, llama.WithPresencePenalty(*s.presencePenalty))
		}
		if s.ignoreEOS != nil {
			generateOpts = append(generateOpts, llama.WithIgnoreEOS(*s.ignoreEOS))
		}
		if s.dryMultiplier != nil {
			generateOpts = append(generateOpts, llama.WithDRYMultiplier(*s.dryMultiplier))
		}
		if s.dryBase != nil {
			generateOpts = append(generateOpts, llama.WithDRYBase(*s.dryBase))
		}
		if s.dryAllowedLength != nil {
			generateOpts = append(generateOpts, llama.WithDRYAllowedLength(*s.dryAllowedLength))
		}
		if s.dryPenaltyLastN != nil {
			generateOpts = append(generateOpts, llama.WithDRYPenaltyLastN(*s.dryPenaltyLastN))
		}
		if len(s.drySeqBreakers) > 0 {
			generateOpts = append(generateOpts, llama.WithDRYSequenceBreakers(s.drySeqBreakers...))
		}
		if s.xtcProbability != nil && s.xtcThreshold != nil {
			generateOpts = append(generateOpts, llama.WithXTC(*s.xtcProbability, *s.xtcThreshold))
		}
		if s.mirostat != nil {
			generateOpts = append(generateOpts, llama.WithMirostat(*s.mirostat))
		}
		if s.mirostatTau != nil {
			generateOpts = append(generateOpts, llama.WithMirostatTau(*s.mirostatTau))
		}
		if s.mirostatEta != nil {
			generateOpts = append(generateOpts, llama.WithMirostatEta(*s.mirostatEta))
		}
		if s.typicalP != nil {
			generateOpts = append(generateOpts, llama.WithTypicalP(*s.typicalP))
		}
		if s.minKeep != nil {
			generateOpts = append(generateOpts, llama.WithMinKeep(*s.minKeep))
		}
		if s.dynatempRange != nil && s.dynatempExponent != nil {
			generateOpts = append(generateOpts, llama.WithDynamicTemperature(*s.dynatempRange, *s.dynatempExponent))
		}
		if s.nPrev != nil {
			generateOpts = append(generateOpts, llama.WithNPrev(*s.nPrev))
		}
		if s.nProbs != nil {
			generateOpts = append(generateOpts, llama.WithNProbs(*s.nProbs))
		}
		if s.debugGeneration {
			generateOpts = append(generateOpts, llama.WithDebug())
		}
		if useSpeculative && s.speculativeDraftTokens > 0 {
			generateOpts = append(generateOpts, llama.WithDraftTokens(s.speculativeDraftTokens))
		}

		emitChunk := func(content, reasoning string) {
			if content == "" && reasoning == "" {
				return
			}
			select {
			case <-ctx.Done():
			case out <- domain.TextStreamChunk{Content: content, ReasoningContent: reasoning}:
			}
		}

		streamFilter := newStopStreamFilter(stopForPredict, func(s string) { emitChunk(s, "") })
		if wantVision {
			mtmdMsgs := toLlamaMTMDChatMessages(norm)
			mtmdMsgs = applyResponseFormatToChatMessages(mtmdMsgs, genMerged)
			err := genCtx.MTMDChat(ctx, mtmdMsgs, strings.TrimSpace(chatTmpl), false, func(token string) bool {
				if token == "" {
					return true
				}

				streamFilter.push(token)

				select {
				case <-ctx.Done():
					return false
				default:
					return true
				}

			}, generateOpts...)
			streamFilter.flush()
			if err != nil {
				emitChunk(fmt.Sprintf("\n[ошибка vision]: %v", err), "")
			}

			return
		}
		if useChatAPI {
			chatOpts := llama.ChatOptions{
				StopWords:       stopForPredict,
				ChatTemplate:    strings.TrimSpace(chatTmpl),
				ReasoningFormat: mapChatReasoningFormat(s.chatReasoningFormat),
			}
			if s.chatStreamBufferSize > 0 {
				chatOpts.StreamBufferSize = s.chatStreamBufferSize
			}
			if genMerged != nil && genMerged.EnableThinking != nil {
				v := *genMerged.EnableThinking
				chatOpts.EnableThinking = &v
			} else if s.chatEnableThinking != nil {
				v := *s.chatEnableThinking
				chatOpts.EnableThinking = &v
			}
			if s.chatReasoningBudget != nil {
				v := *s.chatReasoningBudget
				chatOpts.ReasoningBudget = &v
			}
			if genMerged != nil {
				if genMerged.MaxTokens != nil && *genMerged.MaxTokens > 0 {
					v := int(*genMerged.MaxTokens)
					chatOpts.MaxTokens = &v
				}
				if genMerged.Temperature != nil {
					v := *genMerged.Temperature
					chatOpts.Temperature = &v
				}
				if genMerged.TopP != nil {
					v := *genMerged.TopP
					chatOpts.TopP = &v
				}
				if genMerged.TopK != nil && *genMerged.TopK > 0 {
					v := int(*genMerged.TopK)
					chatOpts.TopK = &v
				}
				if yamlCfg != nil && yamlCfg.Parameter != nil && yamlCfg.Parameter.Seed != nil {
					v := *yamlCfg.Parameter.Seed
					chatOpts.Seed = &v
				}
			}

			deltas, errs := genCtx.ChatStream(ctx, chatMessages, chatOpts)
			for d := range deltas {
				if d.ReasoningContent != "" {
					emitChunk("", d.ReasoningContent)
				}
				if d.Content != "" {
					streamFilter.push(d.Content)
				}
			}
			if err := <-errs; err != nil {
				streamFilter.flush()
				return
			}
		} else if useTokenPipeline {
			err := genCtx.GenerateWithTokensStream(promptTokens, func(token string) bool {
				if token == "" {
					return true
				}
				streamFilter.push(token)
				select {
				case <-ctx.Done():
					return false
				default:
					return true
				}
			}, generateOpts...)
			if err != nil {
				streamFilter.flush()
				return
			}
		} else {
			var tokens <-chan string
			var errs <-chan error
			if useSpeculative {
				tokens, errs = genCtx.GenerateWithDraftChannel(ctx, prompt, draftCtx, generateOpts...)
			} else {
				tokens, errs = genCtx.GenerateChannel(ctx, prompt, generateOpts...)
			}
			for token := range tokens {
				if token == "" {
					continue
				}
				streamFilter.push(token)
			}
			if err := <-errs; err != nil {
				streamFilter.flush()
				return
			}
		}
		streamFilter.flush()
		if cached, err := genCtx.GetCachedTokenCount(); err == nil {
			logger.D("llama: cached token count=%d", cached)
		}
	}()
	return out, nil
}

func toLlamaChatMessages(norm []*domain.AIChatMessage) []llama.ChatMessage {
	messages := make([]llama.ChatMessage, 0, len(norm))
	for _, m := range norm {
		messages = append(messages, llama.ChatMessage{
			Role:    ChatRoleString(m.Role),
			Content: FormatContentForBuiltinChatTemplate(m),
		})
	}
	return messages
}

func toLlamaMTMDChatMessages(norm []*domain.AIChatMessage) []llama.ChatMessage {
	out := make([]llama.ChatMessage, 0, len(norm))
	for _, m := range norm {
		if m == nil {
			continue
		}

		cm := llama.ChatMessage{
			Role:    ChatRoleString(m.Role),
			Content: FormatContentForBuiltinChatTemplate(m),
		}

		if messageLikelyVisionImageAttachment(m) {
			cm.ImageBytes = append([]byte(nil), m.AttachmentContent...)
			if m.Role == domain.AIChatMessageRoleUser {
				cm.Content = strings.TrimSpace(m.Content)
			}
		}

		out = append(out, cm)
	}

	return out
}

func requiresGeneratePipeline(genMerged *domain.GenerationParams, yamlCfg *ModelYAML, s *LlamaService) bool {
	if genMerged != nil && genMerged.MinP != nil {
		return true
	}
	if yamlCfg != nil && yamlCfg.Parameter != nil {
		p := yamlCfg.Parameter
		if p.RepeatLastN != nil || p.RepeatPenalty != nil {
			return true
		}
	}
	if s.topNSigma != nil || s.frequencyPenalty != nil || s.presencePenalty != nil || s.ignoreEOS != nil {
		return true
	}
	if s.dryMultiplier != nil || s.dryBase != nil || s.dryAllowedLength != nil || s.dryPenaltyLastN != nil || len(s.drySeqBreakers) > 0 {
		return true
	}
	if s.xtcProbability != nil || s.xtcThreshold != nil || s.mirostat != nil || s.mirostatTau != nil || s.mirostatEta != nil {
		return true
	}
	if s.typicalP != nil || s.minKeep != nil || s.dynatempRange != nil || s.dynatempExponent != nil || s.nPrev != nil || s.nProbs != nil || s.debugGeneration {
		return true
	}
	return false
}

func applyResponseFormatPrompt(prompt string, genParams *domain.GenerationParams) string {
	if genParams == nil || genParams.ResponseFormat == nil {
		return prompt
	}
	if !strings.EqualFold(strings.TrimSpace(genParams.ResponseFormat.Type), "json_object") {
		return prompt
	}

	grammar := DefaultJSONObjectGrammar
	if genParams.ResponseFormat.Schema != nil && strings.TrimSpace(*genParams.ResponseFormat.Schema) != "" {
		grammar = strings.TrimSpace(*genParams.ResponseFormat.Schema)
	}

	var b strings.Builder
	b.WriteString(prompt)
	b.WriteString("\n\n")
	b.WriteString("### RESPONSE FORMAT REQUIREMENT\n")
	b.WriteString("Return ONLY a single valid JSON object. No markdown, no prose, no code fences.\n")
	b.WriteString("Use this grammar/schema constraint:\n")
	b.WriteString(grammar)
	b.WriteString("\n")
	return b.String()
}

func applyResponseFormatToChatMessages(messages []llama.ChatMessage, genParams *domain.GenerationParams) []llama.ChatMessage {
	if genParams == nil || genParams.ResponseFormat == nil {
		return messages
	}
	if !strings.EqualFold(strings.TrimSpace(genParams.ResponseFormat.Type), "json_object") {
		return messages
	}

	grammar := DefaultJSONObjectGrammar
	if genParams.ResponseFormat.Schema != nil && strings.TrimSpace(*genParams.ResponseFormat.Schema) != "" {
		grammar = strings.TrimSpace(*genParams.ResponseFormat.Schema)
	}

	instruction := "Return ONLY a single valid JSON object. No markdown, no prose, no code fences.\nUse this grammar/schema constraint:\n" + grammar
	if len(messages) == 0 {
		return []llama.ChatMessage{{Role: "system", Content: instruction}}
	}

	out := append([]llama.ChatMessage(nil), messages...)
	last := out[len(out)-1]
	last.Content = strings.TrimSpace(last.Content) + "\n\n### RESPONSE FORMAT REQUIREMENT\n" + instruction
	out[len(out)-1] = last
	return out
}

func (s *LlamaService) Embed(ctx context.Context, model string, text string) ([]float32, error) {
	if _, _, err := s.ensureModel(model); err != nil {
		return nil, err
	}

	s.mu.RLock()
	modelRef := s.model
	nCtx := s.llamaNCtx
	s.mu.RUnlock()
	if nCtx <= 0 {
		nCtx = 4096
	}
	if modelRef == nil {
		return nil, fmt.Errorf("llama: модель не загружена")
	}

	ctxOpts := []llama.ContextOption{
		llama.WithContext(nCtx),
		llama.WithEmbeddings(),
	}
	if s.threads > 0 {
		ctxOpts = append(ctxOpts, llama.WithThreads(s.threads))
	}
	if s.threadsBatch > 0 {
		ctxOpts = append(ctxOpts, llama.WithThreadsBatch(s.threadsBatch))
	}
	if s.batchSize > 0 {
		ctxOpts = append(ctxOpts, llama.WithBatch(s.batchSize))
	}
	if s.f16Memory {
		ctxOpts = append(ctxOpts, llama.WithF16Memory())
	}
	if strings.TrimSpace(s.kvCacheType) != "" {
		ctxOpts = append(ctxOpts, llama.WithKVCacheType(s.kvCacheType))
	}
	if strings.TrimSpace(s.flashAttn) != "" {
		ctxOpts = append(ctxOpts, llama.WithFlashAttn(s.flashAttn))
	}
	if s.prefixCaching != nil {
		ctxOpts = append(ctxOpts, llama.WithPrefixCaching(*s.prefixCaching))
	}
	if s.parallel > 0 {
		ctxOpts = append(ctxOpts, llama.WithParallel(s.parallel))
	}
	embedCtx, err := modelRef.NewContext(ctxOpts...)
	if err != nil {
		return nil, err
	}
	defer embedCtx.Close()

	return embedCtx.GetEmbeddings(text)
}

func (s *LlamaService) EmbedBatch(ctx context.Context, model string, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("llama: пустой список текстов")
	}
	if _, _, err := s.ensureModel(model); err != nil {
		return nil, err
	}

	s.mu.RLock()
	modelRef := s.model
	nCtx := s.llamaNCtx
	s.mu.RUnlock()
	if nCtx <= 0 {
		nCtx = 4096
	}
	if modelRef == nil {
		return nil, fmt.Errorf("llama: модель не загружена")
	}

	ctxOpts := []llama.ContextOption{
		llama.WithContext(nCtx),
		llama.WithEmbeddings(),
	}
	if s.threads > 0 {
		ctxOpts = append(ctxOpts, llama.WithThreads(s.threads))
	}
	if s.threadsBatch > 0 {
		ctxOpts = append(ctxOpts, llama.WithThreadsBatch(s.threadsBatch))
	}
	if s.batchSize > 0 {
		ctxOpts = append(ctxOpts, llama.WithBatch(s.batchSize))
	}
	if s.f16Memory {
		ctxOpts = append(ctxOpts, llama.WithF16Memory())
	}
	if strings.TrimSpace(s.kvCacheType) != "" {
		ctxOpts = append(ctxOpts, llama.WithKVCacheType(s.kvCacheType))
	}
	if strings.TrimSpace(s.flashAttn) != "" {
		ctxOpts = append(ctxOpts, llama.WithFlashAttn(s.flashAttn))
	}
	if s.prefixCaching != nil {
		ctxOpts = append(ctxOpts, llama.WithPrefixCaching(*s.prefixCaching))
	}
	if s.parallel > 0 {
		ctxOpts = append(ctxOpts, llama.WithParallel(s.parallel))
	}
	embedCtx, err := modelRef.NewContext(ctxOpts...)
	if err != nil {
		return nil, err
	}
	defer embedCtx.Close()

	return embedCtx.GetEmbeddingsBatch(texts)
}
