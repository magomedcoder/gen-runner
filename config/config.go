package config

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var LoadedFrom string

var loadedMu sync.Mutex

type Config struct {
	Server                       ServerConfig
	Database                     DatabaseConfig
	JWT                          JWTConfig
	Runners                      RunnersConfig
	UploadDir                    string `yaml:"upload_dir"`
	AttachmentHydrateParallelism int    `yaml:"attachment_hydrate_parallelism"`
	LogLevel                     string `yaml:"log_level"`
	MinClientBuild               int32
}

type RunnerEntry struct {
	Name              string `yaml:"name"`
	Address           string `yaml:"address"`
	Token             string `yaml:"token"`
	RegistrationToken string `yaml:"registration_token"`
}

func (e RunnerEntry) EffectiveToken() string {
	if t := strings.TrimSpace(e.Token); t != "" {
		return t
	}
	return strings.TrimSpace(e.RegistrationToken)
}

type RunnersConfig struct {
	Entries []RunnerEntry
}

func (c *Config) DefaultRunnerAddress() string {
	if c == nil {
		return ""
	}

	for _, e := range c.Runners.Entries {
		if a := strings.TrimSpace(e.Address); a != "" {
			return a
		}
	}

	return ""
}

type runnerEntryYAML struct {
	Name              string `yaml:"name"`
	Address           string `yaml:"address"`
	Token             string `yaml:"token"`
	RegistrationToken string `yaml:"registration_token"`
}

type runnersBlockYAML struct {
	Entries []runnerEntryYAML
}

func (b *runnersBlockYAML) UnmarshalYAML(n *yaml.Node) error {
	b.Entries = nil
	if n.Kind == 0 || n.IsZero() {
		return nil
	}
	switch n.Kind {
	case yaml.SequenceNode:
		var seq []runnerEntryYAML
		if err := n.Decode(&seq); err != nil {
			return fmt.Errorf("runners: %w", err)
		}
		b.Entries = seq
		return nil
	case yaml.MappingNode:
		out := make([]runnerEntryYAML, 0, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			var key string
			if err := n.Content[i].Decode(&key); err != nil {
				return fmt.Errorf("runners: ключ: %w", err)
			}
			key = strings.TrimSpace(key)
			valNode := n.Content[i+1]
			if key == "model" {
				continue
			}
			if valNode.Kind != yaml.ScalarNode {
				return fmt.Errorf("runners: для %q значение должно быть строкой-токеном", key)
			}
			var tok string
			if err := valNode.Decode(&tok); err != nil {
				return err
			}
			tok = strings.TrimSpace(tok)
			if key == "" || tok == "" {
				continue
			}
			out = append(out, runnerEntryYAML{Address: key, Token: tok})
		}
		b.Entries = out
		return nil
	default:
		return fmt.Errorf("runners: список записей или карта \"host:port\": token")
	}
}

type ServerConfig struct {
	Port string `yaml:"port"`
	Host string `yaml:"host"`
}

type DatabaseConfig struct {
	Host              string `yaml:"host"`
	Port              string `yaml:"port"`
	User              string `yaml:"user"`
	Password          string `yaml:"password"`
	Name              string `yaml:"name"`
	SSLMode           string `yaml:"ssl_mode"`
	MaxOpenConns      int    `yaml:"max_open_conns"`
	MaxIdleConns      int    `yaml:"max_idle_conns"`
	ConnMaxLifetime   string `yaml:"conn_max_lifetime"`
	ConnMaxIdleTime   string `yaml:"conn_max_idle_time"`
	ConnectTimeoutSec int    `yaml:"connect_timeout_sec"`
}

type JWTConfig struct {
	AccessSecret  string        `yaml:"access_secret"`
	RefreshSecret string        `yaml:"refresh_secret"`
	AccessTTL     time.Duration `yaml:"-"`
	RefreshTTL    time.Duration `yaml:"-"`
}

type databaseYAML struct {
	Host              string  `yaml:"host"`
	Port              string  `yaml:"port"`
	User              string  `yaml:"user"`
	Password          *string `yaml:"password"`
	Name              string  `yaml:"name"`
	SSLMode           string  `yaml:"ssl_mode"`
	MaxOpenConns      int     `yaml:"max_open_conns"`
	MaxIdleConns      int     `yaml:"max_idle_conns"`
	ConnMaxLifetime   string  `yaml:"conn_max_lifetime"`
	ConnMaxIdleTime   string  `yaml:"conn_max_idle_time"`
	ConnectTimeoutSec int     `yaml:"connect_timeout_sec"`
}

