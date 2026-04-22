package llama

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"
)

/*
#cgo CFLAGS: -I./llama.cpp -I./ -I./llama.cpp/ggml/include -I./llama.cpp/include -I./llama.cpp/common -I./llama.cpp/vendor
#cgo CPPFLAGS: -I./llama.cpp -I./ -I./llama.cpp/ggml/include -I./llama.cpp/include -I./llama.cpp/common -I./llama.cpp/vendor
#cgo LDFLAGS: -L./ -lbinding -lcommon -lmtmd -lllama -lggml -lggml-cpu -lggml-base -lstdc++ -lm -lgomp
#include "wrapper.h"
#include <stdlib.h>

extern bool goProgressCallback(float progress, void* user_data);

static inline llama_progress_callback_wrapper get_go_progress_callback() {
	return (llama_progress_callback_wrapper)goProgressCallback;
}
*/
import "C"

func init() {
	C.llama_wrapper_init_logging()
}

var (
	progressCallbackRegistry sync.Map
	progressCallbackCounter  uintptr
	progressCallbackMutex    sync.Mutex
)

func InitLogging() {
	C.llama_wrapper_init_logging()
}

type Model struct {
	modelPtr           unsafe.Pointer // указатель llama_wrapper_model_t* (только веса)
	mu                 sync.RWMutex
	closed             bool
	chatTemplates      unsafe.Pointer // кэшированный common_chat_templates*
	ProgressCallbackID uintptr        // Внутренний ID для очистки progress callback (для тестов)
}

func LoadModel(path string, opts ...ModelOption) (*Model, error) {
	if path == "" {
		return nil, fmt.Errorf("путь к модели не может быть пустым")
	}

	config := defaultModelConfig

	for _, opt := range opts {
		opt(&config)
	}

	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	var cMainGPU *C.char
	if config.mainGPU != "" {
		cMainGPU = C.CString(config.mainGPU)
		defer C.free(unsafe.Pointer(cMainGPU))
	}

	var cTensorSplit *C.char
	if config.tensorSplit != "" {
		cTensorSplit = C.CString(config.tensorSplit)
		defer C.free(unsafe.Pointer(cTensorSplit))
	}

	var cMmproj *C.char
	if config.mmproj != "" {
		cMmproj = C.CString(config.mmproj)
		defer C.free(unsafe.Pointer(cMmproj))
	}

	params := C.llama_wrapper_model_params{
		n_ctx:           0, // Не используется при загрузке модели
		n_batch:         0, // Не используется при загрузке модели
		n_gpu_layers:    C.int(config.gpuLayers),
		n_threads:       0, // Не используется при загрузке модели
		n_threads_batch: 0, // Не используется при загрузке модели
		n_parallel:      0, // Не используется при загрузке модели
		f16_memory:      false,
		mlock:           C.bool(config.mlock),
		mmap:            C.bool(config.mmap),
		embeddings:      false,
		main_gpu:        cMainGPU,
		tensor_split:    cTensorSplit,
		kv_cache_type:   nil,
		flash_attn:      nil,
		mmproj_path:     cMmproj,
	}

	var callbackID uintptr
	if config.progressCallback != nil {
		progressCallbackMutex.Lock()
		progressCallbackCounter++
		callbackID = progressCallbackCounter
		progressCallbackMutex.Unlock()

		progressCallbackRegistry.Store(callbackID, config.progressCallback)

		params.progress_callback = C.get_go_progress_callback()
		params.progress_callback_user_data = unsafe.Pointer(callbackID)
	} else if config.disableProgressCallback {
		params.disable_progress_callback = C.bool(true)
	}

	modelPtr := C.llama_wrapper_model_load(cPath, params)
	if modelPtr == nil {
		if callbackID != 0 {
			progressCallbackRegistry.Delete(callbackID)
		}

		return nil, fmt.Errorf("не удалось загрузить модель: %s", C.GoString(C.llama_wrapper_last_error()))
	}

	model := &Model{
		modelPtr:           modelPtr,
		ProgressCallbackID: callbackID,
	}

	runtime.SetFinalizer(model, (*Model).Close)

	return model, nil
}

