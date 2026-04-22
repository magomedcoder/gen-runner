package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/magomedcoder/gen-runner/domain"
	"gopkg.in/yaml.v3"
)

var ErrNoSidecarManifest = errors.New("нет YAML-манифеста рядом с файлом весов")

type ModelYAML struct {
	From      string              `yaml:"from,omitempty"`
	System    string              `yaml:"system,omitempty"`
	Template  string              `yaml:"template,omitempty"`
	MMProj    string              `yaml:"mmproj,omitempty"`
	Parameter *ModelYAMLParameter `yaml:"parameter,omitempty"`
	NumCtx    *int                `yaml:"num_ctx,omitempty"`
	Stop      []string            `yaml:"stop,omitempty"`
	Messages  []ModelYAMLMessage  `yaml:"messages,omitempty"`
}

type ModelYAMLMessage struct {
	Role    string `yaml:"role"`
	Content string `yaml:"content"`
}

type ModelYAMLParameter struct {
	Temperature   *float64 `yaml:"temperature"`
	TopP          *float64 `yaml:"top_p"`
	TopK          *int     `yaml:"top_k"`
	MaxTokens     *int     `yaml:"max_tokens"`
	RepeatLastN   *int     `yaml:"repeat_last_n,omitempty"`
	RepeatPenalty *float64 `yaml:"repeat_penalty,omitempty"`
	Seed          *int     `yaml:"seed,omitempty"`
	MinP          *float64 `yaml:"min_p,omitempty"`
}