type yamlRoot struct {
	Server                       ServerConfig      `yaml:"server"`
	Database                     databaseYAML      `yaml:"database"`
	JWT                          jwtYAML           `yaml:"jwt"`
	Runners                      *runnersBlockYAML `yaml:"runners"`
	UploadDir                    string            `yaml:"upload_dir"`
	AttachmentHydrateParallelism int               `yaml:"attachment_hydrate_parallelism"`
	LogLevel                     string            `yaml:"log_level"`
	MinClientBuild               int32
}

type jwtYAML struct {
	AccessSecret  string `yaml:"access_secret"`
	RefreshSecret string `yaml:"refresh_secret"`
	AccessTTL     string `yaml:"access_ttl"`
	RefreshTTL    string `yaml:"refresh_ttl"`
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: "50051",
			Host: "0.0.0.0",
		},
		Database: DatabaseConfig{
			Host:     "localhost",
			Port:     "5432",
			User:     "postgres",
			Password: "postgres",
			Name:     "gen_db",
			SSLMode:  "disable",
		},
		JWT: JWTConfig{
			AccessSecret:  "gen",
			RefreshSecret: "gen",
			AccessTTL:     15 * time.Minute,
			RefreshTTL:    7 * 24 * time.Hour,
		},
		Runners: RunnersConfig{
			Entries: nil,
		},
		UploadDir:      "./uploads",
		LogLevel:       "info",
		MinClientBuild: 1,
	}
}

func Load() (*Config, error) {
	return LoadFrom("config.yaml")
}

func LoadFrom(path string) (*Config, error) {
	loadedMu.Lock()
	defer loadedMu.Unlock()

	c := defaultConfig()
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("путь к файлу конфигурации пустой")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			LoadedFrom = fmt.Sprintf("встроенные значения по умолчанию (файл не найден: %s)", path)
			if err := c.validate(); err != nil {
				return nil, err
			}
			return c, nil
		}
		return nil, fmt.Errorf("чтение конфигурации %q: %w", path, err)
	}

	var raw yamlRoot
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("разбор YAML %q: %w", path, err)
	}

	if err := mergeYAML(c, &raw); err != nil {
		return nil, fmt.Errorf("конфигурация %q: %w", path, err)
	}
	if err := parseJWTTTL(c, &raw.JWT); err != nil {
		return nil, fmt.Errorf("jwt ttl в %q: %w", path, err)
	}

	LoadedFrom = path
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("конфигурация %q: %w", path, err)
	}
	return c, nil
}

func (c *Config) validate() error {
	if len(c.Runners.Entries) == 0 {
		return fmt.Errorf("нужна хотя бы одна запись в runners в конфигурационном файле")
	}
	seenTok := make(map[string]struct{})
	for i, e := range c.Runners.Entries {
		tok := e.EffectiveToken()
		addr := strings.TrimSpace(e.Address)
		if tok == "" && addr == "" {
			return fmt.Errorf("runners[%d]: укажите token (саморегистрация) или address (статический раннер)", i)
		}
		if tok != "" {
			if _, dup := seenTok[tok]; dup {
				return fmt.Errorf("runners: повторяется token")
			}
			seenTok[tok] = struct{}{}
		}
	}
	return nil
}

func (c *Config) EntryMatchingRegistrationToken(provided string) *RunnerEntry {
	given := strings.TrimSpace(provided)
	if given == "" {
		return nil
	}
	for i := range c.Runners.Entries {
		e := &c.Runners.Entries[i]
		exp := e.EffectiveToken()
		if exp == "" {
			continue
		}
		if len(given) != len(exp) {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(given), []byte(exp)) == 1 {
			return e
		}
	}
	return nil
}

