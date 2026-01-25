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

void *llama_allocate_params(
    const char *prompt,
    int seed,
    int threads,
    int tokens,
    int top_k,
    float top_p,
    float min_p,
    float temp,
    float repeat_penalty,
    int repeat_last_n,
    bool ignore_eos,
    bool memory_f16,
    int n_batch,
    int n_keep,
    const char **antiprompt,
    int antiprompt_count,
    float tfs_z,
    float typical_p,
    float frequency_penalty,
    float presence_penalty,
    int mirostat,
    float mirostat_eta,
    float mirostat_tau,
    bool penalize_nl,
    const char *logit_bias,
    const char *session_file,
    bool prompt_cache_all,
    bool mlock,
    bool mmap,
    const char *maingpu,
    const char *tensorsplit,
    bool prompt_cache_ro,
    const char *grammar,
    float rope_freq_base,
    float rope_freq_scale,
    int n_draft,
    float xtc_probability,
    float xtc_threshold,
    float dry_multiplier,
    float dry_base,
    int dry_allowed_length,
    int dry_penalty_last_n,
    float top_n_sigma
);

void llama_free_params(void *params_ptr);

int llama_predict(void *params_ptr, void *state_pr, char *result, bool debug);

#ifdef __cplusplus
}
#endif
