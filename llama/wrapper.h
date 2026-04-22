#pragma once

#ifdef __cplusplus
extern "C" {
#endif

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

typedef bool (*llama_progress_callback_wrapper)(float progress, void* user_data);

typedef struct {
    int n_ctx;  // Размер контекста
    int n_batch; // Размер батча
    int n_gpu_layers; // Количество GPU-слоев
    int n_threads;  // Количество потоков для генерации (на токен)
    int n_threads_batch; // Количество потоков для пакетной обработки (промпт)
    int n_parallel; // Количество параллельных последовательностей (для пакетных эмбеддингов)
    bool f16_memory; // Использовать F16 для памяти
    bool mlock; // Блокировка памяти
    bool mmap; // Отображение памяти (mmap)
    bool embeddings; // Включить эмбеддинги
    const char* main_gpu; // Основной gpu
    const char* tensor_split; // Разделение тензоров
    const char* kv_cache_type; // Квантизация KV-кэша - f16, q8_0, q4_0
    const char* flash_attn; // Режим Flash Attention - auto, enabled, disabled
    bool disable_progress_callback; // Для тихой загрузки
    llama_progress_callback_wrapper progress_callback; // Пользовательский callback
    void* progress_callback_user_data; // Пользовательские данные для callback
    const char* mmproj_path; // Опциональный путь к mmproj (GGUF проектор для vision). NULL - только текст
} llama_wrapper_model_params;

// Параметры генерации
typedef struct {
    const char* prompt;
    // 0 = до границы контекста (n_ctx − длина промпта), EOS/стоп-слова - раньше
    int max_tokens;
    int seed;
    const char** stop_words;
    int stop_words_count;

    // Для спекулятивного сэмплирования
    int n_draft;
    bool debug;

    // Дескриптор Go-callback
    uintptr_t callback_handle;

    // Включить переиспользование KV-кэша для совпадающих префиксов
    bool enable_prefix_caching;

    // Базовые параметры сэмплирования
    float temperature;
    int top_k;
    float top_p;
    float min_p;
    float typ_p;
    float top_n_sigma;
    int min_keep;

    // Штрафы за повторения
    int penalty_last_n;
    float penalty_repeat;
    float penalty_freq;
    float penalty_present;

    // DRY-сэмплирование
    float dry_multiplier;
    float dry_base;
    int dry_allowed_length;
    int dry_penalty_last_n;
    const char** dry_sequence_breakers;
    int dry_sequence_breakers_count;

    // Динамическая температура
    float dynatemp_range;
    float dynatemp_exponent;

    // XTC-сэмплирование
    float xtc_probability;
    float xtc_threshold;

    // Mirostat-сэмплирование
    int mirostat;
    float mirostat_tau;
    float mirostat_eta;

    // Прочие параметры
    int n_prev;
    int n_probs;
    bool ignore_eos;
} llama_wrapper_generate_params;

// Callback для потоковой передачи токенов
typedef bool (*llama_wrapper_token_callback)(const char* token);

void llama_wrapper_init_logging();

// Управление моделью
void* llama_wrapper_model_load(const char* model_path, llama_wrapper_model_params params);
void llama_wrapper_model_free(void* model);
// true, если к модели привязан mtmd (загружен mmproj)
bool llama_wrapper_model_has_mtmd(void* model);

// Мультимодальный чат (libmtmd)
// 0 - успех
// отрицательное - ошибка
int llama_wrapper_mtmd_chat_prompt(
    void* ctx,
    void* model,
    const char* chat_template_override,
    int use_jinja,
    const char** roles,
    const char** contents,
    const unsigned char** image_bytes,
    const size_t* image_lens,
    const int* has_image,
    int n_messages,
    llama_wrapper_generate_params gen_params);

// Управление контекстом
void* llama_wrapper_context_create(void* model, llama_wrapper_model_params params);
void llama_wrapper_context_free(void* ctx);

// Генерация текста
char* llama_wrapper_generate(void* ctx, llama_wrapper_generate_params params);
char* llama_wrapper_generate_with_tokens(void* ctx, const int* tokens, int n_tokens, int prefix_len, llama_wrapper_generate_params params);

// Спекулятивная генерация с draft-моделью
char* llama_wrapper_generate_draft(void* ctx_target, void* ctx_draft, llama_wrapper_generate_params params);
char* llama_wrapper_generate_draft_with_tokens(void* ctx_target, void* ctx_draft, const int* tokens, int n_tokens, int target_prefix_len, int draft_prefix_len, llama_wrapper_generate_params params);

// Токенизация
int llama_wrapper_tokenize(void* ctx, const char* text, int* tokens, int max_tokens);

// Токенизация с динамическим выделением памяти (память управляется C)
// Выделяет точный размер под токены - вызывающая сторона должна освободить через llama_wrapper_free_tokens
// tokens - выходной параметр с указателем на выделенный массив токенов
// count - выходной параметр с количеством токенов (или -1 при ошибке)
void llama_wrapper_tokenize_alloc(void* ctx, const char* text, int** tokens, int* count);

// Освобождение токенов, выделенных llama_wrapper_tokenize_alloc
void llama_wrapper_free_tokens(int* tokens);

// Эмбеддинги
int llama_wrapper_embeddings(void* ctx, const char* text, float* embeddings, int max_embeddings);

// Пакетные эмбеддинги - эффективная обработка нескольких текстов
// texts - массив текстовых строк для получения эмбеддингов
// n_texts - количество текстов в массиве
// embeddings - выходной буфер (должен вмещать n_texts * n_embd float-значений)
// n_embd - размерность эмбеддинга модели (llama_model_n_embd)
// Возвращает количество сгенерированных эмбеддингов (должно совпадать с n_texts), либо -1 при ошибке
int llama_wrapper_embeddings_batch(void* ctx, const char** texts, int n_texts, float* embeddings, int n_embd);

// Вспомогательные функции
void llama_wrapper_free_result(char* result);
const char* llama_wrapper_last_error();
int llama_wrapper_get_cached_token_count(void* ctx);

// Получить нативную максимальную длину контекста модели
int llama_wrapper_get_model_context_length(void* model);

// Получить размерность эмбеддинга модели
int llama_wrapper_model_n_embd(void* model);

// Поддержка chat-шаблонов
const char* llama_wrapper_get_chat_template(void* model);
char* llama_wrapper_apply_chat_template(const char* tmpl, const char** roles, const char** contents, int n_messages, bool add_assistant);

// Парсинг reasoning-контента
typedef enum {
    REASONING_FORMAT_NONE = 0,
    REASONING_FORMAT_AUTO = 1,
    REASONING_FORMAT_DEEPSEEK_LEGACY = 2,
    REASONING_FORMAT_DEEPSEEK = 3
} llama_wrapper_reasoning_format;

typedef struct {
    const char* content;
    // NULL, если пусто
    const char* reasoning_content;
} llama_wrapper_parsed_message;

// Разобрать вывод модели, чтобы извлечь reasoning/thinking-контент
// Для стриминга - вызывать с is_partial=true, reasoning_format=DEEPSEEK или AUTO
// Возвращает NULL при ошибке
// Освобождать результат через llama_wrapper_free_parsed_message()
llama_wrapper_parsed_message* llama_wrapper_parse_reasoning(
    const char* text,
    bool is_partial,
    llama_wrapper_reasoning_format format,
    int chat_format
);

void llama_wrapper_free_parsed_message(llama_wrapper_parsed_message* msg);

// Автоопределение chat-формата по метаданным модели
void* llama_wrapper_chat_templates_init(void* model, const char* template_override);
void llama_wrapper_chat_templates_free(void* templates);
int llama_wrapper_chat_templates_get_format(void* templates);

// Константы chat-формата (значения соответствуют enum common_chat_format в llama.cpp/common/chat.h)
#define LLAMA_CHAT_FORMAT_CONTENT_ONLY 0

// Доступ к метаданным модели
const char* llama_wrapper_model_meta_string(void* model, const char* key);
int llama_wrapper_model_meta_count(void* model);

// Информация о gpu
typedef struct {
    int device_id;
    char device_name[256];
    int free_memory_mb;
    int total_memory_mb;
} llama_wrapper_gpu_info;

int llama_wrapper_get_gpu_count();
bool llama_wrapper_get_gpu_info(int device_id, llama_wrapper_gpu_info* info);

// Информация о рантайме модели
typedef struct {
    int n_ctx; // Размер контекста
    int n_batch; // Размер батча
    int kv_cache_size_mb;  // Оценка использования памяти KV-кэша
    int gpu_layers; // Загруженные gpu-слои
    int total_layers; // Общее число слоев в модели
} llama_wrapper_runtime_info;

void llama_wrapper_get_runtime_info(void* model, void* ctx, const char* kv_cache_type, llama_wrapper_runtime_info* info);

#ifdef __cplusplus
}
#endif
