package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Host                           string   `yaml:"host"`
	Port                           int      `yaml:"port"`
	RegistrationToken              string   `yaml:"registration_token"`
	LogLevel                       string   `yaml:"log_level"`
	ModelPath                      string   `yaml:"model_path"`
	MaxContextTokens               int      `yaml:"max_context_tokens"`
	LLMHistoryMaxMessages          int      `yaml:"llm_history_max_messages"`
	LLMHistorySummarizeDropped     bool     `yaml:"llm_history_summarize_dropped"`
	LLMHistorySummaryMaxInputRunes int      `yaml:"llm_history_summary_max_input_runes"`
	LLMHistorySummaryModel         string   `yaml:"llm_history_summary_model"`
	LLMHistorySummaryRunnerListen  string   `yaml:"llm_history_summary_runner_listen"`
	LLMHistorySummaryCacheEntries  int      `yaml:"llm_history_summary_cache_entries"`
	MaxToolInvocationRounds        int      `yaml:"max_tool_invocation_rounds"`
	MaxConcurrentGenerations       int      `yaml:"max_concurrent_generations"`
	ModelRetention                 string   `yaml:"model_retention"`
	GpuLayers                      int      `yaml:"gpu_layers"`
	MLock                          bool     `yaml:"mlock"`
	MMap                           *bool    `yaml:"mmap"`
	MainGPU                        string   `yaml:"main_gpu"`
	TensorSplit                    string   `yaml:"tensor_split"`
	SilentLoading                  bool     `yaml:"silent_loading"`
	ProgressCallback               bool     `yaml:"progress_callback"`
	Threads                        int      `yaml:"threads"`
	ThreadsBatch                   int      `yaml:"threads_batch"`
	BatchSize                      int      `yaml:"batch_size"`
	F16Memory                      bool     `yaml:"f16_memory"`
	KVCacheType                    string   `yaml:"kv_cache_type"`
	FlashAttn                      string   `yaml:"flash_attn"`
	PrefixCaching                  *bool    `yaml:"prefix_caching"`
	Parallel                       int      `yaml:"parallel"`
	TopNSigma                      *float32 `yaml:"top_n_sigma"`
	FrequencyPenalty               *float32 `yaml:"frequency_penalty"`
	PresencePenalty                *float32 `yaml:"presence_penalty"`
	IgnoreEOS                      *bool    `yaml:"ignore_eos"`
	DRYMultiplier                  *float32 `yaml:"dry_multiplier"`
	DRYBase                        *float32 `yaml:"dry_base"`
	DRYAllowedLength               *int     `yaml:"dry_allowed_length"`
	DRYPenaltyLastN                *int     `yaml:"dry_penalty_last_n"`
	DRYSequenceBreakers            []string `yaml:"dry_sequence_breakers"`
	XTCProbability                 *float32 `yaml:"xtc_probability"`
	XTCThreshold                   *float32 `yaml:"xtc_threshold"`
	Mirostat                       *int     `yaml:"mirostat"`
	MirostatTau                    *float32 `yaml:"mirostat_tau"`
	MirostatEta                    *float32 `yaml:"mirostat_eta"`
	TypicalP                       *float32 `yaml:"typical_p"`
	MinKeep                        *int     `yaml:"min_keep"`
	DynamicTemperatureRange        *float32 `yaml:"dynamic_temperature_range"`
	DynamicTemperatureExponent     *float32 `yaml:"dynamic_temperature_exponent"`
	NPrev                          *int     `yaml:"n_prev"`
	NProbs                         *int     `yaml:"n_probs"`
	DebugGeneration                bool     `yaml:"debug_generation"`
	SpeculativeEnabled             bool     `yaml:"speculative_enabled"`
	SpeculativeDraftModel          string   `yaml:"speculative_draft_model"`
	SpeculativeDraftTokens         int      `yaml:"speculative_draft_tokens"`
	TokenPipelineEnabled           bool     `yaml:"token_pipeline_enabled"`
	ChatAPIEnabled                 bool     `yaml:"chat_api_enabled"`
	ChatStreamBufferSize           int      `yaml:"chat_stream_buffer_size"`
	ChatReasoningFormat            string   `yaml:"chat_reasoning_format"`
	ChatEnableThinking             *bool    `yaml:"chat_enable_thinking"`
	ChatReasoningBudget            *int     `yaml:"chat_reasoning_budget"`
	ReinitLlamaLogging             bool     `yaml:"reinit_llama_logging"`
	LogModelStats                  bool     `yaml:"log_model_stats"`
}

func Load(path string) (*Config, error) {
	c := &Config{}

	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("путь к файлу конфигурации пустой")
	}

	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("ошибка чтения конфигурационного файла %s: %w", path, err)
		}

		if err := yaml.Unmarshal(data, c); err != nil {
			return nil, fmt.Errorf("ошибка парсинга конфигурационного файла %s: %w", path, err)
		}
	}

	c.applyDefaults()

	if err := c.validate(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Config) applyDefaults() {
	if c.MaxContextTokens <= 0 {
		c.MaxContextTokens = 4096
	}
}

func (c *Config) ListenAddr() string {
	h := strings.TrimSpace(c.Host)
	if h == "" || c.Port <= 0 {
		return ""
	}
	return net.JoinHostPort(h, strconv.Itoa(c.Port))
}

func (c *Config) validate() error {
	if strings.TrimSpace(c.Host) == "" {
		return fmt.Errorf("listen.host: укажите хост для прослушивания")
	}

	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("listen.port: укажите порт от 1 до 65535")
	}

	if c.KVCacheType != "" {
		switch strings.ToLower(strings.TrimSpace(c.KVCacheType)) {
		case "f16", "q8_0", "q4_0":
		default:
			return fmt.Errorf("kv_cache_type: допустимы f16, q8_0, q4_0")
		}
	}

	if c.FlashAttn != "" {
		switch strings.ToLower(strings.TrimSpace(c.FlashAttn)) {
		case "auto", "enabled", "disabled":
		default:
			return fmt.Errorf("flash_attn: допустимы auto, enabled, disabled")
		}
	}
	if c.ChatReasoningFormat != "" {
		switch strings.ToLower(strings.TrimSpace(c.ChatReasoningFormat)) {
		case "none", "auto", "deepseek-legacy", "deepseek":
		default:
			return fmt.Errorf("chat_reasoning_format: допустимы none, auto, deepseek-legacy, deepseek")
		}
	}

	r := strings.TrimSpace(c.ModelRetention)
	if r == "" {
		return nil
	}

	rl := strings.ToLower(r)
	if rl == "keep" || rl == "unload_after_rpc" {
		return nil
	}

	return fmt.Errorf("model_retention: неизвестное значение %q (допустимы keep, unload_after_rpc)", r)
}

func (c *Config) UnloadAfterRPC() bool {
	return strings.ToLower(strings.TrimSpace(c.ModelRetention)) == "unload_after_rpc"
}

func (c *Config) ModelsDir() string {
	p := strings.TrimSpace(c.ModelPath)
	if p == "" {
		return ""
	}

	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return filepath.Dir(p)
	}

	if strings.EqualFold(filepath.Ext(p), ".gguf") {
		return filepath.Dir(p)
	}

	return p
}

func (c *Config) RequireAbsModelsDir() (string, error) {
	d := strings.TrimSpace(c.ModelsDir())
	if d == "" {
		return "", fmt.Errorf("model_path: укажите каталог или файл модели в config.yaml")
	}

	abs, err := filepath.Abs(d)
	if err != nil {
		return "", fmt.Errorf("model_path: %w", err)
	}

	return abs, nil
}
