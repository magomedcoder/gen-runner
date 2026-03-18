# LLM Runner

#### Сборка

```bash
# Установка необходимых зависимостей и клонирование llama.cpp
make deps

# Генерация proto
make gen

# Сборка libllama.a (без CUDA)
make build-llama

# Сборка libllama.a с поддержкой NVIDIA (CUDA)
make build-llama-cublas

# Запуск
make run

# Сборка бинарника (CUDA)
make build-nvidia
```