func (m *Model) NewContext(opts ...ContextOption) (*Context, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, fmt.Errorf("модель закрыта")
	}

	modelPtr := m.modelPtr
	m.mu.RUnlock()

	config := defaultContextConfig

	for _, opt := range opts {
		opt(&config)
	}

	if config.embeddings && config.nParallel == 1 {
		config.nParallel = 8
	}

	if config.contextSize == 0 {
		nativeContext := int(C.llama_wrapper_get_model_context_length(modelPtr))
		config.contextSize = nativeContext
	}

	if config.batchSize > config.contextSize {
		config.batchSize = config.contextSize
	}

	var cKVCacheType *C.char
	if config.kvCacheType != "" {
		cKVCacheType = C.CString(config.kvCacheType)
		defer C.free(unsafe.Pointer(cKVCacheType))
	}

	var cFlashAttn *C.char
	if config.flashAttn != "" {
		cFlashAttn = C.CString(config.flashAttn)
		defer C.free(unsafe.Pointer(cFlashAttn))
	}

	params := C.llama_wrapper_model_params{
		n_ctx:           C.int(config.contextSize),
		n_batch:         C.int(config.batchSize),
		n_gpu_layers:    0, // Не используется при создании контекста (модель уже загружена)
		n_threads:       C.int(config.threads),
		n_threads_batch: C.int(config.threadsBatch),
		n_parallel:      C.int(config.nParallel),
		f16_memory:      C.bool(config.f16Memory),
		mlock:           false, // Не используется при создании контекста
		mmap:            false, // Не используется при создании контекста
		embeddings:      C.bool(config.embeddings),
		main_gpu:        nil, // Не используется при создании контекста
		tensor_split:    nil, // Не используется при создании контекста
		kv_cache_type:   cKVCacheType,
		flash_attn:      cFlashAttn,
	}

	ctxPtr := C.llama_wrapper_context_create(modelPtr, params)
	if ctxPtr == nil {
		return nil, fmt.Errorf("не удалось создать контекст: %s", C.GoString(C.llama_wrapper_last_error()))
	}

	ctx := &Context{
		contextPtr: ctxPtr,
		model:      m,
		config:     config,
	}

	runtime.SetFinalizer(ctx, (*Context).Close)

	return ctx, nil
}

func (m *Model) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	runtime.SetFinalizer(m, nil)

	if m.ProgressCallbackID != 0 {
		progressCallbackRegistry.Delete(m.ProgressCallbackID)
		m.ProgressCallbackID = 0
	}

	if m.chatTemplates != nil {
		C.llama_wrapper_chat_templates_free(m.chatTemplates)
		m.chatTemplates = nil
	}

	if m.modelPtr != nil {
		C.llama_wrapper_model_free(m.modelPtr)
		m.modelPtr = nil
	}

	m.closed = true
	return nil
}

func (m *Model) HasMTMD() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed || m.modelPtr == nil {
		return false
	}

	return bool(C.llama_wrapper_model_has_mtmd(m.modelPtr))
}

func (m *Model) ChatTemplate() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return ""
	}

	cTemplate := C.llama_wrapper_get_chat_template(m.modelPtr)
	if cTemplate == nil {
		return ""
	}

	return C.GoString(cTemplate)
}

func (m *Model) FormatChatPrompt(messages []ChatMessage, opts ChatOptions) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return "", fmt.Errorf("модель закрыта")
	}

	template := opts.ChatTemplate
	if template == "" {
		template = m.ChatTemplate()
	}
	if template == "" {
		return "", fmt.Errorf("chat-шаблон недоступен: укажите ChatOptions.ChatTemplate или используйте модель со встроенным шаблоном")
	}

	return applyChatTemplate(template, messages, true)
}

func (m *Model) getChatFormat() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.chatTemplates == nil {
		m.chatTemplates = C.llama_wrapper_chat_templates_init(m.modelPtr, nil)
		if m.chatTemplates == nil {
			return int(C.LLAMA_CHAT_FORMAT_CONTENT_ONLY)
		}
	}

	return int(C.llama_wrapper_chat_templates_get_format(m.chatTemplates))
}

func applyChatTemplate(template string, messages []ChatMessage, addAssistant bool) (string, error) {
	if template == "" {
		return "", fmt.Errorf("шаблон не может быть пустым")
	}

	if len(messages) == 0 {
		return "", fmt.Errorf("список сообщений не может быть пустым")
	}

	cTemplate := C.CString(template)
	defer C.free(unsafe.Pointer(cTemplate))

	cRoles := make([]*C.char, len(messages))
	cContents := make([]*C.char, len(messages))

	for i, msg := range messages {
		cRoles[i] = C.CString(msg.Role)
		cContents[i] = C.CString(msg.Content)
	}

	defer func() {
		for i := range messages {
			C.free(unsafe.Pointer(cRoles[i]))
			C.free(unsafe.Pointer(cContents[i]))
		}
	}()

	cResult := C.llama_wrapper_apply_chat_template(
		cTemplate,
		(**C.char)(unsafe.Pointer(&cRoles[0])),
		(**C.char)(unsafe.Pointer(&cContents[0])),
		C.int(len(messages)),
		C.bool(addAssistant),
	)

	if cResult == nil {
		return "", fmt.Errorf("не удалось применить chat-шаблон: %s", C.GoString(C.llama_wrapper_last_error()))
	}

	result := C.GoString(cResult)
	C.llama_wrapper_free_result(cResult)

	return result, nil
}
