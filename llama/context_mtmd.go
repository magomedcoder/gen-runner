package llama

/*
#include "wrapper.h"
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"fmt"
	"runtime/cgo"
	"unsafe"
)

// MTMDChat выполняет мультимодальный проход (libmtmd): сообщения по порядку; при непустом
// ChatMessage.ImageBytes для сообщения подставляется маркер изображения (как в llama-mtmd-cli)
func (c *Context) MTMDChat(ctx context.Context, messages []ChatMessage, chatTemplate string, useJinja bool, callback func(string) bool, opts ...GenerateOption) error {
	if len(messages) == 0 {
		return fmt.Errorf("пустой список сообщений")
	}

	cfg := defaultGenerateConfig
	for _, o := range opts {
		o(&cfg)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return fmt.Errorf("контекст закрыт")
	}

	if c.model == nil {
		return fmt.Errorf("модель недоступна")
	}

	c.model.mu.RLock()
	modelClosed := c.model.closed
	modelPtr := c.model.modelPtr
	c.model.mu.RUnlock()
	if modelClosed || modelPtr == nil {
		return fmt.Errorf("модель закрыта")
	}

	if !bool(C.llama_wrapper_model_has_mtmd(modelPtr)) {
		return fmt.Errorf("модель без mmproj (mtmd)")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	n := len(messages)
	roles := make([]*C.char, n)
	contents := make([]*C.char, n)
	imgPtrs := make([]*C.uchar, n)
	imgLens := make([]C.size_t, n)
	hasImg := make([]C.int, n)

	defer func() {
		for i := range roles {
			if roles[i] != nil {
				C.free(unsafe.Pointer(roles[i]))
			}
			if contents[i] != nil {
				C.free(unsafe.Pointer(contents[i]))
			}
		}
	}()

	for i := range messages {
		m := &messages[i]
		roles[i] = C.CString(m.Role)
		contents[i] = C.CString(m.Content)
		if len(m.ImageBytes) > 0 {
			hasImg[i] = 1
			imgLens[i] = C.size_t(len(m.ImageBytes))
			imgPtrs[i] = (*C.uchar)(unsafe.Pointer(&m.ImageBytes[0]))
		}
	}

	var cStopWords **C.char
	var stopWordsCount C.int
	if len(cfg.stopWords) > 0 {
		stopWordsCount = C.int(len(cfg.stopWords))
		cStopWordsArray := make([]*C.char, len(cfg.stopWords))
		for i, word := range cfg.stopWords {
			cStopWordsArray[i] = C.CString(word)
		}
		defer func() {
			for _, ptr := range cStopWordsArray {
				C.free(unsafe.Pointer(ptr))
			}
		}()
		cStopWords = (**C.char)(unsafe.Pointer(&cStopWordsArray[0]))
	}

	var cDryBreakers **C.char
	var dryBreakersCount C.int
	if len(cfg.drySequenceBreakers) > 0 {
		dryBreakersCount = C.int(len(cfg.drySequenceBreakers))
		cDryBreakersArray := make([]*C.char, len(cfg.drySequenceBreakers))
		for i, breaker := range cfg.drySequenceBreakers {
			cDryBreakersArray[i] = C.CString(breaker)
		}
		defer func() {
			for _, ptr := range cDryBreakersArray {
				C.free(unsafe.Pointer(ptr))
			}
		}()
		cDryBreakers = (**C.char)(unsafe.Pointer(&cDryBreakersArray[0]))
	}

	var handle cgo.Handle
	var callbackHandle C.uintptr_t
	if callback != nil {
		handle = cgo.NewHandle(callback)
		callbackHandle = C.uintptr_t(handle)
		defer handle.Delete()
	}

	params := C.llama_wrapper_generate_params{
		prompt:                      nil,
		max_tokens:                  C.int(cfg.maxTokens),
		temperature:                 C.float(cfg.temperature),
		top_k:                       C.int(cfg.topK),
		top_p:                       C.float(cfg.topP),
		min_p:                       C.float(cfg.minP),
		typ_p:                       C.float(cfg.typP),
		top_n_sigma:                 C.float(cfg.topNSigma),
		penalty_last_n:              C.int(cfg.penaltyLastN),
		penalty_repeat:              C.float(cfg.penaltyRepeat),
		penalty_freq:                C.float(cfg.penaltyFreq),
		penalty_present:             C.float(cfg.penaltyPresent),
		dry_multiplier:              C.float(cfg.dryMultiplier),
		dry_base:                    C.float(cfg.dryBase),
		dry_allowed_length:          C.int(cfg.dryAllowedLength),
		dry_penalty_last_n:          C.int(cfg.dryPenaltyLastN),
		dry_sequence_breakers:       cDryBreakers,
		dry_sequence_breakers_count: dryBreakersCount,
		dynatemp_range:              C.float(cfg.dynatempRange),
		dynatemp_exponent:           C.float(cfg.dynatempExponent),
		xtc_probability:             C.float(cfg.xtcProbability),
		xtc_threshold:               C.float(cfg.xtcThreshold),
		mirostat:                    C.int(cfg.mirostat),
		mirostat_tau:                C.float(cfg.mirostatTau),
		mirostat_eta:                C.float(cfg.mirostatEta),
		n_prev:                      C.int(cfg.nPrev),
		n_probs:                     C.int(cfg.nProbs),
		min_keep:                    C.int(cfg.minKeep),
		seed:                        C.int(cfg.seed),
		stop_words:                  cStopWords,
		stop_words_count:            stopWordsCount,
		callback_handle:             callbackHandle,
		ignore_eos:                  C.bool(cfg.ignoreEOS),
		debug:                       C.bool(cfg.debug),
	}

	var cTmpl *C.char
	if chatTemplate != "" {
		cTmpl = C.CString(chatTemplate)
		defer C.free(unsafe.Pointer(cTmpl))
	}

	jinja := C.int(0)
	if useJinja {
		jinja = 1
	}

	st := C.llama_wrapper_mtmd_chat_prompt(
		c.contextPtr,
		modelPtr,
		cTmpl,
		jinja,
		(**C.char)(unsafe.Pointer(&roles[0])),
		(**C.char)(unsafe.Pointer(&contents[0])),
		(**C.uchar)(unsafe.Pointer(&imgPtrs[0])),
		(*C.size_t)(unsafe.Pointer(&imgLens[0])),
		(*C.int)(unsafe.Pointer(&hasImg[0])),
		C.int(n),
		params,
	)
	if st != 0 {
		return fmt.Errorf("mtmd: %s", C.GoString(C.llama_wrapper_last_error()))
	}

	_ = ctx
	return nil
}
