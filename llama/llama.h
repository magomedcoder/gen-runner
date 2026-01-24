#ifdef __cplusplus
extern "C" {
#endif

#include <stdbool.h>

void *load_model(
    const char *fname,
    int n_ctx,
    int seed,
    bool memory_f16,
    bool mlock,
    bool embeddings,
    bool mmap,
    bool low_vram,
    int n_gpu,
    int n_batch,
    const char *maingpu,
    const char *tensorsplit,
    bool numa,
    float rope_freq_base,
    float rope_freq_scale,
    const char *lora,
    const char *lora_base
);

void llama_binding_free_model(void *state);

int get_model_n_vocab(void *state_ptr);

int get_model_n_ctx_train(void *state_ptr);

int get_model_n_embd(void *state_ptr);

int get_model_n_layer(void *state_ptr);

long long get_model_size(void *state_ptr);

long long get_model_n_params(void *state_ptr);

int get_model_description(void *state_ptr, char *buf, int buf_size);

#ifdef __cplusplus
}
#endif
