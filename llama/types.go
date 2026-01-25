//go:build llama
// +build llama

package llama

type ModelOptions struct {
	ContextSize   int
	Seed          int
	NBatch        int
	F16Memory     bool
	MLock         bool
	MMap          bool
	LowVRAM       bool
	Embeddings    bool
	NUMA          bool
	NGPULayers    int
	MainGPU       string
	TensorSplit   string
	FreqRopeBase  float32
	FreqRopeScale float32
	LoraBase      string
	LoraAdapter   string
}

type PredictOptions struct {
	Seed, Threads, Tokens, TopK, Repeat, Batch, NKeep              int
	TopP, MinP, Temperature, Penalty                               float32
	F16KV, DebugMode, IgnoreEOS                                    bool
	StopPrompts                                                    []string
	TailFreeSamplingZ, TypicalP, FrequencyPenalty, PresencePenalty float32
	Mirostat                                                       int
	MirostatETA, MirostatTAU                                       float32
	PenalizeNL                                                     bool
	LogitBias, PathPromptCache                                     string
	MLock, MMap, PromptCacheAll, PromptCacheRO                     bool
	Grammar, MainGPU, TensorSplit                                  string
	RopeFreqBase, RopeFreqScale                                    float32
	NDraft                                                         int
	XTCProbability, XTCThreshold                                   float32
	DRYMultiplier, DRYBase                                         float32
	DRYAllowedLength, DRYPenaltyLastN                              int
	TopNSigma                                                      float32
	TokenCallback                                                  func(string) bool
}

type ModelOption func(p *ModelOptions)

type PredictOption func(p *PredictOptions)