func parseModelYAML(data []byte) (*ModelYAML, error) {
	var cfg ModelYAML
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func yamlPathCandidates(modelsDir, stem string) []string {
	return []string{
		filepath.Join(modelsDir, stem+".yaml"),
		filepath.Join(modelsDir, stem+".yml"),
	}
}

func readYAMLByStem(modelsDir, stem string) ([]byte, error) {
	for _, p := range yamlPathCandidates(modelsDir, stem) {
		data, err := os.ReadFile(p)
		if err == nil {
			return data, nil
		}
	}
	return nil, os.ErrNotExist
}

func ManifestYAMLStemForRef(userInput string) string {
	name, tag := SplitModelRef(userInput)
	if tag != "" && !strings.EqualFold(tag, "latest") {
		return fmt.Sprintf("%s-%s", name, tag)
	}

	return name
}

func LoadManifestYAMLForShow(modelsDir, userInput string) (yamlPath string, raw []byte, weightsBasename string, err error) {
	if strings.TrimSpace(modelsDir) == "" {
		return "", nil, "", fmt.Errorf("каталог моделей не задан")
	}

	ref := strings.TrimSpace(userInput)
	if ref == "" {
		return "", nil, "", fmt.Errorf("пустое имя модели")
	}

	canonical, errGGUF := ResolveGGUFFile(modelsDir, ref)
	if errGGUF == nil {
		stem := strings.TrimSuffix(canonical, ".gguf")
		for _, p := range yamlPathCandidates(modelsDir, stem) {
			data, e := os.ReadFile(p)
			if e == nil {
				if _, e := parseModelYAML(data); e != nil {
					return "", nil, canonical, fmt.Errorf("%s: %w", p, e)
				}

				return p, data, canonical, nil
			}
		}

		return "", nil, canonical, fmt.Errorf("%w (%s)", ErrNoSidecarManifest, canonical)
	}

	stem := ManifestYAMLStemForRef(ref)
	for _, p := range yamlPathCandidates(modelsDir, stem) {
		data, e := os.ReadFile(p)
		if e != nil {
			continue
		}

		cfg, e2 := parseModelYAML(data)
		if e2 != nil {
			return "", nil, "", fmt.Errorf("%s: %w", p, e2)
		}

		w := ""
		if f := strings.TrimSpace(cfg.From); f != "" {
			if wb, e3 := ResolveGGUFFile(modelsDir, f); e3 == nil {
				w = wb
			}
		}

		return p, data, w, nil
	}

	return "", nil, "", fmt.Errorf("модель %q: %w", ref, errGGUF)
}

func ResolveModelForInference(modelsDir, userInput string) (canonicalGGUF string, cfg *ModelYAML, err error) {
	if strings.TrimSpace(modelsDir) == "" {
		return "", nil, fmt.Errorf("каталог моделей не задан")
	}

	canonical, errGGUF := ResolveGGUFFile(modelsDir, userInput)
	if errGGUF == nil {
		stem := strings.TrimSuffix(canonical, ".gguf")
		data, e := readYAMLByStem(modelsDir, stem)
		if e != nil {
			return canonical, nil, nil
		}

		cfg, e := parseModelYAML(data)
		if e != nil {
			return "", nil, fmt.Errorf("YAML %s: %w", stem, e)
		}

		if strings.TrimSpace(cfg.From) != "" {
			cfg.From = ""
		}

		return canonical, cfg, nil
	}

	stem := ManifestYAMLStemForRef(userInput)
	data, err := readYAMLByStem(modelsDir, stem)
	if err != nil {
		return "", nil, errGGUF
	}

	cfg, err = parseModelYAML(data)
	if err != nil {
		return "", nil, fmt.Errorf("манифест %s.yaml: %w", stem, err)
	}

	from := strings.TrimSpace(cfg.From)
	if from == "" {
		return "", nil, fmt.Errorf("%w (в манифесте %s.yaml укажите from: базовый .gguf)", errGGUF, stem)
	}

	canonical, err = ResolveGGUFFile(modelsDir, from)
	if err != nil {
		return "", nil, fmt.Errorf("from в манифесте %q: %w", from, err)
	}

	return canonical, cfg, nil
}

func MergeGenParams(req *domain.GenerationParams, yamlCfg *ModelYAML) *domain.GenerationParams {
	if yamlCfg == nil || yamlCfg.Parameter == nil {
		return req
	}

	p := yamlCfg.Parameter
	out := domain.GenerationParams{}
	if req != nil {
		out.Temperature = req.Temperature
		out.MaxTokens = req.MaxTokens
		out.TopK = req.TopK
		out.TopP = req.TopP
		out.MinP = req.MinP
		out.EnableThinking = req.EnableThinking
		out.ResponseFormat = req.ResponseFormat
		if len(req.Tools) > 0 {
			out.Tools = append([]domain.Tool(nil), req.Tools...)
		}
	}

	if out.Temperature == nil && p.Temperature != nil {
		v := float32(*p.Temperature)
		out.Temperature = &v
	}

	if out.TopP == nil && p.TopP != nil {
		v := float32(*p.TopP)
		out.TopP = &v
	}

	if out.TopK == nil && p.TopK != nil {
		v := int32(*p.TopK)
		out.TopK = &v
	}

	if out.MaxTokens == nil && p.MaxTokens != nil {
		v := int32(*p.MaxTokens)
		out.MaxTokens = &v
	}

	if out.MinP == nil && p.MinP != nil {
		v := float32(*p.MinP)
		out.MinP = &v
	}

	return &out
}

func ApplyModelYAMLSystem(norm []*domain.AIChatMessage, yamlCfg *ModelYAML) []*domain.AIChatMessage {
	if yamlCfg == nil {
		return norm
	}

	extra := strings.TrimSpace(yamlCfg.System)
	if extra == "" {
		return norm
	}

	if len(norm) == 0 {
		return []*domain.AIChatMessage{domain.NewAIChatMessage(0, extra, domain.AIChatMessageRoleSystem)}
	}

	sid := norm[0].SessionId
	if norm[0].Role == domain.AIChatMessageRoleSystem {
		merged := extra + "\n\n" + strings.TrimSpace(norm[0].Content)
		cp := *norm[0]
		cp.Content = merged
		out := make([]*domain.AIChatMessage, len(norm))
		out[0] = &cp
		copy(out[1:], norm[1:])
		return out
	}

	out := make([]*domain.AIChatMessage, 0, len(norm)+1)
	out = append(out, domain.NewAIChatMessage(sid, extra, domain.AIChatMessageRoleSystem))
	out = append(out, norm...)

	return out
}

func ApplyModelYAMLMessages(norm []*domain.AIChatMessage, yamlCfg *ModelYAML) []*domain.AIChatMessage {
	if yamlCfg == nil || len(yamlCfg.Messages) == 0 {
		return norm
	}

	var sid int64
	if len(norm) > 0 {
		sid = norm[0].SessionId
	}

	insertAt := 0
	for insertAt < len(norm) && norm[insertAt].Role == domain.AIChatMessageRoleSystem {
		insertAt++
	}

	prefix := make([]*domain.AIChatMessage, 0, len(yamlCfg.Messages))
	for _, m := range yamlCfg.Messages {
		role := domain.AIFromProtoRole(m.Role)
		prefix = append(prefix, domain.NewAIChatMessage(sid, m.Content, role))
	}

	if len(norm) == 0 {
		return prefix
	}

	out := make([]*domain.AIChatMessage, 0, len(norm)+len(prefix))
	out = append(out, norm[:insertAt]...)
	out = append(out, prefix...)
	out = append(out, norm[insertAt:]...)

	return out
}

func cloneNormalizeManifestRefs(cfg *ModelYAML, modelsDir string) (*ModelYAML, error) {
	if cfg == nil {
		return nil, fmt.Errorf("пустой манифест")
	}

	cp := *cfg
	if cp.Parameter != nil {
		p := *cp.Parameter
		cp.Parameter = &p
	}

	if len(cp.Messages) > 0 {
		cp.Messages = append([]ModelYAMLMessage(nil), cp.Messages...)
	}

	if len(cp.Stop) > 0 {
		cp.Stop = append([]string(nil), cp.Stop...)
	}

	if err := normalizeManifestGGUFRefs(modelsDir, &cp); err != nil {
		return nil, err
	}

	return &cp, nil
}

func normalizeManifestGGUFRefs(modelsDir string, cp *ModelYAML) error {
	_ = modelsDir
	_ = cp
	return nil
}

func addManifestCatalogEntries(dir string, seen map[string]struct{}) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		name := e.Name()
		low := strings.ToLower(name)
		var stem string
		switch {
		case strings.HasSuffix(low, ".yaml"):
			stem = name[:len(name)-5]
		case strings.HasSuffix(low, ".yml"):
			stem = name[:len(name)-4]
		default:
			continue
		}

		if stem == "" {
			continue
		}

		gguf := filepath.Join(dir, stem+".gguf")
		if _, err := os.Stat(gguf); err == nil {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}

		cfg, err := parseModelYAML(data)
		if err != nil || strings.TrimSpace(cfg.From) == "" {
			continue
		}

		seen[stem] = struct{}{}
		if b, tg, ok := splitStemToTagged(stem); ok {
			seen[b+":"+tg] = struct{}{}
		}
	}

	return nil
}
