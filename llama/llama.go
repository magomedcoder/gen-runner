//go:build llama
// +build llama

package llama

/*
#cgo CFLAGS: -I${SRCDIR}
#cgo LDFLAGS: -L${SRCDIR} -lllama -lm -lstdc++
#include "llama.h"
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"strings"
	"unsafe"
)

type LLama struct {
	state       unsafe.Pointer
	embeddings  bool
	contextSize int
}

func New(model string, opts ...ModelOption) (*LLama, error) {
	mo := NewModelOptions(opts...)
	modelPath := C.CString(model)

	defer C.free(unsafe.Pointer(modelPath))

	loraBase := C.CString(mo.LoraBase)

	defer C.free(unsafe.Pointer(loraBase))

	loraAdapter := C.CString(mo.LoraAdapter)

	defer C.free(unsafe.Pointer(loraAdapter))

	result := C.load_model(
		modelPath,
		C.int(mo.ContextSize),
		C.int(mo.Seed),
		C.bool(mo.F16Memory),
		C.bool(mo.MLock),
		C.bool(mo.Embeddings),
		C.bool(mo.MMap),
		C.bool(mo.LowVRAM),
		C.int(mo.NGPULayers),
		C.int(mo.NBatch),
		C.CString(mo.MainGPU),
		C.CString(mo.TensorSplit),
		C.bool(mo.NUMA),
		C.float(mo.FreqRopeBase),
		C.float(mo.FreqRopeScale),
		loraAdapter, loraBase,
	)
	if result == nil {
		return nil, fmt.Errorf("не удалось загрузить модель")
	}

	return &LLama{
		state:       result,
		contextSize: mo.ContextSize,
		embeddings:  mo.Embeddings,
	}, nil
}

func (l *LLama) Free() {
	C.llama_binding_free_model(l.state)
}

type ModelInfo struct {
	VocabSize     int
	ContextLength int
	EmbeddingSize int
	LayerCount    int
	ModelSize     int64
	ParamCount    int64
	Description   string
}

func (l *LLama) GetModelInfo() ModelInfo {
	descBuf := make([]byte, 256)
	C.get_model_description(l.state, (*C.char)(unsafe.Pointer(&descBuf[0])), C.int(len(descBuf)))
	return ModelInfo{
		VocabSize:     int(C.get_model_n_vocab(l.state)),
		ContextLength: int(C.get_model_n_ctx_train(l.state)),
		EmbeddingSize: int(C.get_model_n_embd(l.state)),
		LayerCount:    int(C.get_model_n_layer(l.state)),
		ModelSize:     int64(C.get_model_size(l.state)),
		ParamCount:    int64(C.get_model_n_params(l.state)),
		Description:   string(descBuf[:cStrLen(descBuf)]),
	}
}

func cStrLen(b []byte) int {
	for i, v := range b {
		if v == 0 {
			return i
		}
	}

	return len(b)
}

func (l *LLama) Predict(text string, opts ...PredictOption) (string, error) {
	po := NewPredictOptions(opts...)
	input := C.CString(text)
	defer C.free(unsafe.Pointer(input))
	if po.Tokens == 0 {
		po.Tokens = 99999999
	}

	out := make([]byte, po.Tokens)
	reverseCount := len(po.StopPrompts)
	reversePrompt := make([]*C.char, reverseCount)
	var pass **C.char
	for i, s := range po.StopPrompts {
		cs := C.CString(s)
		reversePrompt[i] = cs
		pass = &reversePrompt[0]
	}

	params := C.llama_allocate_params(
		input,
		C.int(po.Seed),
		C.int(po.Threads),
		C.int(po.Tokens),
		C.int(po.TopK),
		C.float(po.TopP),
		C.float(po.MinP),
		C.float(po.Temperature),
		C.float(po.Penalty),
		C.int(po.Repeat),
		C.bool(po.IgnoreEOS),
		C.bool(po.F16KV),
		C.int(po.Batch),
		C.int(po.NKeep),
		pass,
		C.int(reverseCount),
		C.float(po.TailFreeSamplingZ),
		C.float(po.TypicalP),
		C.float(po.FrequencyPenalty),
		C.float(po.PresencePenalty),
		C.int(po.Mirostat),
		C.float(po.MirostatETA),
		C.float(po.MirostatTAU),
		C.bool(po.PenalizeNL),
		C.CString(po.LogitBias),
		C.CString(po.PathPromptCache),
		C.bool(po.PromptCacheAll),
		C.bool(po.MLock),
		C.bool(po.MMap),
		C.CString(po.MainGPU),
		C.CString(po.TensorSplit),
		C.bool(po.PromptCacheRO),
		C.CString(po.Grammar),
		C.float(po.RopeFreqBase),
		C.float(po.RopeFreqScale),
		C.int(po.NDraft),
		C.float(po.XTCProbability),
		C.float(po.XTCThreshold),
		C.float(po.DRYMultiplier),
		C.float(po.DRYBase),
		C.int(po.DRYAllowedLength),
		C.int(po.DRYPenaltyLastN),
		C.float(po.TopNSigma),
	)
	ret := C.llama_predict(params, l.state, (*C.char)(unsafe.Pointer(&out[0])), C.bool(po.DebugMode))
	C.llama_free_params(params)
	for _, c := range reversePrompt {
		C.free(unsafe.Pointer(c))
	}

	if ret != 0 {
		return "", fmt.Errorf("прогнозирование не удалось\n")
	}

	res := strings.TrimPrefix(C.GoString((*C.char)(unsafe.Pointer(&out[0]))), " ")
	res = strings.TrimPrefix(res, text)
	res = strings.TrimPrefix(res, "\n")
	for _, s := range po.StopPrompts {
		res = strings.TrimRight(res, s)
	}

	return res, nil
}