func mergeYAML(dst *Config, raw *yamlRoot) error {
	if raw.Server.Host != "" {
		dst.Server.Host = raw.Server.Host
	}
	if raw.Server.Port != "" {
		dst.Server.Port = raw.Server.Port
	}
	if raw.Database.Host != "" {
		dst.Database.Host = raw.Database.Host
	}
	if raw.Database.Port != "" {
		dst.Database.Port = raw.Database.Port
	}
	if raw.Database.User != "" {
		dst.Database.User = raw.Database.User
	}
	if raw.Database.Password != nil {
		dst.Database.Password = *raw.Database.Password
	}
	if raw.Database.Name != "" {
		dst.Database.Name = raw.Database.Name
	}
	if raw.Database.SSLMode != "" {
		dst.Database.SSLMode = raw.Database.SSLMode
	}
	if raw.Database.MaxOpenConns != 0 {
		dst.Database.MaxOpenConns = raw.Database.MaxOpenConns
	}
	if raw.Database.MaxIdleConns != 0 {
		dst.Database.MaxIdleConns = raw.Database.MaxIdleConns
	}
	if strings.TrimSpace(raw.Database.ConnMaxLifetime) != "" {
		dst.Database.ConnMaxLifetime = strings.TrimSpace(raw.Database.ConnMaxLifetime)
	}
	if strings.TrimSpace(raw.Database.ConnMaxIdleTime) != "" {
		dst.Database.ConnMaxIdleTime = strings.TrimSpace(raw.Database.ConnMaxIdleTime)
	}
	if raw.Database.ConnectTimeoutSec != 0 {
		dst.Database.ConnectTimeoutSec = raw.Database.ConnectTimeoutSec
	}
	if raw.JWT.AccessSecret != "" {
		dst.JWT.AccessSecret = raw.JWT.AccessSecret
	}
	if raw.JWT.RefreshSecret != "" {
		dst.JWT.RefreshSecret = raw.JWT.RefreshSecret
	}
	if err := mergeRunnersFromYAML(dst, raw.Runners); err != nil {
		return err
	}
	if raw.UploadDir != "" {
		dst.UploadDir = raw.UploadDir
	}
	if raw.AttachmentHydrateParallelism != 0 {
		dst.AttachmentHydrateParallelism = raw.AttachmentHydrateParallelism
	}
	if raw.LogLevel != "" {
		dst.LogLevel = raw.LogLevel
	}
	if raw.MinClientBuild != 0 {
		dst.MinClientBuild = raw.MinClientBuild
	}
	return nil
}

func mergeRunnersFromYAML(dst *Config, block *runnersBlockYAML) error {
	if block == nil {
		return nil
	}
	entries := make([]RunnerEntry, 0, len(block.Entries))
	for i, e := range block.Entries {
		tok := strings.TrimSpace(e.Token)
		if tok == "" {
			tok = strings.TrimSpace(e.RegistrationToken)
		}
		addr := strings.TrimSpace(e.Address)
		if addr == "" && tok == "" {
			return fmt.Errorf("runners[%d]: пустая запись (нужен token и/или address)", i)
		}
		entries = append(entries, RunnerEntry{
			Name:              strings.TrimSpace(e.Name),
			Address:           addr,
			Token:             strings.TrimSpace(e.Token),
			RegistrationToken: strings.TrimSpace(e.RegistrationToken),
		})
	}
	dst.Runners.Entries = entries
	return nil
}

func parseJWTTTL(dst *Config, y *jwtYAML) error {
	if strings.TrimSpace(y.AccessTTL) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(y.AccessTTL))
		if err != nil {
			return fmt.Errorf("access_ttl: %w", err)
		}
		dst.JWT.AccessTTL = d
	}
	if strings.TrimSpace(y.RefreshTTL) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(y.RefreshTTL))
		if err != nil {
			return fmt.Errorf("refresh_ttl: %w", err)
		}
		dst.JWT.RefreshTTL = d
	}
	return nil
}

func (c DatabaseConfig) PostgresDSN() (string, error) {
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return "", fmt.Errorf("database: поле name обязательно")
	}

	host := strings.TrimSpace(c.Host)
	port := strings.TrimSpace(c.Port)
	if host == "" || port == "" {
		return "", fmt.Errorf("database: host и port обязательны")
	}

	u := &url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + strings.TrimPrefix(name, "/"),
	}

	user := strings.TrimSpace(c.User)
	if user != "" {
		u.User = url.UserPassword(user, c.Password)
	}

	q := url.Values{}
	if sm := strings.TrimSpace(c.SSLMode); sm != "" {
		q.Set("sslmode", sm)
	}

	if c.ConnectTimeoutSec > 0 {
		q.Set("connect_timeout", strconv.Itoa(c.ConnectTimeoutSec))
	}

	if len(q) > 0 {
		u.RawQuery = q.Encode()
	}

	return u.String(), nil
}

func (c DatabaseConfig) AdminPostgresDSN() (string, error) {
	admin := c
	admin.Name = "postgres"
	return admin.PostgresDSN()
}

func (c DatabaseConfig) TargetDBName() (string, error) {
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return "", fmt.Errorf("database: поле name обязательно")
	}

	return name, nil
}
