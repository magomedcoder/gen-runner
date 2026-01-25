//go:build llama
// +build llama

package llama

var DefaultOptions = PredictOptions{
	Seed:          -1,
	Threads:       4,
	Tokens:        128,
	TopK:          40,
	TopP:          0.95,
	MinP:          0.05,
	Temperature:   0.8,
	Penalty:       1.1,
	Repeat:        64,
	Batch:         512,
	NKeep:         64,
	MMap:          true,
	RopeFreqBase:  10000,
	RopeFreqScale: 1.0,
}

func NewPredictOptions(opts ...PredictOption) PredictOptions {
	p := DefaultOptions
	for _, opt := range opts {
		opt(&p)
	}
	return p
}

func SetSeed(seed int) PredictOption {
	return func(p *PredictOptions) { p.Seed = seed }
}

func SetThreads(threads int) PredictOption {
	return func(p *PredictOptions) { p.Threads = threads }
}

func SetTokens(tokens int) PredictOption {
	return func(p *PredictOptions) { p.Tokens = tokens }
}

func SetTopK(topk int) PredictOption {
	return func(p *PredictOptions) { p.TopK = topk }
}

func SetTopP(topp float32) PredictOption {
	return func(p *PredictOptions) { p.TopP = topp }
}

func SetTemperature(temp float32) PredictOption {
	return func(p *PredictOptions) { p.Temperature = temp }
}

func SetStopWords(stop ...string) PredictOption {
	return func(p *PredictOptions) { p.StopPrompts = stop }
}
