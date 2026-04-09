package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"log"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/magomedcoder/gen/internal/domain"
)

type ServerConfig struct {
	Port string `yaml:"port"`
	Host string `yaml:"host"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
}

type JWTConfig struct {
	AccessSecret  string        `yaml:"access_secret"`
	RefreshSecret string        `yaml:"refresh_secret"`
	AccessTTL     time.Duration `yaml:"access_ttl"`
	RefreshTTL    time.Duration `yaml:"refresh_ttl"`
}

type MCPConfig struct {
	HTTPAllowAny              bool     `yaml:"http_allow_any"`
	HTTPAllowHosts            []string `yaml:"http_allow_hosts"`
	Roots                     []string `yaml:"roots"`
	SamplingEnabled           bool     `yaml:"sampling_enabled"`
	LogServerMessages         bool     `yaml:"log_server_messages"`
	HTTPReuseSessions         bool     `yaml:"http_reuse_sessions"`
	HTTPSessionMaxIdleSeconds int      `yaml:"http_session_max_idle_seconds"`
}

type RAGConfig struct {
	ChunkSizeRunes                int `yaml:"chunk_size_runes"`
	ChunkOverlapRunes             int `yaml:"chunk_overlap_runes"`
	EmbedBatchSize                int `yaml:"embed_batch_size"`
	MaxChunkEmbedRunes            int `yaml:"max_chunk_embed_runes"`
	BackgroundIndexTimeoutSeconds int `yaml:"background_index_timeout_seconds"`
	LLMContextFallbackTokens      int `yaml:"llm_context_fallback_tokens"`
	MaxExtractedRunesOnUpload     int `yaml:"max_extracted_runes_on_upload"`
}

func (r RAGConfig) EffectiveChunkSizeRunes() int {
	if r.ChunkSizeRunes <= 0 {
		return 1024
	}
	return r.ChunkSizeRunes
}

func (r RAGConfig) EffectiveChunkOverlapRunes() int {
	if r.ChunkOverlapRunes < 0 {
		return 0
	}
	return r.ChunkOverlapRunes
}

func (r RAGConfig) EffectiveEmbedBatchSize() int {
	if r.EmbedBatchSize <= 0 {
		return 32
	}
	return r.EmbedBatchSize
}

func (r RAGConfig) EffectiveMaxChunkEmbedRunes() int {
	if r.MaxChunkEmbedRunes <= 0 {
		return 8192
	}
	return r.MaxChunkEmbedRunes
}

func (r RAGConfig) BackgroundIndexTimeout() time.Duration {
	if r.BackgroundIndexTimeoutSeconds <= 0 {
		return 30 * time.Minute
	}

	return time.Duration(r.BackgroundIndexTimeoutSeconds) * time.Second
}

const ragBuiltinLLMContextFallbackTokens = 4096

func (r RAGConfig) EffectiveLLMContextFallbackTokens() int {
	if r.LLMContextFallbackTokens < 0 {
		return 0
	}

	if r.LLMContextFallbackTokens == 0 {
		return ragBuiltinLLMContextFallbackTokens
	}

	return r.LLMContextFallbackTokens
}

func (r RAGConfig) EffectiveMaxExtractedRunesOnUpload() int {
	if r.MaxExtractedRunesOnUpload <= 0 {
		return 0
	}
	return r.MaxExtractedRunesOnUpload
}

type Config struct {
	Server                       ServerConfig
	Database                     DatabaseConfig
	JWT                          JWTConfig
	MCP                          MCPConfig
	RAG                          RAGConfig `yaml:"rag"`
	DataDir                      string    `yaml:"data_dir"`
	AttachmentHydrateParallelism int       `yaml:"attachment_hydrate_parallelism"`
	LogLevel                     string    `yaml:"log_level"`
	MinClientBuild               int32
}

func LoadFrom(path string) (*Config, error) {
	var conf Config
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("путь к файлу конфигурации пустой")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}

	if err := yaml.Unmarshal(content, &conf); err != nil {
		log.Println(err)
		panic(fmt.Sprintf("Ошибка при разборе: %v", err))
	}

	return &conf, nil
}

func (c *Config) MCPHTTPHostAllowed(host string) bool {
	if c == nil {
		return false
	}

	if c.MCP.HTTPAllowAny {
		return true
	}

	h := strings.TrimSpace(host)
	if h == "" {
		return false
	}

	if ip := net.ParseIP(h); ip != nil {
		if ip.IsLoopback() {
			return true
		}

		for _, e := range c.MCP.HTTPAllowHosts {
			e = strings.TrimSpace(e)
			if e == "" {
				continue
			}

			if ip2 := net.ParseIP(e); ip2 != nil && ip.Equal(ip2) {
				return true
			}
		}

		return false
	}

	h = strings.ToLower(h)
	if h == "localhost" {
		return true
	}

	for _, s := range c.MCP.HTTPAllowHosts {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" || net.ParseIP(s) != nil {
			continue
		}

		if h == s || strings.HasSuffix(h, "."+s) {
			return true
		}
	}

	return false
}

func (c *Config) ValidateMCPServerHTTP(s *domain.MCPServer) error {
	if c == nil || s == nil {
		return nil
	}

	tr := strings.ToLower(strings.TrimSpace(s.Transport))
	if tr != "sse" && tr != "streamable" {
		return nil
	}

	raw := strings.TrimSpace(s.URL)
	if raw == "" {
		return fmt.Errorf("для транспорта %s нужен непустой url", tr)
	}

	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("url: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url: ожидается http или https")
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("url: пустой хост")
	}

	if !c.MCPHTTPHostAllowed(host) {
		return fmt.Errorf("хост %q не разрешён политикой GEN-MCP ", host)
	}

	return nil
}
