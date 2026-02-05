package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Llama struct {
	ModelPath        string `yaml:"model_path"`
	MaxContextTokens int    `yaml:"max_context_tokens"`
}

type LogConfig struct {
	Level string `yaml:"level"`
}

type Config struct {
	CoreAddr                 string    `yaml:"core_addr"`
	ListenAddr               string    `yaml:"listen_addr"`
	RegistrationToken        string    `yaml:"registration_token"`
	Log                      LogConfig `yaml:"log"`
	Llama                    Llama     `yaml:"llama"`
	MaxConcurrentGenerations int       `yaml:"max_concurrent_generations"`
}

func Load() (*Config, error) {
	c := &Config{}

	configPath := os.Getenv("LLM_RUNNER_CONFIG")
	if configPath == "" {
		configPath = "./config.yaml"
	}

	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("ошибка чтения конфигурационного файла %s: %w", configPath, err)
		}

		if err := yaml.Unmarshal(data, c); err != nil {
			return nil, fmt.Errorf("ошибка парсинга конфигурационного файла %s: %w", configPath, err)
		}
	}

	return c, nil
}
