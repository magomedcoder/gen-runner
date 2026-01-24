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
