package llama

import (
	"runtime"
)

type contextConfig struct {
	contextSize   int
	batchSize     int
	threads       int
	threadsBatch  int
	nParallel     int // Количество параллельных последовательностей (для пакетных эмбеддингов)
	f16Memory     bool
	embeddings    bool
	prefixCaching bool   // Включить переиспользование префикса KV-кэша (по умолчанию: true)
	kvCacheType   string // Тип квантизации KV-кэша - f16, q8_0, q4_0 (по умолчанию: q8_0)
	flashAttn     string // Режим Flash Attention: auto, enabled, disabled (по умолчанию: "auto")
}

type generateConfig struct {
	maxTokens           int
	temperature         float32
	seed                int
	stopWords           []string
	draftTokens         int
	debug               bool
	topK                int
	topP                float32
	minP                float32
	typP                float32
	topNSigma           float32
	minKeep             int
	penaltyLastN        int
	penaltyRepeat       float32
	penaltyFreq         float32
	penaltyPresent      float32
	dryMultiplier       float32
	dryBase             float32
	dryAllowedLength    int
	dryPenaltyLastN     int
	drySequenceBreakers []string
	dynatempRange       float32
	dynatempExponent    float32
	xtcProbability      float32
	xtcThreshold        float32
	mirostat            int
	mirostatTau         float32
	mirostatEta         float32
	nPrev               int
	nProbs              int
	ignoreEOS           bool
}

var defaultContextConfig = contextConfig{
	contextSize:   0, // 0 = использовать нативный максимум модели (запрашивается после загрузки)
	batchSize:     512,
	threads:       runtime.NumCPU(),
	threadsBatch:  0, // 0 означает использовать то же значение, что и threads (задается в wrapper)
	nParallel:     1, // 1 для генерации, для эмбеддингов автоматически ставится выше
	f16Memory:     false,
	embeddings:    false,
	prefixCaching: true,   // Включено по умолчанию для производительности
	kvCacheType:   "q8_0", // Экономия VRAM ~50% при потере качества ~0.1%
	flashAttn:     "auto", // Позволить llama.cpp выбрать оптимальный путь
}

var defaultGenerateConfig = generateConfig{
	maxTokens:           0, // 0 = до заполнения контекста или (EOS / стоп-слова завершить раньше)
	temperature:         0.8,
	seed:                -1,
	draftTokens:         16,
	debug:               false,
	topK:                40,
	topP:                0.95,
	minP:                0.05,
	typP:                1.0,  // 1.0 = отключено
	topNSigma:           -1.0, // -1.0 = отключено
	minKeep:             0,
	penaltyLastN:        64,
	penaltyRepeat:       1.0, // 1.0 = отключено
	penaltyFreq:         0.0, // 0.0 = отключено
	penaltyPresent:      0.0, // 0.0 = отключено
	dryMultiplier:       0.0, // 0.0 = отключено
	dryBase:             1.75,
	dryAllowedLength:    2,
	dryPenaltyLastN:     -1, // -1 = размер контекста
	drySequenceBreakers: []string{"\n", ":", "\"", "*"},
	dynatempRange:       0.0, // 0.0 = отключено
	dynatempExponent:    1.0,
	xtcProbability:      0.0, // 0.0 = отключено
	xtcThreshold:        0.1,
	mirostat:            0, // 0 = отключено
	mirostatTau:         5.0,
	mirostatEta:         0.1,
	nPrev:               64,
	nProbs:              0, // 0 = отключено
	ignoreEOS:           false,
}

type modelConfig struct {
	gpuLayers               int
	mlock                   bool
	mmap                    bool
	mainGPU                 string
	tensorSplit             string
	disableProgressCallback bool
	progressCallback        ProgressCallback
	mmproj                  string
}

var defaultModelConfig = modelConfig{
	gpuLayers: -1, // По умолчанию выгружать все слои на gpu (если недоступно, используется cpu)
	mlock:     false,
	mmap:      true,
}

type ModelOption func(*modelConfig)

type ContextOption func(*contextConfig)

type GenerateOption func(*generateConfig)
