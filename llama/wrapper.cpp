#include "wrapper.h"
#include "llama.cpp/include/llama.h"
#include "llama.cpp/ggml/include/ggml.h"
#include "llama.cpp/common/common.h"
#include "llama.cpp/common/sampling.h"
#include "llama.cpp/common/speculative.h"
#include "llama.cpp/common/chat.h"
#include "llama.cpp/tools/mtmd/mtmd.h"
#include "llama.cpp/tools/mtmd/mtmd-helper.h"
#include "llama.cpp/vendor/nlohmann/json.hpp"

#include <algorithm>
#include <string>
#include <vector>
#include <memory>
#include <cstring>

#ifdef GGML_USE_CUDA
#include "llama.cpp/ggml/include/ggml-cuda.h"
#endif

// Глобальная обработка ошибок
static std::string g_last_error;

// Глобальное управление уровнем логирования
static ggml_log_level g_min_log_level = GGML_LOG_LEVEL_INFO;

// Колбэк логирования с учетом переменной окружения LLAMA_LOG
static void llama_log_callback(ggml_log_level level, const char * text, void * /*user_data*/) {
    if (level >= g_min_log_level) {
        fprintf(stderr, "%s", text);
    }
}

extern "C" {
void llama_wrapper_init_logging() {
    const char* log_level = std::getenv("LLAMA_LOG");
    if (log_level != nullptr) {
        std::string level_str(log_level);
        if (level_str == "none") {
            g_min_log_level = GGML_LOG_LEVEL_NONE;
        } else if (level_str == "debug") {
            g_min_log_level = GGML_LOG_LEVEL_DEBUG;
        } else if (level_str == "info") {
            g_min_log_level = GGML_LOG_LEVEL_INFO;
        } else if (level_str == "warn") {
            g_min_log_level = GGML_LOG_LEVEL_WARN;
        } else if (level_str == "error") {
            g_min_log_level = GGML_LOG_LEVEL_ERROR;
        }
    }

    llama_log_set(llama_log_callback, nullptr);
}

// Предварительные объявления Go callback-функций
extern bool goTokenCallback(uintptr_t handle, const char* token);

extern bool goProgressCallback(float progress, void* user_data);

// Раздельные обертки для модели и контекста
struct llama_wrapper_model_t {
    llama_model* model;
    // Запрошенное число gpu-слоев (для статистики)
    int n_gpu_layers;
    mtmd_context* mtmd = nullptr;
};

struct llama_wrapper_context_t {
    llama_context* ctx;
    // Ссылка на родительскую модель
    llama_model* model;
    // Кэш для оптимизации сопоставления префикса
    std::vector<int> cached_tokens;
};

const char* llama_wrapper_last_error() {
    return g_last_error.c_str();
}

void llama_wrapper_free_result(char* result) {
    if (result) {
        free(result);
    }
}

// Статический callback-заглушка для тихой загрузки
static bool silent_progress_callback(float progress, void* user_data) {
    (void)progress;
    (void)user_data;

    return true;
}

// Преобразование параметров в параметры модели llama.cpp
static struct llama_model_params convert_model_params(llama_wrapper_model_params params) {
    struct llama_model_params model_params = llama_model_default_params();

    // Устанавливаем n_gpu_layers только если значение не -1 (это означает "по умолчанию/все слои")
    // Значение по умолчанию в llama.cpp - 999, что фактически означает все слои
    if (params.n_gpu_layers != -1) {
        model_params.n_gpu_layers = params.n_gpu_layers;
    }

    model_params.main_gpu = params.main_gpu ? atoi(params.main_gpu) : 0;
    model_params.use_mmap = params.mmap;
    model_params.use_mlock = params.mlock;

    // Использовать host-буферы
    model_params.no_host = false;

    // Настройка progress callback
    if (params.disable_progress_callback) {
        model_params.progress_callback = silent_progress_callback;
        model_params.progress_callback_user_data = nullptr;
    } else if (params.progress_callback) {
        model_params.progress_callback = params.progress_callback;
        model_params.progress_callback_user_data = params.progress_callback_user_data;
    }

    // Иначе NULL -> llama.cpp установит стандартный вывод точек

    return model_params;
}

// Преобразование параметров в параметры контекста llama.cpp
static struct llama_context_params convert_context_params(llama_wrapper_model_params params) {
    struct llama_context_params ctx_params = llama_context_default_params();
    ctx_params.n_ctx = params.n_ctx > 0 ? params.n_ctx : 2048;
    ctx_params.n_batch = params.n_batch > 0 ? params.n_batch : 512;
    ctx_params.n_threads = params.n_threads > 0 ? params.n_threads : 4;
    ctx_params.n_threads_batch = params.n_threads_batch > 0 ? params.n_threads_batch : ctx_params.n_threads;
    ctx_params.n_seq_max = params.n_parallel > 0 ? params.n_parallel : 1;
    ctx_params.embeddings = params.embeddings;

    // Настройка типа квантизации KV-кэша
    if (params.kv_cache_type != nullptr) {
        std::string cache_type(params.kv_cache_type);
        if (cache_type == "f16") {
            ctx_params.type_k = GGML_TYPE_F16;
            ctx_params.type_v = GGML_TYPE_F16;
        } else if (cache_type == "q8_0") {
            ctx_params.type_k = GGML_TYPE_Q8_0;
            ctx_params.type_v = GGML_TYPE_Q8_0;
        } else if (cache_type == "q4_0") {
            ctx_params.type_k = GGML_TYPE_Q4_0;
            ctx_params.type_v = GGML_TYPE_Q4_0;
        }

        // Если значение не распознано, оставляем по умолчанию (f16)
    }

    // Настройка режима Flash Attention
    if (params.flash_attn != nullptr) {
        std::string fa_mode(params.flash_attn);
        if (fa_mode == "enabled") {
            ctx_params.flash_attn_type = LLAMA_FLASH_ATTN_TYPE_ENABLED;
        } else if (fa_mode == "disabled") {
            ctx_params.flash_attn_type = LLAMA_FLASH_ATTN_TYPE_DISABLED;
        } else if (fa_mode == "auto") {
            ctx_params.flash_attn_type = LLAMA_FLASH_ATTN_TYPE_AUTO;
        }

        // Если значение не распознано, оставляем по умолчанию (auto)
    }

    return ctx_params;
}

void* llama_wrapper_model_load(const char* model_path, llama_wrapper_model_params params) {
    if (!model_path) {
        g_last_error = "Путь к модели не может быть пустым";
        return nullptr;
    }

    try {
        // Инициализация llama
        llama_backend_init();

        // Загрузка модели (только весов)
        auto model_params = convert_model_params(params);
        llama_model* model = llama_model_load_from_file(model_path, model_params);
        if (!model) {
            g_last_error = "Не удалось загрузить модель из: " + std::string(model_path);

            return nullptr;
        }

        // Создание обертки (только модель, без контекста)
        auto wrapper = new llama_wrapper_model_t();
        wrapper->model = model;

        // Сохраняем n_gpu_layers для статистики
        // Если передано -1 (означает "по умолчанию"), llama.cpp использует 999 слоев
        wrapper->n_gpu_layers = (params.n_gpu_layers == -1) ? 999 : params.n_gpu_layers;

        wrapper->mtmd = nullptr;
        if (params.mmproj_path && params.mmproj_path[0] != '\0') {
            mtmd_context_params mparams = mtmd_context_params_default();
            mparams.use_gpu = true;
            mparams.print_timings = false;
            if (params.n_threads > 0) {
                mparams.n_threads = params.n_threads;
            }

            if (params.flash_attn != nullptr) {
                std::string fa_mode(params.flash_attn);
                if (fa_mode == "enabled") {
                    mparams.flash_attn_type = LLAMA_FLASH_ATTN_TYPE_ENABLED;
                } else if (fa_mode == "disabled") {
                    mparams.flash_attn_type = LLAMA_FLASH_ATTN_TYPE_DISABLED;
                } else {
                    mparams.flash_attn_type = LLAMA_FLASH_ATTN_TYPE_AUTO;
                }
            }

            wrapper->mtmd = mtmd_init_from_file(params.mmproj_path, model, mparams);
            if (!wrapper->mtmd) {
                llama_model_free(model);
                delete wrapper;
                g_last_error = "Не удалось загрузить mmproj из: " + std::string(params.mmproj_path);
                return nullptr;
            }
        }

        return wrapper;
    } catch (const std::exception& e) {
        g_last_error = "Exception loading model: " + std::string(e.what());
        return nullptr;
    }
}

void llama_wrapper_model_free(void* model) {
    if (!model) {
        return;
    }

    auto wrapper = static_cast<llama_wrapper_model_t*>(model);
    if (wrapper->mtmd) {
        mtmd_free(wrapper->mtmd);
        wrapper->mtmd = nullptr;
    }
    if (wrapper->model) {
        llama_model_free(wrapper->model);

        // Предотвращение двойного освобождения
        wrapper->model = nullptr;
    }

    delete wrapper;
}

bool llama_wrapper_model_has_mtmd(void* model) {
    if (!model) {
        return false;
    }

    auto wrapper = static_cast<llama_wrapper_model_t*>(model);
    return wrapper->mtmd != nullptr;
}

void* llama_wrapper_context_create(void* model, llama_wrapper_model_params params) {
    if (!model) {
        g_last_error = "Модель не может быть пустой";
        return nullptr;
    }

    try {
        auto model_wrapper = static_cast<llama_wrapper_model_t*>(model);

        // Создание контекста из модели
        auto ctx_params = convert_context_params(params);
        llama_context* ctx = llama_init_from_model(model_wrapper->model, ctx_params);
        if (!ctx) {
            g_last_error = "Не удалось создать контекст";
            return nullptr;
        }

        // Создание обертки контекста
        auto ctx_wrapper = new llama_wrapper_context_t();
        ctx_wrapper->ctx = ctx;

        // Сохраняем ссылку на родительскую модель
        ctx_wrapper->model = model_wrapper->model;

        return ctx_wrapper;
    } catch (const std::exception& e) {
        g_last_error = "Исключение при создании контекста: " + std::string(e.what());
        return nullptr;
    }
}

void llama_wrapper_context_free(void* ctx) {
    if (!ctx) {
        return;
    }

    auto wrapper = static_cast<llama_wrapper_context_t*>(ctx);
    if (wrapper->ctx) {
        llama_free(wrapper->ctx);

        // Предотвращение двойного освобождения
        wrapper->ctx = nullptr;
    }
    
    delete wrapper;
}

// Получить нативную максимальную длину контекста модели из метаданных GGUF
int llama_wrapper_get_model_context_length(void* model) {
    if (!model) {

        // Запасной вариант, если модель равна null
        return 32768;
    }

    auto model_wrapper = static_cast<llama_wrapper_model_t*>(model);

    // Запрос нативной длины контекста модели из метаданных GGUF
    int n_ctx_train = llama_model_n_ctx_train(model_wrapper->model);

    // Возвращаем тренировочный контекст модели или разумное значение по умолчанию
    return (n_ctx_train > 0) ? n_ctx_train : 32768;
}

// Получить размерность эмбеддинга модели
int llama_wrapper_model_n_embd(void* model) {
    if (!model) {
        // Ошибка, если модель равна null
        return -1;
    }

    auto model_wrapper = static_cast<llama_wrapper_model_t*>(model);
    return llama_model_n_embd(model_wrapper->model);
}

// Вспомогательная функция для поиска длины общего префикса двух векторов токенов
static int findCommonPrefix(const std::vector<int>& a, const std::vector<int>& b) {
    int commonLen = 0;
    size_t minLen = std::min(a.size(), b.size());
    for (size_t i = 0; i < minLen; i++) {
        if (a[i] != b[i]) {
            break;
        }

        commonLen++;
    }

    return commonLen;
}

char* llama_wrapper_generate_with_tokens(void* ctx, const int* tokens, int n_tokens, int prefix_len, llama_wrapper_generate_params params) {
    if (!ctx || !tokens) {
        g_last_error = "Контекст и токены не могут быть пустыми";
        return nullptr;
    }

    auto wrapper = static_cast<llama_wrapper_context_t*>(ctx);
    if (!wrapper->ctx) {
        g_last_error = "Контекст освобожден";
        return nullptr;
    }

    try {
        // Преобразование C-токенов в вектор
        std::vector<llama_token> prompt_tokens(tokens, tokens + n_tokens);

        if (prompt_tokens.empty()) {
            g_last_error = "Массив токенов пуст";
            return nullptr;
        }

        // Проверка размера контекста с запасом ДО изменения KV-кэша
        int available_ctx = llama_n_ctx(wrapper->ctx);
        if (available_ctx <= 0) {
            g_last_error = "Invalid context size";
            return nullptr;
        }

        // Промпт + бюджет генерации не должны превышать n_ctx
        int prompt_len = (int)prompt_tokens.size();
        int gen_room_preview = available_ctx - prompt_len;
        if (gen_room_preview < 1) {
            g_last_error = "Подсказка слишком длинная для размера контекста (для генерации требуется как минимум 1 токен)";
            return nullptr;
        }
        if (params.max_tokens > 0 && prompt_len + params.max_tokens > available_ctx) {
            char err_msg[512];
            snprintf(err_msg, sizeof(err_msg), R"(Подсказка слишком длинная для размера контекста: требуется %d токенов (%d подсказка + %d генерация), но контекст содержит только %d токенов)", prompt_len + params.max_tokens, prompt_len, params.max_tokens, available_ctx);
            g_last_error = err_msg;

            return nullptr;
        }

        // Очистка KV-кэша от точки расхождения и далее
        // Для полного попадания в кэш обновляем последний токен промпта, поэтому чистим с prefix_len - 1
        // Для частичного совпадения чистим с prefix_len как обычно
        int clear_from = (prefix_len == n_tokens && n_tokens > 0) ? prefix_len - 1 : prefix_len;

        // Очищаем только если clear_from валиден и в пределах контекста
        if (clear_from >= 0 && clear_from < available_ctx) {
            llama_memory_seq_rm(llama_get_memory(wrapper->ctx), 0, clear_from, -1);
        }

        // Создание параметров сэмплирования - используем структуру напрямую вместо вызова функции
        common_params_sampling sampling_params;

        // Базовое сэмплирование
        sampling_params.seed = params.seed;
        sampling_params.temp = params.temperature;
        sampling_params.top_k = params.top_k;
        sampling_params.top_p = params.top_p;
        sampling_params.min_p = params.min_p;
        sampling_params.typ_p = params.typ_p;
        sampling_params.top_n_sigma = params.top_n_sigma;
        sampling_params.min_keep = params.min_keep;

        // Штрафы за повторения
        sampling_params.penalty_last_n = params.penalty_last_n;
        sampling_params.penalty_repeat = params.penalty_repeat;
        sampling_params.penalty_freq = params.penalty_freq;
        sampling_params.penalty_present = params.penalty_present;

        // DRY-сэмплирование
        sampling_params.dry_multiplier = params.dry_multiplier;
        sampling_params.dry_base = params.dry_base;
        sampling_params.dry_allowed_length = params.dry_allowed_length;
        sampling_params.dry_penalty_last_n = params.dry_penalty_last_n;

        // Преобразование dry_sequence_breakers из C-массива в std::vector
        sampling_params.dry_sequence_breakers.clear();
        for (int i = 0; i < params.dry_sequence_breakers_count; i++) {
            sampling_params.dry_sequence_breakers.push_back(std::string(params.dry_sequence_breakers[i]));
        }

        // Динамическая температура
        sampling_params.dynatemp_range = params.dynatemp_range;
        sampling_params.dynatemp_exponent = params.dynatemp_exponent;

        // XTC-сэмплирование
        sampling_params.xtc_probability = params.xtc_probability;
        sampling_params.xtc_threshold = params.xtc_threshold;

        // Mirostat-сэмплирование
        sampling_params.mirostat = params.mirostat;
        sampling_params.mirostat_tau = params.mirostat_tau;
        sampling_params.mirostat_eta = params.mirostat_eta;

        // Другие параметры
        sampling_params.n_prev = params.n_prev;
        sampling_params.n_probs = params.n_probs;
        sampling_params.ignore_eos = params.ignore_eos;

        // Инициализация сэмплера
        common_sampler* sampler = common_sampler_init(wrapper->model, sampling_params);
        if (!sampler) {
            g_last_error = "Не удалось инициализировать сэмплер";
            return nullptr;
        }

        // Валидация параметров генерации
        // 0 = заполнить оставшийся контекст; отрицательные значения запрещены
        if (params.max_tokens < 0) {
            common_sampler_free(sampler);
            g_last_error = "Недопустимое значение max_tokens (должно быть >= 0)";

            return nullptr;
        }

        // После очистки кэша с prefix_len и далее, кэш заканчивается на prefix_len - 1
        // Следующая позиция для использования - prefix_len
        int n_past = prefix_len;

        // Обработка токенов промпта с prefix_len и далее с явным указанием позиций
        if (prefix_len < n_tokens) {
            int tokens_to_process = n_tokens - prefix_len;
            int n_batch = llama_n_batch(wrapper->ctx);

            // Обрабатываем токены чанками с учетом ограничения n_batch
            for (int chunk_start = 0; chunk_start < tokens_to_process; chunk_start += n_batch) {
                int chunk_size = std::min(n_batch, tokens_to_process - chunk_start);
                llama_batch batch = llama_batch_init(chunk_size, 0, 1);
                common_batch_clear(batch);

                // Добавляем токены этого чанка с явными позициями
                for (int i = 0; i < chunk_size; i++) {
                    int token_idx = prefix_len + chunk_start + i;
                    int position = prefix_len + chunk_start + i;

                    // Логиты нужны только для самого последнего токена всего промпта
                    bool needs_logits = (chunk_start + i == tokens_to_process - 1);
                    common_batch_add(batch, prompt_tokens[token_idx], position, { 0 }, needs_logits);
                }

                if (llama_decode(wrapper->ctx, batch) != 0) {
                    if (params.debug) {
                        fprintf(stderr, "ЧТО-ТО: декодирование запроса не удалось для фрагмента, начинающегося с %d\n", chunk_start);
                    }

                    llama_batch_free(batch);
                    common_sampler_free(sampler);
                    g_last_error = "Не удалось расшифровать подсказку";

                    return nullptr;
                }

                llama_batch_free(batch);
            }

            // Позиция теперь в конце промпта
            n_past = n_tokens;

        } else if (prefix_len == n_tokens && n_tokens > 0) {
            // Полное попадание в кэш - обновляем логиты последнего токена для детерминизма
            // Это критично - иначе сэмплирование идет по устаревшим логитам от предыдущей генерации
            // Последний токен промпта находится на позиции n_tokens - 1 (индексация с 0)
            llama_batch batch = llama_batch_init(512, 0, 1);
            common_batch_clear(batch);
            common_batch_add(batch, prompt_tokens[n_tokens - 1], n_tokens - 1, { 0 }, true);

            if (llama_decode(wrapper->ctx, batch) != 0) {
                if (params.debug) {
                    fprintf(stderr, "ЧТО-ТО: обновление логит не удалось\n");
                }

                llama_batch_free(batch);
                common_sampler_free(sampler);
                g_last_error = "Не удалось обновить логит для кэшированного запроса";

                return nullptr;
            }

            llama_batch_free(batch);
            // Устанавливаем позицию в конец промпта для генерации
            n_past = n_tokens;
        }

        // Если n_tokens == 0, декодировать нечего

        int gen_room = available_ctx - n_past;
        if (gen_room < 1) {
            common_sampler_free(sampler);
            g_last_error = "Нет места в контексте для генерации";
            return nullptr;
        }

        int n_predict = params.max_tokens > 0 ? std::min(params.max_tokens, gen_room) : gen_room;

        // Цикл генерации - повторяет шаблон simple.cpp
        std::string result;
        int n_decode = 0;

        if (params.debug) {
            fprintf(stderr, "Запуск цикла генерации, n_predict=%d, n_past=%d\n", n_predict, n_past);
        }

        // Основной цикл генерации - сначала decode, затем sample
        for (int n_gen = 0; n_gen < n_predict; n_gen++) {
            if (params.debug && n_gen == 0) {
                fprintf(stderr, "Первая итерация, скоро будет записан образец\n");
            }

            // Сэмплируем следующий токен (используя логиты предыдущего decode или промпта)
            llama_token new_token_id = common_sampler_sample(sampler, wrapper->ctx, -1);

            if (params.debug && n_gen == 0) {
                fprintf(stderr, "Образец токена: %d\n", new_token_id);
            }

            // Проверка EOS
            if (llama_vocab_is_eog(llama_model_get_vocab(wrapper->model), new_token_id)) {
                if (params.debug) {
                    fprintf(stderr, "ИНФО: Обнаружен конец генерации токена\n");
                }
                break;
            }

            if (params.debug && n_gen == 0) {
                fprintf(stderr, "Преобразование токена в текст\n");
            }

            // Преобразование токена в текст
            std::string token_str = common_token_to_piece(wrapper->ctx, new_token_id);

            if (params.debug && n_gen == 0) {
                fprintf(stderr, "Текст токена: '%s'\n", token_str.c_str());
            }

            // Вызов callback при наличии
            if (params.callback_handle != 0) {
                if (!goTokenCallback(params.callback_handle, token_str.c_str())) {
                    if (params.debug) {
                        fprintf(stderr, "ИНФО: Генерация остановлена функцией обратного вызова\n");
                    }
                    break;
                }
            }

            result += token_str;

            // Проверка стоп-слов
            for (int j = 0; j < params.stop_words_count; j++) {
                if (result.find(params.stop_words[j]) != std::string::npos) {
                    if (params.debug) {
                        fprintf(stderr, "ИНФО: Обнаружено стоп-слово, завершающее генерирование\n");
                    }

                    goto generation_done;
                }
            }

            if (params.debug && n_gen == 0) {
                // Запрашиваем фактическое состояние кэша перед decode
                int cache_pos = llama_memory_seq_pos_max(llama_get_memory(wrapper->ctx), 0);
                fprintf(stderr, "Сейчас будет декодирован токен, n_past=%d, cache_pos_max=%d\n", n_past, cache_pos);
            }

            // Декодируем сэмплированный токен, чтобы получить логиты для следующей итерации
            // Выделяем достаточно места под batch (минимум 512 токенов)
            llama_batch gen_batch = llama_batch_init(512, 0, 1);
            common_batch_clear(gen_batch);
            common_batch_add(gen_batch, new_token_id, n_past, { 0 }, true);

            if (params.debug && n_gen == 0) {
                fprintf(stderr, "Пакет токенов=%d, позиция=%d, n_токенов=%d\n", new_token_id, n_past, gen_batch.n_tokens);
            }

            // Увеличиваем позицию для следующей итерации
            n_past++;

            if (params.debug && n_gen == 0) {
                fprintf(stderr, "Пакет подготовлен, вызывается функция llama_decode\n");
            }

            if (llama_decode(wrapper->ctx, gen_batch) != 0) {
                if (params.debug) {
                    fprintf(stderr, "ЧТО-ТО: декодирование не удалось, генерация остановлена.n\n");
                }

                llama_batch_free(gen_batch);

                break;
            }

            if (params.debug && n_gen == 0) {
                fprintf(stderr, "Расшифровка прошла успешно, освобождение пакета\n");
            }

            llama_batch_free(gen_batch);
            n_decode += 1;

            if (params.debug && n_gen == 0) {
                fprintf(stderr, "Первая итерация завершена\n");
            }
        }

generation_done:
        common_sampler_free(sampler);

        // Возвращаем выделенную строку (вызывающий код должен освободить память)
        char* c_result = (char*)malloc(result.length() + 1);
        if (c_result) {
            memcpy(c_result, result.c_str(), result.length());
            c_result[result.length()] = '\0';
        } else {
            g_last_error = "Не удалось выделить память для результата";
        }

        return c_result;

    } catch (const std::exception& e) {
        g_last_error = "Исключение во время генерации: " + std::string(e.what());
        return nullptr;
    }
}

// Простая обертка: токенизирует промпт и автоматически обрабатывает префиксный кэш
char* llama_wrapper_generate(void* ctx, llama_wrapper_generate_params params) {
    if (!ctx) {
        g_last_error = "Контекст не может быть нулевым";
        return nullptr;
    }

    auto wrapper = static_cast<llama_wrapper_context_t*>(ctx);
    if (!wrapper->ctx) {
        g_last_error = "Контекст освобожден";
        return nullptr;
    }

    try {
        // Токенизация промпта
        std::vector<llama_token> prompt_tokens = common_tokenize(wrapper->ctx, params.prompt, true, true);

        if (prompt_tokens.empty()) {
            g_last_error = "Не удалось разбить подсказку на токены";
            return nullptr;
        }

        // Преобразование в вектор int для сравнения
        std::vector<int> tokens_int(prompt_tokens.begin(), prompt_tokens.end());

        // Поиск общего префикса с кэшированными токенами (если включен префиксный кэш)
        int prefix_len = params.enable_prefix_caching
            ? findCommonPrefix(wrapper->cached_tokens, tokens_int)
            : 0;

        // Обновление кэша новой последовательностью токенов (если включен префиксный кэш)
        if (params.enable_prefix_caching) {
            wrapper->cached_tokens = tokens_int;
        } else {
            // Гарантируем, что кэш пуст при отключении
            wrapper->cached_tokens.clear();
        }

        // Вызов генерации по токенам с префиксным кэшированием
        return llama_wrapper_generate_with_tokens(ctx, tokens_int.data(), tokens_int.size(), prefix_len, params);

    } catch (const std::exception& e) {
        g_last_error = "Исключение во время генерации: " + std::string(e.what());
        return nullptr;
    }
}

char* llama_wrapper_generate_draft_with_tokens(void* ctx_target, void* ctx_draft, const int* tokens, int n_tokens, int target_prefix_len, int draft_prefix_len, llama_wrapper_generate_params params) {
    if (!ctx_target || !ctx_draft || !tokens) {
        g_last_error = "Цель, черновой контекст и токены не могут быть пустыми";
        return nullptr;
    }

    auto wrapper_tgt = static_cast<llama_wrapper_context_t*>(ctx_target);
    auto wrapper_dft = static_cast<llama_wrapper_context_t*>(ctx_draft);
    if (!wrapper_tgt->ctx) {
        g_last_error = "Целевой контекст освобожден";
        return nullptr;
    }

    if (!wrapper_dft->ctx) {
        g_last_error = "Контекст черновика освобожден";
        return nullptr;
    }

    try {
        // Очистка KV-кэша от точек расхождения
        // Идентификатор последовательности 0 - значение по умолчанию для однопоследовательного инференса
        // Для спекулятивной генерации при полном попадании в кэш нужно обновить предпоследний токен
        int target_clear_from = (target_prefix_len == n_tokens && n_tokens > 1) ? n_tokens - 2 : target_prefix_len;
        int draft_clear_from = (draft_prefix_len == n_tokens && n_tokens > 1) ? n_tokens - 2 : draft_prefix_len;
        llama_memory_seq_rm(llama_get_memory(wrapper_tgt->ctx), 0, target_clear_from, -1);
        llama_memory_seq_rm(llama_get_memory(wrapper_dft->ctx), 0, draft_clear_from, -1);

        // Преобразование C-токенов в вектор
        std::vector<llama_token> prompt_tokens(tokens, tokens + n_tokens);

        if (prompt_tokens.empty()) {
            g_last_error = "Массив токенов пуст";
            return nullptr;
        }

        // Инициализация спекулятивного сэмплирования
        common_speculative* spec = common_speculative_init(wrapper_tgt->ctx, wrapper_dft->ctx);
        if (!spec) {
            g_last_error = "Не удалось инициализировать спекулятивную выборку";
            return nullptr;
        }

        // Настройка параметров
        common_speculative_params spec_params;
        spec_params.n_draft = params.n_draft > 0 ? params.n_draft : 16;
        spec_params.p_min = 0.75f;

        // Создание параметров сэмплирования
        common_params_sampling sampling_params;

        // Базовое сэмплирование
        sampling_params.seed = params.seed;
        sampling_params.temp = params.temperature;
        sampling_params.top_k = params.top_k;
        sampling_params.top_p = params.top_p;
        sampling_params.min_p = params.min_p;
        sampling_params.typ_p = params.typ_p;
        sampling_params.top_n_sigma = params.top_n_sigma;
        sampling_params.min_keep = params.min_keep;

        // Штрафы за повторения
        sampling_params.penalty_last_n = params.penalty_last_n;
        sampling_params.penalty_repeat = params.penalty_repeat;
        sampling_params.penalty_freq = params.penalty_freq;
        sampling_params.penalty_present = params.penalty_present;

        // DRY-сэмплирование
        sampling_params.dry_multiplier = params.dry_multiplier;
        sampling_params.dry_base = params.dry_base;
        sampling_params.dry_allowed_length = params.dry_allowed_length;
        sampling_params.dry_penalty_last_n = params.dry_penalty_last_n;

        // Преобразование dry_sequence_breakers из C-массива в std::vector
        sampling_params.dry_sequence_breakers.clear();
        for (int i = 0; i < params.dry_sequence_breakers_count; i++) {
            sampling_params.dry_sequence_breakers.push_back(std::string(params.dry_sequence_breakers[i]));
        }

        // Динамическая температура
        sampling_params.dynatemp_range = params.dynatemp_range;
        sampling_params.dynatemp_exponent = params.dynatemp_exponent;

        // XTC-сэмплирование
        sampling_params.xtc_probability = params.xtc_probability;
        sampling_params.xtc_threshold = params.xtc_threshold;

        // Mirostat-сэмплирование
        sampling_params.mirostat = params.mirostat;
        sampling_params.mirostat_tau = params.mirostat_tau;
        sampling_params.mirostat_eta = params.mirostat_eta;

        // Другие параметры
        sampling_params.n_prev = params.n_prev;
        sampling_params.n_probs = params.n_probs;
        sampling_params.ignore_eos = params.ignore_eos;

        // Инициализация сэмплера
        common_sampler* sampler = common_sampler_init(wrapper_tgt->model, sampling_params);
        if (!sampler) {
            common_speculative_free(spec);
            g_last_error = "Не удалось инициализировать сэмплер";
            return nullptr;
        }

        // Вычисляем промпт (все, кроме последнего токена), но обрабатываем только токены после target_prefix_len
        // Если target_prefix_len равен или больше позиции последнего токена, декодирование не требуется
        if (prompt_tokens.size() > 1 && target_prefix_len < (int)prompt_tokens.size() - 1) {
            // Обрабатываем токены от target_prefix_len до size - 1
            int tokens_to_process = prompt_tokens.size() - 1 - target_prefix_len;
            int n_batch = llama_n_batch(wrapper_tgt->ctx);

            // Обрабатываем токены чанками с учетом ограничения n_batch
            for (int chunk_start = 0; chunk_start < tokens_to_process; chunk_start += n_batch) {
                int chunk_size = std::min(n_batch, tokens_to_process - chunk_start);
                llama_batch batch = llama_batch_init(chunk_size, 0, 1);
                common_batch_clear(batch);

                // Добавляем токены этого чанка с явными позициями
                for (int i = 0; i < chunk_size; i++) {
                    int token_idx = target_prefix_len + chunk_start + i;
                    // Логиты нужны только для самого последнего токена всего промпта
                    bool needs_logits = (chunk_start + i == tokens_to_process - 1);
                    common_batch_add(batch, prompt_tokens[token_idx], token_idx, { 0 }, needs_logits);
                }

                if (llama_decode(wrapper_tgt->ctx, batch) != 0) {
                    llama_batch_free(batch);
                    common_sampler_free(sampler);
                    common_speculative_free(spec);
                    g_last_error = "Не удалось расшифровать подсказку";
                    return nullptr;
                }

                llama_batch_free(batch);
            }
        } else if (target_prefix_len == (int)prompt_tokens.size() && prompt_tokens.size() > 1) {
            // Полное попадание в кэш - обновляем предпоследний токен для детерминизма
            // Это соответствует шаблону, где декодируются все токены кроме последнего
            llama_batch batch = llama_batch_init(512, 0, 1);
            common_batch_clear(batch);
            common_batch_add(batch, prompt_tokens[prompt_tokens.size() - 2], prompt_tokens.size() - 2, { 0 }, true);

            if (llama_decode(wrapper_tgt->ctx, batch) != 0) {
                if (params.debug) {
                    fprintf(stderr, "ЧТО-ТО: спекулятивный запрос логит обновление не удалось\n");
                }

                llama_batch_free(batch);
                common_sampler_free(sampler);
                common_speculative_free(spec);
                g_last_error = "Не удалось обновить логити для кэшированного спекулятивного запроса";
                return nullptr;
            }
            llama_batch_free(batch);
        }

        // Переменные генерации
        std::string result;
        llama_token last_token = prompt_tokens.back();
        llama_tokens prompt_tgt(prompt_tokens.begin(), prompt_tokens.end() - 1);
        int n_past = prompt_tokens.size() - 1;
        if (params.max_tokens < 0) {
            common_sampler_free(sampler);
            common_speculative_free(spec);
            g_last_error = "Недопустимое значение max_tokens (должно быть >= 0)";
            return nullptr;
        }

        int available_ctx_tgt = llama_n_ctx(wrapper_tgt->ctx);
        int gen_room_spec = available_ctx_tgt - (int)prompt_tokens.size();
        if (gen_room_spec < 1) {
            common_sampler_free(sampler);
            common_speculative_free(spec);
            g_last_error = "Нет места в контексте для генерации";
            return nullptr;
        }

        int n_predict = params.max_tokens > 0 ? std::min(params.max_tokens, gen_room_spec) : gen_room_spec;

        llama_batch batch_tgt = llama_batch_init(llama_n_batch(wrapper_tgt->ctx), 0, 1);

        // Цикл генерации
        while (result.length() < (size_t)n_predict) {
            // Генерируем draft-токены
            llama_tokens draft = common_speculative_gen_draft(spec, spec_params, prompt_tgt, last_token);

            // Готовим batch с последним токеном и draft
            common_batch_clear(batch_tgt);
            common_batch_add(batch_tgt, last_token, n_past, { 0 }, true);

            for (size_t i = 0; i < draft.size(); ++i) {
                common_batch_add(batch_tgt, draft[i], n_past + i + 1, { 0 }, true);
            }

            // Выполняем вычисление на целевой модели
            if (llama_decode(wrapper_tgt->ctx, batch_tgt) != 0) {
                if (params.debug) {
                    fprintf(stderr, "ЧТО-ТО: декодирование целевого объекта не удалось, остановка.\n");
                }

                break;
            }

            // Сэмплируем и принимаем токены
            const auto ids = common_sampler_sample_and_accept_n(sampler, wrapper_tgt->ctx, draft);

            if (ids.empty()) {
                break;
            }

            // Обрабатываем принятые токены - учитываем фактическое количество при раннем завершении
            size_t tokens_processed = 0;
            bool early_termination = false;

            for (size_t i = 0; i < ids.size(); ++i) {
                const llama_token id = ids[i];

                // Проверка EOS
                if (llama_vocab_is_eog(llama_model_get_vocab(wrapper_tgt->model), id)) {
                    early_termination = true;
                    break;
                }

                const std::string token_str = common_token_to_piece(wrapper_tgt->ctx, id);

                // Вызов callback при наличии
                if (params.callback_handle != 0) {
                    if (!goTokenCallback(params.callback_handle, token_str.c_str())) {
                        early_termination = true;
                        break;
                    }
                }

                result += token_str;
                prompt_tgt.push_back(id);
                tokens_processed++;

                // Проверка стоп-слов
                for (int j = 0; j < params.stop_words_count; j++) {
                    if (result.find(params.stop_words[j]) != std::string::npos) {
                        early_termination = true;
                        goto early_exit;
                    }
                }
            }

early_exit:
            // Обновляем позицию на основе реально обработанных токенов
            if (early_termination) {
                n_past += tokens_processed;
                if (params.debug) {
                    fprintf(stderr, "Досрочное прекращение после обработки токенов %zu/%zu\n", tokens_processed, ids.size());
                }
            } else {
                n_past += ids.size();
            }

            // Очищаем из KV-кэша непринятые/необработанные токены
            // Это удаляет все от позиции n_past и далее, гарантируя, что кэш содержит только токены, которые действительно обработаны и приняты
            llama_memory_seq_rm(llama_get_memory(wrapper_tgt->ctx), 0, n_past, -1);

            // Обновляем последний токен для следующей итерации
            if (tokens_processed > 0) {
                // Используем последний реально обработанный токен
                last_token = prompt_tgt[prompt_tgt.size() - 1];
            }

            // Выходим при раннем завершении
            if (early_termination) {
                break;
            }
        }

        llama_batch_free(batch_tgt);
        common_sampler_free(sampler);
        common_speculative_free(spec);

        // Возвращаем выделенную строку
        char* c_result = (char*)malloc(result.length() + 1);
        if (c_result) {
            memcpy(c_result, result.c_str(), result.length());
            c_result[result.length()] = '\0';
        } else {
            g_last_error = "Не удалось выделить память для результата";
        }

        return c_result;

    } catch (const std::exception& e) {
        g_last_error = "Исключение во время спекулятивной генерации: " + std::string(e.what());
        return nullptr;
    }
}

// Простая обертка - токенизирует промпт и автоматически обрабатывает префиксный кэш для обеих моделей
char* llama_wrapper_generate_draft(void* ctx_target, void* ctx_draft, llama_wrapper_generate_params params) {
    if (!ctx_target || !ctx_draft) {
        g_last_error = "Целевой и черновой контексты не могут быть пустыми";
        return nullptr;
    }

    auto wrapper_tgt = static_cast<llama_wrapper_context_t*>(ctx_target);
    auto wrapper_dft = static_cast<llama_wrapper_context_t*>(ctx_draft);
    if (!wrapper_tgt->ctx) {
        g_last_error = "Целевой контекст освобожден";
        return nullptr;
    }

    if (!wrapper_dft->ctx) {
        g_last_error = "Контекст черновика освобожден";
        return nullptr;
    }

    try {
        // Токенизация промпта
        std::vector<llama_token> prompt_tokens = common_tokenize(wrapper_tgt->ctx, params.prompt, true, true);

        if (prompt_tokens.empty()) {
            g_last_error = "Не удалось разбить подсказку на токены";
            return nullptr;
        }

        // Преобразование в вектор int для сравнения
        std::vector<int> tokens_int(prompt_tokens.begin(), prompt_tokens.end());

        // Поиск общего префикса для обоих контекстов (если включен префиксный кэш)
        int target_prefix_len = params.enable_prefix_caching
            ? findCommonPrefix(wrapper_tgt->cached_tokens, tokens_int)
            : 0;
        int draft_prefix_len = params.enable_prefix_caching
            ? findCommonPrefix(wrapper_dft->cached_tokens, tokens_int)
            : 0;

        // Обновление обоих кэшей новой последовательностью токенов (если включен префиксный кэш)
        if (params.enable_prefix_caching) {
            wrapper_tgt->cached_tokens = tokens_int;
            wrapper_dft->cached_tokens = tokens_int;
        } else {
            // Гарантируем, что кэш пуст при отключении
            wrapper_tgt->cached_tokens.clear();
            wrapper_dft->cached_tokens.clear();
        }

        // Вызов спекулятивной генерации по токенам с префиксным кэшированием
        return llama_wrapper_generate_draft_with_tokens(ctx_target, ctx_draft, tokens_int.data(), tokens_int.size(), target_prefix_len, draft_prefix_len, params);

    } catch (const std::exception& e) {
        g_last_error = "Исключение во время спекулятивной генерации: " + std::string(e.what());
        return nullptr;
    }
}

int llama_wrapper_tokenize(void* ctx, const char* text, int* tokens, int max_tokens) {
    if (!ctx || !text || !tokens) {
        g_last_error = "Недопустимые параметры для токенизации";
        return -1;
    }

    auto wrapper = static_cast<llama_wrapper_context_t*>(ctx);

    try {
        std::vector<llama_token> token_vec = common_tokenize(wrapper->ctx, text, true, true);

        int count = std::min((int)token_vec.size(), max_tokens);
        for (int i = 0; i < count; i++) {
            tokens[i] = token_vec[i];
        }

        return count;
    } catch (const std::exception& e) {
        g_last_error = "Ошибка при токенизации: " + std::string(e.what());
        return -1;
    }
}

// Токенизация с динамическим выделением памяти (управление памятью на стороне C)
// Должен освободить возвращенный массив токенов через llama_wrapper_free_tokens
void llama_wrapper_tokenize_alloc(void* ctx, const char* text, int** tokens, int* count) {
    // Инициализируем выходные параметры безопасными значениями по умолчанию
    if (tokens) {
        *tokens = nullptr;
    }

    if (count) {
        *count = -1;
    }

    if (!ctx || !text || !tokens || !count) {
        g_last_error = "Недопустимые параметры для токенизации";
        return;
    }

    auto wrapper = static_cast<llama_wrapper_context_t*>(ctx);

    try {
        // Токенизируем текст (без усечения)
        std::vector<llama_token> token_vec = common_tokenize(wrapper->ctx, text, true, true);

        // Выделяем память точного размера
        int n_tokens = token_vec.size();
        int* allocated_tokens = (int*)malloc(n_tokens * sizeof(int));
        if (!allocated_tokens) {
            g_last_error = "Не удалось выделить память для токенов";
            return;
        }

        // Копируем токены из вектора в выделенный массив
        for (int i = 0; i < n_tokens; i++) {
            allocated_tokens[i] = token_vec[i];
        }

        // Возвращаем указатель и количество
        *tokens = allocated_tokens;
        *count = n_tokens;

    } catch (const std::exception& e) {
        g_last_error = "Ошибка при токенизации: " + std::string(e.what());
        if (tokens && *tokens) {
            free(*tokens);
            *tokens = nullptr;
        }
        if (count) *count = -1;
    }
}

// Освобождение токенов, выделенных llama_wrapper_tokenize_alloc
void llama_wrapper_free_tokens(int* tokens) {
    if (tokens) {
        free(tokens);
    }
}

int llama_wrapper_embeddings(void* ctx, const char* text, float* embeddings, int max_embeddings) {
    if (!ctx || !text || !embeddings) {
        g_last_error = "Недопустимые параметры для эмбеддингов";
        return -1;
    }

    auto wrapper = static_cast<llama_wrapper_context_t*>(ctx);

    try {
        // Очищаем KV-кэш для гарантии чистого состояния
        llama_memory_seq_rm(llama_get_memory(wrapper->ctx), 0, -1, -1);

        // Токенизируем текст
        std::vector<llama_token> tokens = common_tokenize(wrapper->ctx, text, true, true);

        if (tokens.empty()) {
            g_last_error = "Не удалось разбить текст на токены для эмбеддингов";
            return -1;
        }

        // Вычисляем токены чанками с учетом ограничения n_batch
        int n_batch = llama_n_batch(wrapper->ctx);
        int n_tokens = tokens.size();

        for (int i = 0; i < n_tokens; i += n_batch) {
            int chunk_size = std::min(n_batch, n_tokens - i);
            llama_batch batch = llama_batch_init(chunk_size, 0, 1);
            common_batch_clear(batch);

            // Добавляем токены этого чанка
            for (int j = 0; j < chunk_size; j++) {
                // Для эмбеддингов логиты нужны для всех токенов
                common_batch_add(batch, tokens[i + j], i + j, { 0 }, true);
            }

            if (llama_decode(wrapper->ctx, batch) != 0) {
                llama_batch_free(batch);
                g_last_error = "Не удалось декодировать токены для эмбеддингов";
                return -1;
            }

            llama_batch_free(batch);
        }

        // Пулинг MEAN/LAST/CLS кладёт вектор в embd_seq для LLAMA_POOLING_TYPE_NONE - только по-токенные строки (llama_get_embeddings_ith)
        // Раньше всегда вызывали *_seq - на обычных LLM без pooling в GGUF это всегда NULL
        const float* embd = llama_get_embeddings_seq(wrapper->ctx, 0);
        if (!embd) {
            embd = llama_get_embeddings_ith(wrapper->ctx, -1);
        }

        if (!embd) {
            g_last_error = "Не удалось получить векторные представления из контекста";
            return -1;
        }

        // Копируем эмбеддинги
        int n_embd = llama_model_n_embd(wrapper->model);
        int count = std::min(n_embd, max_embeddings);

        memcpy(embeddings, embd, count * sizeof(float));

        return count;
    } catch (const std::exception& e) {
        g_last_error = "Исключение при генерации эмбеддинга: " + std::string(e.what());
        return -1;
    }
}

int llama_wrapper_embeddings_batch(void* ctx, const char** texts, int n_texts, float* embeddings, int n_embd) {
    if (!ctx || !texts || !embeddings || n_texts <= 0 || n_embd <= 0) {
        g_last_error = "Неверные параметры для пакетной обработки эмбеддингов";
        return -1;
    }

    auto wrapper = static_cast<llama_wrapper_context_t*>(ctx);

    try {
        // При отсутствии пулинга seq-эмбеддинги недоступны; отдельный decode на текст совпадает
        // с однопроходной логикой и даёт корректный вектор последнего токена на строку.
        if (llama_pooling_type(wrapper->ctx) == LLAMA_POOLING_TYPE_NONE) {
            for (int i = 0; i < n_texts; i++) {
                if (llama_wrapper_embeddings(ctx, texts[i], embeddings + i * n_embd, n_embd) < 0) {
                    return -1;
                }
            }
            return n_texts;
        }

        // Очищаем KV-кэш для гарантии чистого состояния
        llama_memory_clear(llama_get_memory(wrapper->ctx), true);

        // Токенизируем все тексты
        std::vector<std::vector<llama_token>> all_tokens;
        all_tokens.reserve(n_texts);

        for (int i = 0; i < n_texts; i++) {
            if (!texts[i]) {
                g_last_error = "Пустой текст в пакете по индексу " + std::to_string(i);
                return -1;
            }

            std::vector<llama_token> tokens = common_tokenize(wrapper->ctx, texts[i], true, true);
            if (tokens.empty()) {
                g_last_error = "Не удалось разбить текст на токены по индексу " + std::to_string(i);
                return -1;
            }

            all_tokens.push_back(std::move(tokens));
        }

        // Получаем размер batch и максимум последовательностей
        int n_batch = llama_n_batch(wrapper->ctx);
        int n_seq_max = llama_n_seq_max(wrapper->ctx);

        // Инициализируем batch
        llama_batch batch = llama_batch_init(n_batch, 0, n_seq_max);

        // Отслеживаем, сколько эмбеддингов уже извлекли
        int embeddings_stored = 0;

        // Обрабатываем тексты батчами

        // Текущий ID последовательности в batch
        int s = 0;
        for (int k = 0; k < n_texts; k++) {
            const auto& tokens = all_tokens[k];
            int n_tokens = tokens.size();

            // Проверяем, не превысит ли добавление этого текста размер batch или лимит последовательностей
            if (batch.n_tokens + n_tokens > n_batch || s >= n_seq_max) {
                // Декодируем текущий batch
                if (llama_decode(wrapper->ctx, batch) != 0) {
                    llama_batch_free(batch);
                    g_last_error = "Не удалось расшифровать пакет";
                    return -1;
                }

                // Извлекаем эмбеддинги для всех последовательностей в этом batch
                for (int seq = 0; seq < s; seq++) {
                    const float* embd = llama_get_embeddings_seq(wrapper->ctx, seq);
                    if (!embd) {
                        llama_batch_free(batch);
                        g_last_error = "Не удалось получить эмбеддинги для последовательности " + std::to_string(seq);
                        return -1;
                    }

                    // Копируем эмбеддинг в выходной буфер
                    memcpy(embeddings + embeddings_stored * n_embd, embd, n_embd * sizeof(float));
                    embeddings_stored++;
                }

                // Очищаем KV-кэш для обработанных последовательностей перед сбросом
                for (int seq = 0; seq < s; seq++) {
                    llama_memory_seq_rm(llama_get_memory(wrapper->ctx), seq, -1, -1);
                }

                // Сбрасываем состояние для следующего batch
                s = 0;
                common_batch_clear(batch);
            }

            // Добавляем токены этого текста с уникальным seq_id
            for (int j = 0; j < n_tokens; j++) {
                // Позиция относительна этой последовательности (начинается с 0)
                // Для эмбеддингов логиты нужны для всех токенов
                common_batch_add(batch, tokens[j], j, { s }, true);
            }

            // Переходим к следующему ID последовательности
            s++;
        }

        // Обрабатываем финальный batch, если остались последовательности
        if (s > 0) {
            if (llama_decode(wrapper->ctx, batch) != 0) {
                llama_batch_free(batch);
                g_last_error = "Не удалось расшифровать последнюю партию";
                return -1;
            }

            // Извлекаем эмбеддинги для оставшихся последовательностей
            for (int seq = 0; seq < s; seq++) {
                const float* embd = llama_get_embeddings_seq(wrapper->ctx, seq);
                if (!embd) {
                    llama_batch_free(batch);
                    g_last_error = "Не удалось получить эмбеддинги для конечной последовательности " + std::to_string(seq);
                    return -1;
                }

                memcpy(embeddings + embeddings_stored * n_embd, embd, n_embd * sizeof(float));
                embeddings_stored++;
            }
        }

        llama_batch_free(batch);

        // Проверяем, что получили все эмбеддинги
        if (embeddings_stored != n_texts) {
            g_last_error = "Несоответствие количества встраиваний: ожидалось " + std::to_string(n_texts) +", получено " + std::to_string(embeddings_stored);
            return -1;
        }

        return embeddings_stored;

    } catch (const std::exception& e) {
        g_last_error = "Исключение при генерации пакетного встраивания: " + std::string(e.what());
        return -1;
    }
}

int llama_wrapper_get_cached_token_count(void* ctx) {
    if (!ctx) {
        g_last_error = "Контекст не может быть нулевым";
        return -1;
    }

    auto wrapper = static_cast<llama_wrapper_context_t*>(ctx);
    return static_cast<int>(wrapper->cached_tokens.size());
}

// Получить chat-шаблон из метаданных модели
// Возвращает nullptr, если шаблон недоступен
const char* llama_wrapper_get_chat_template(void* model) {
    if (!model) {
        return nullptr;
    }

    auto model_wrapper = static_cast<llama_wrapper_model_t*>(model);

    // Получаем chat-шаблон по умолчанию (name = nullptr)
    const char* tmpl = llama_model_chat_template(model_wrapper->model, nullptr);

    // Может быть nullptr, если у модели нет шаблона
    return tmpl;
}

// Применить chat-шаблон к сообщениям
// Возвращает выделенную строку с отформатированным промптом (освобождать через llama_wrapper_free_result)
// Возвращает nullptr при ошибке
char* llama_wrapper_apply_chat_template(const char* tmpl, const char** roles, const char** contents, int n_messages, bool add_assistant) {
    if (!tmpl || !roles || !contents || n_messages < 0) {
        g_last_error = "Неверные параметры для шаблона чата";
        return nullptr;
    }

    try {
        // Формируем массив структур llama_chat_message
        std::vector<llama_chat_message> messages;
        messages.reserve(n_messages);

        for (int i = 0; i < n_messages; i++) {
            if (!roles[i] || !contents[i]) {
                g_last_error = "Роль или контент не могут быть пустыми";
                return nullptr;
            }
            messages.push_back({roles[i], contents[i]});
        }

        // Начинаем с разумного размера буфера (8КБ)
        std::vector<char> buffer(8192);

        // Пытаемся применить шаблон
        int32_t result_len = llama_chat_apply_template(tmpl, messages.data(), n_messages, add_assistant, buffer.data(),buffer.size());

        // Если буфер оказался мал, увеличиваем и повторяем
        if (result_len > (int32_t)buffer.size()) {
            buffer.resize(result_len);
            result_len = llama_chat_apply_template(
                tmpl,
                messages.data(),
                n_messages,
                add_assistant,
                buffer.data(),
                buffer.size()
            );
        }

        // Проверяем ошибки
        if (result_len < 0) {
            g_last_error = "Не удалось применить шаблон чата";
            return nullptr;
        }

        // Выделяем память под результат и копируем
        char* c_result = (char*)malloc(result_len + 1);
        if (c_result) {
            memcpy(c_result, buffer.data(), result_len);
            c_result[result_len] = '\0';
        } else {
            g_last_error = "Не удалось выделить память для результата шаблона чата";
            return nullptr;
        }

        return c_result;
    } catch (const std::exception& e) {
        g_last_error = "Ошибка при применении шаблона чата: " + std::string(e.what());
        return nullptr;
    }
}

// Разобрать вывод модели для извлечения reasoning/thinking-контента
// Возвращает NULL при ошибке
// Освобождать результат через llama_wrapper_free_parsed_message()
llama_wrapper_parsed_message* llama_wrapper_parse_reasoning(
    const char* text,
    bool is_partial,
    llama_wrapper_reasoning_format format,
    int chat_format
) {
    if (!text) {
        g_last_error = "Текст не может быть пустым для целей логического анализа";
        return nullptr;
    }

    try {
        // Настраиваем синтаксис для парсинга
        common_chat_parser_params syntax;
        syntax.format = static_cast<common_chat_format>(chat_format);
        syntax.reasoning_format = static_cast<common_reasoning_format>(format);

        // Извлекаем в отдельное поле для стриминга
        syntax.reasoning_in_content = false;
        syntax.thinking_forced_open = false;

        // Парсинг вызовов инструментов в этом сценарии не нужен
        syntax.parse_tool_calls = false;

        // Парсим текст
        common_chat_msg msg = common_chat_parse(std::string(text), is_partial, syntax);

        // Выделяем структуру результата
        auto* result = new llama_wrapper_parsed_message;
        result->content = strdup(msg.content.c_str());
        result->reasoning_content = msg.reasoning_content.empty()
            ? nullptr
            : strdup(msg.reasoning_content.c_str());

        return result;
    } catch (const std::exception& e) {
        g_last_error = "Исключение в процессе анализа логических выводов: " + std::string(e.what());
        return nullptr;
    }
}

void llama_wrapper_free_parsed_message(llama_wrapper_parsed_message* msg) {
    if (!msg) {
        return;
    }

    if (msg->content) {
        free(const_cast<char*>(msg->content));
    }

    if (msg->reasoning_content) {
        free(const_cast<char*>(msg->reasoning_content));
    }

    delete msg;
}

void* llama_wrapper_chat_templates_init(void* model, const char* template_override) {
    if (!model) return nullptr;

    auto model_wrapper = static_cast<llama_wrapper_model_t*>(model);
    std::string tmpl_override = template_override ? template_override : "";

    auto templates = common_chat_templates_init(model_wrapper->model, tmpl_override);

    // Передаем владение
    return templates.release();
}

void llama_wrapper_chat_templates_free(void* templates) {
    if (!templates) {
        return;
    }

    common_chat_templates_free(static_cast<common_chat_templates*>(templates));
}

int llama_wrapper_chat_templates_get_format(void* templates) {
    if (!templates) {
        // значение константы COMMON_CHAT_FORMAT_CONTENT_ONLY
        return 0;
    }

    auto tmpl = static_cast<common_chat_templates*>(templates);

    try {
        // Применяем с минимальными тестовыми сообщениями, чтобы запустить определение формата
        common_chat_templates_inputs inputs;
        inputs.use_jinja = true;
        inputs.add_generation_prompt = true;

        // Создаем минимальное тестовое сообщение для корректного применения шаблона
        common_chat_msg dummy_msg;
        dummy_msg.role = "user";
        // Непустое значение, чтобы избежать потенциальных проблем
        dummy_msg.content = "test";
        inputs.messages.push_back(dummy_msg);

        auto params = common_chat_templates_apply(tmpl, inputs);
        return static_cast<int>(params.format);
    } catch (const std::exception& e) {
        // Если применение шаблона не удалось, возвращаем CONTENT_ONLY как запасной вариант
        g_last_error = "Определение формата не удалось: " + std::string(e.what());

        // значение константы COMMON_CHAT_FORMAT_CONTENT_ONLY
        return 0;
    }
}

// Получить строковое значение метаданных модели по ключу
const char* llama_wrapper_model_meta_string(void* model, const char* key) {
    if (!model || !key) return nullptr;

    auto model_wrapper = static_cast<llama_wrapper_model_t*>(model);

    // Используем метаданных llama.cpp с буфером
    // Статический буфер для строк метаданных
    static char buffer[2048];
    int32_t result = llama_model_meta_val_str(model_wrapper->model, key, buffer, sizeof(buffer));

    if (result < 0) {
        return nullptr;
    }

    return buffer;
}

// Получить количество пар ключ-значение в метаданных
int llama_wrapper_model_meta_count(void* model) {
    if (!model) {
        return 0;
    }

    auto model_wrapper = static_cast<llama_wrapper_model_t*>(model);
    return llama_model_meta_count(model_wrapper->model);
}

// Получить количество CUDA-устройств
int llama_wrapper_get_gpu_count() {
#ifdef GGML_USE_CUDA
    return ggml_backend_cuda_get_device_count();
#else
    return 0;
#endif
}

// Получить информацию об устройстве gpu
bool llama_wrapper_get_gpu_info(int device_id, llama_wrapper_gpu_info* info) {
    if (!info) {
        return false;
    }

#ifdef GGML_USE_CUDA
    int count = ggml_backend_cuda_get_device_count();
    if (device_id < 0 || device_id >= count) {
        return false;
    }

    // Получаем описание устройства
    ggml_backend_cuda_get_device_description(device_id, info->device_name, sizeof(info->device_name));
    info->device_id = device_id;

    // Получаем информацию о памяти
    size_t free_mem, total_mem;
    ggml_backend_cuda_get_device_memory(device_id, &free_mem, &total_mem);
    info->free_memory_mb = free_mem / (1024 * 1024);
    info->total_memory_mb = total_mem / (1024 * 1024);

    return true;
#else
    return false;
#endif
}

// Получить runtime-информацию о модели и контексте
void llama_wrapper_get_runtime_info(void* model, void* ctx, const char* kv_cache_type, llama_wrapper_runtime_info* info) {
    if (!model || !info) {
        return;
    }

    auto model_wrapper = static_cast<llama_wrapper_model_t*>(model);

    // Получаем число слоев (в llama.cpp используется "layer")
    info->total_layers = llama_model_n_layer(model_wrapper->model);

    // Загруженные gpu-слои - минимум из запрошенного и общего числа слоев
    // (Короче невозможно загрузить больше слоев, чем есть у модели)
    info->gpu_layers = std::min(model_wrapper->n_gpu_layers, info->total_layers);

    if (ctx) {
        auto ctx_wrapper = static_cast<llama_wrapper_context_t*>(ctx);
        info->n_ctx = llama_n_ctx(ctx_wrapper->ctx);
        info->n_batch = llama_n_batch(ctx_wrapper->ctx);

        // Корректно вычисляем размер KV-кэша с учетом GQA/MQA
        // Формула - 2 * n_ctx * (head_dim * n_head_kv) * n_layers * bytes_per_element
        int n_embd = llama_model_n_embd(model_wrapper->model);
        int n_head = llama_model_n_head(model_wrapper->model);
        int n_head_kv = llama_model_n_head_kv(model_wrapper->model);
        int head_dim = n_embd / n_head;

        // Определяем размер элемента по типу квантизации
        // По умолчанию f16
        float bytes_per_element = 2.0f;

        if (kv_cache_type) {
            std::string cache_type(kv_cache_type);
            if (cache_type == "f16") {
                bytes_per_element = 2.0f;
            } else if (cache_type == "q8_0") {
                // ~1 байт + накладные расходы
                bytes_per_element = 1.125f;
            } else if (cache_type == "q4_0") {
                // ~0.5 байта + накладные расходы
                bytes_per_element = 0.625f;
            }
        }

        // Кэш K и V
        // n_ctx * head_dim * n_head_kv * 2 (K+V) * n_layers * element_size
        long long cache_bytes = (long long)info->n_ctx * head_dim * n_head_kv * 2LL * info->total_layers * bytes_per_element;
        info->kv_cache_size_mb = cache_bytes / (1024 * 1024);
    } else {
        // Нет контекста - используем значения по умолчанию или нули
        info->n_ctx = 0;
        info->n_batch = 0;
        info->kv_cache_size_mb = 0;
    }
}

static void fill_sampling_params_from_generate(llama_wrapper_generate_params gen_params, common_params_sampling& sampling_params) {
    sampling_params.seed = gen_params.seed;
    sampling_params.temp = gen_params.temperature;
    sampling_params.top_k = gen_params.top_k;
    sampling_params.top_p = gen_params.top_p;
    sampling_params.min_p = gen_params.min_p;
    sampling_params.typ_p = gen_params.typ_p;
    sampling_params.top_n_sigma = gen_params.top_n_sigma;
    sampling_params.min_keep = gen_params.min_keep;
    sampling_params.penalty_last_n = gen_params.penalty_last_n;
    sampling_params.penalty_repeat = gen_params.penalty_repeat;
    sampling_params.penalty_freq = gen_params.penalty_freq;
    sampling_params.penalty_present = gen_params.penalty_present;
    sampling_params.dry_multiplier = gen_params.dry_multiplier;
    sampling_params.dry_base = gen_params.dry_base;
    sampling_params.dry_allowed_length = gen_params.dry_allowed_length;
    sampling_params.dry_penalty_last_n = gen_params.dry_penalty_last_n;
    sampling_params.dry_sequence_breakers.clear();
    for (int i = 0; i < gen_params.dry_sequence_breakers_count; i++) {
        if (gen_params.dry_sequence_breakers && gen_params.dry_sequence_breakers[i]) {
            sampling_params.dry_sequence_breakers.push_back(std::string(gen_params.dry_sequence_breakers[i]));
        }
    }
    sampling_params.dynatemp_range = gen_params.dynatemp_range;
    sampling_params.dynatemp_exponent = gen_params.dynatemp_exponent;
    sampling_params.xtc_probability = gen_params.xtc_probability;
    sampling_params.xtc_threshold = gen_params.xtc_threshold;
    sampling_params.mirostat = gen_params.mirostat;
    sampling_params.mirostat_tau = gen_params.mirostat_tau;
    sampling_params.mirostat_eta = gen_params.mirostat_eta;
    sampling_params.n_prev = gen_params.n_prev;
    sampling_params.n_probs = gen_params.n_probs;
    sampling_params.ignore_eos = gen_params.ignore_eos;
}

int llama_wrapper_mtmd_chat_prompt(
    void* ctx_raw,
    void* model_raw,
    const char* chat_template_override,
    int use_jinja_int,
    const char** roles,
    const char** contents,
    const unsigned char** image_bytes,
    const size_t* image_lens,
    const int* has_image,
    int n_messages,
    llama_wrapper_generate_params gen_params) {
    if (!ctx_raw || !model_raw || !roles || !contents || !has_image || n_messages <= 0) {
        g_last_error = "mtmd: некорректные аргументы";
        return -1;
    }

    auto* mw = static_cast<llama_wrapper_model_t*>(model_raw);
    if (!mw->mtmd) {
        g_last_error = "mtmd: проектор не загружен (укажите mmproj при загрузке модели)";
        return -2;
    }

    auto* cw = static_cast<llama_wrapper_context_t*>(ctx_raw);
    if (!cw->ctx || !mw->model) {
        g_last_error = "mtmd: контекст или модель недоступны";
        return -3;
    }

    llama_context* lctx = cw->ctx;
    llama_model* lmodel = mw->model;
    const bool use_jinja = use_jinja_int != 0;

    std::string tmpl_override = chat_template_override ? std::string(chat_template_override) : std::string();
    common_chat_templates_ptr tmpls = common_chat_templates_init(lmodel, tmpl_override, "", "");
    if (!tmpls) {
        g_last_error = "mtmd: не удалось инициализировать chat templates";
        return -4;
    }

    std::vector<common_chat_msg> history;
    llama_pos n_past = 0;
    const int n_batch = llama_n_batch(lctx);

    for (int i = 0; i < n_messages; i++) {
        common_chat_msg new_msg;
        new_msg.role = roles[i] ? roles[i] : "";
        new_msg.content = contents[i] ? contents[i] : "";

        mtmd_bitmap* bmp = nullptr;
        std::vector<const mtmd_bitmap*> bmp_ptrs;
        if (has_image[i]) {
            if (!image_bytes || !image_lens || !image_bytes[i] || image_lens[i] == 0) {
                g_last_error = "mtmd: пустое изображение в сообщении";
                return -5;
            }

            bmp = mtmd_helper_bitmap_init_from_buf(mw->mtmd, image_bytes[i], image_lens[i]);
            if (!bmp) {
                g_last_error = "mtmd: не удалось декодировать изображение";
                return -6;
            }

            const std::string marker = mtmd_default_marker();
            if (new_msg.content.find(marker) == std::string::npos) {
                new_msg.content = marker + new_msg.content;
            }

            bmp_ptrs.push_back(bmp);
        }

        const std::string formatted = common_chat_format_single(tmpls.get(), history, new_msg, new_msg.role == "user", use_jinja);

        mtmd_input_text text{};
        text.text = formatted.c_str();
        text.add_special = history.empty();
        text.parse_special = true;

        mtmd_input_chunks* chunks = mtmd_input_chunks_init();
        if (!chunks) {
            if (bmp) {
                mtmd_bitmap_free(bmp);
            }
            g_last_error = "mtmd: mtmd_input_chunks_init failed";
            return -7;
        }

        const int32_t tr = mtmd_tokenize(mw->mtmd, chunks, &text, bmp_ptrs.empty() ? nullptr : bmp_ptrs.data(), bmp_ptrs.size());
        if (tr != 0) {
            if (bmp) {
                mtmd_bitmap_free(bmp);
            }
            mtmd_input_chunks_free(chunks);
            g_last_error = "mtmd: mtmd_tokenize ошибка " + std::to_string(tr);
            return -8;
        }
        if (bmp) {
            mtmd_bitmap_free(bmp);
            bmp = nullptr;
        }

        llama_pos new_n_past = n_past;
        if (mtmd_helper_eval_chunks(mw->mtmd, lctx, chunks, n_past, 0, n_batch, true, &new_n_past) != 0) {
            mtmd_input_chunks_free(chunks);
            g_last_error = "mtmd: mtmd_helper_eval_chunks не удался";
            return -9;
        }
        mtmd_input_chunks_free(chunks);
        n_past = new_n_past;
        history.push_back(std::move(new_msg));
    }

    common_params_sampling sampling_params{};
    fill_sampling_params_from_generate(gen_params, sampling_params);
    common_sampler* smpl = common_sampler_init(lmodel, sampling_params);
    if (!smpl) {
        g_last_error = "mtmd: не удалось создать sampler";
        return -10;
    }

    const llama_vocab* vocab = llama_model_get_vocab(lmodel);
    const int available_ctx = llama_n_ctx(lctx);
    int gen_room = available_ctx - static_cast<int>(n_past);
    if (gen_room < 1) {
        common_sampler_free(smpl);
        g_last_error = "mtmd: нет места в контексте для генерации";
        return -11;
    }

    int n_predict = gen_params.max_tokens > 0 ? std::min(gen_params.max_tokens, gen_room) : gen_room;
    if (n_predict < 1) {
        common_sampler_free(smpl);
        g_last_error = "mtmd: max_tokens некорректен";
        return -12;
    }

    const int batch_cap = std::max(512, n_batch);
    llama_batch gen_batch = llama_batch_init(batch_cap, 0, 1);

    std::string accumulated;
    for (int step = 0; step < n_predict; step++) {
        const llama_token tid = common_sampler_sample(smpl, lctx, -1);
        if (llama_vocab_is_eog(vocab, tid)) {
            break;
        }

        common_sampler_accept(smpl, tid, true);
        const std::string piece = common_token_to_piece(lctx, tid);
        accumulated += piece;

        if (gen_params.stop_words_count > 0 && gen_params.stop_words) {
            for (int si = 0; si < gen_params.stop_words_count; si++) {
                if (gen_params.stop_words[si] && accumulated.find(gen_params.stop_words[si]) != std::string::npos) {
                    goto mtmd_gen_done;
                }
            }
        }

        if (gen_params.callback_handle != 0) {
            if (!goTokenCallback(gen_params.callback_handle, piece.c_str())) {
                break;
            }
        }

        common_batch_clear(gen_batch);
        common_batch_add(gen_batch, tid, n_past, {0}, true);
        n_past++;
        if (llama_decode(lctx, gen_batch) != 0) {
            break;
        }
    }
mtmd_gen_done:
    llama_batch_free(gen_batch);
    common_sampler_free(smpl);
    return 0;
}
}
