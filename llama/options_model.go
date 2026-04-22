package llama

import "strings"

// WithGPULayers задает количество слоев модели, которые выгружаются на gpu
// Большее значение обычно ускоряет инференс, но требует больше видеопамяти
func WithGPULayers(n int) ModelOption {
	return func(c *modelConfig) {
		c.gpuLayers = n
	}
}

// WithMLock включает блокировку модели в ram
// Это уменьшает риск выгрузки страниц в swap и делает задержку стабильнее
func WithMLock() ModelOption {
	return func(c *modelConfig) {
		c.mlock = true
	}
}

// WithMMap включает или выключает загрузку модели через mmap
// mmap снижает стартовое потребление ram, но зависит от поведения файловой системы
func WithMMap(enabled bool) ModelOption {
	return func(c *modelConfig) {
		c.mmap = enabled
	}
}

// WithMainGPU задает идентификатор основного gpu для размещения весов
// Полезно на системах с несколькими видеокартами
func WithMainGPU(gpu string) ModelOption {
	return func(c *modelConfig) {
		c.mainGPU = gpu
	}
}

// WithTensorSplit задает пропорции разделения тензоров между несколькими gpu
// Позволяет вручную распределить нагрузку по доступной видеопамяти
func WithTensorSplit(split string) ModelOption {
	return func(c *modelConfig) {
		c.tensorSplit = split
	}
}

// WithSilentLoading отключает вызов колбэка прогресса во время загрузки модели
// Удобно для "тихого" запуска без обновления ui/логов прогресса
func WithSilentLoading() ModelOption {
	return func(c *modelConfig) {
		c.disableProgressCallback = true
	}
}

type ProgressCallback func(progress float32) bool

// WithProgressCallback задает функцию, которая получает прогресс загрузки модели
// Возвращаемое значение bool позволяет продолжать или прервать процесс загрузки
func WithProgressCallback(cb ProgressCallback) ModelOption {
	return func(c *modelConfig) {
		c.progressCallback = cb
	}
}

// WithMMProj задаёт путь к GGUF-проектору (mmproj) для vision-моделей; пустая строка - только текст
func WithMMProj(path string) ModelOption {
	return func(c *modelConfig) {
		c.mmproj = strings.TrimSpace(path)
	}
}
