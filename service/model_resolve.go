package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

func normalizeCatalogKey(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}

	return strings.ToLower(filepath.Clean(strings.ReplaceAll(v, "\\", "/")))
}

func putUniqueCatalogValue(byKey map[string]string, value string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return
	}

	key := normalizeCatalogKey(trimmed)
	if key == "" {
		return
	}

	prev, exists := byKey[key]
	if !exists || len(trimmed) < len(prev) {
		byKey[key] = trimmed
	}
}

func addCatalogEntriesFromStem(seen map[string]struct{}, stem string) {
	stem = strings.TrimSpace(stem)
	if stem == "" {
		return
	}

	visible := filepath.Base(stem)
	visible = strings.TrimSpace(visible)
	if visible == "" || visible == "." || visible == string(filepath.Separator) {
		return
	}

	seen[visible] = struct{}{}
	if b, tg, ok := splitStemToTagged(visible); ok {
		seen[b+":"+tg] = struct{}{}
	}
}

// DisplayModelName возвращает отображаемое имя: для весов в подкаталоге - папка/стем без .gguf
func DisplayModelName(relPath string) string {
	p := filepath.Clean(relPath)
	base := filepath.Base(p)
	if len(base) <= 5 || !strings.EqualFold(base[len(base)-5:], ".gguf") {
		return base
	}

	stem := base[:len(base)-5]
	d := filepath.Dir(p)
	if d == "." {
		return stem
	}

	return filepath.Join(d, stem)
}

// ListGGUFRelPaths рекурсивно собирает относительные пути всех .gguf от корня dir
func ListGGUFRelPaths(dir string) ([]string, error) {
	dir = filepath.Clean(dir)
	var out []string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if d.Name() == ".git" && path != dir {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.EqualFold(filepath.Ext(d.Name()), ".gguf") {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		out = append(out, rel)

		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(out)

	return out, nil
}

// ListGGUFBasenames совместимое имя: возвращает относительные пути .gguf (рекурсивно), не только basename
func ListGGUFBasenames(dir string) ([]string, error) {
	return ListGGUFRelPaths(dir)
}

func SplitModelRef(userInput string) (name, tag string) {
	s := strings.TrimSpace(userInput)
	if s == "" {
		return "", ""
	}

	if strings.Contains(s, "/") || strings.Contains(s, string(filepath.Separator)) {
		return s, ""
	}

	i := strings.LastIndex(s, ":")
	if i <= 0 || i+1 >= len(s) {
		return s, ""
	}

	return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
}

func reasonableTag(tag string) bool {
	if tag == "" || len(tag) > 48 {
		return false
	}

	for _, r := range tag {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' {
			continue
		}

		return false
	}

	return true
}

func splitStemToTagged(stem string) (base, tag string, ok bool) {
	if stem == "" {
		return "", "", false
	}

	i := strings.LastIndex(stem, "-")
	if i <= 0 || i+1 >= len(stem) {
		return "", "", false
	}

	base, tag = stem[:i], stem[i+1:]
	if base == "" || !reasonableTag(tag) {
		return "", "", false
	}

	return base, tag, true
}

func CatalogModelNames(dir string) ([]string, error) {
	files, err := ListGGUFBasenames(dir)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(files)*2)
	for _, f := range files {
		stem := DisplayModelName(f)
		addCatalogEntriesFromStem(seen, stem)
	}

	if err := addManifestCatalogEntries(dir, seen); err != nil {
		return nil, err
	}

	byKey := make(map[string]string, len(seen))
	for k := range seen {
		putUniqueCatalogValue(byKey, k)
	}

	out := make([]string, 0, len(byKey))
	for _, v := range byKey {
		out = append(out, v)
	}
	sort.Strings(out)

	return out, nil
}

func SortedDisplayModelNames(dir string) ([]string, error) {
	return CatalogModelNames(dir)
}

func ResolveGGUFFile(modelsDir, userInput string) (canonical string, err error) {
	raw := strings.TrimSpace(userInput)
	if raw == "" {
		return "", fmt.Errorf("пустое имя модели")
	}

	refName, refTag := SplitModelRef(raw)

	if refTag != "" && !strings.EqualFold(refTag, "latest") {
		candRel := filepath.Clean(strings.ReplaceAll(
			fmt.Sprintf("%s-%s.gguf", refName, refTag),
			"/",
			string(filepath.Separator),
		))

		full := filepath.Join(modelsDir, candRel)
		if fi, e := os.Stat(full); e == nil && !fi.IsDir() && strings.EqualFold(filepath.Ext(candRel), ".gguf") {
			return candRel, nil
		}

		if c, err := matchGGUFBasename(modelsDir, filepath.Base(candRel)); err == nil && c != "" {
			return c, nil
		}

		return "", fmt.Errorf("модель %q не найдена", userInput)
	}

	if refTag == "" || strings.EqualFold(refTag, "latest") {
		relTry := filepath.Clean(strings.ReplaceAll(refName, "/", string(filepath.Separator)))
		if relTry != "." {
			full := filepath.Join(modelsDir, relTry)
			if fi, e := os.Stat(full); e == nil && !fi.IsDir() {
				if strings.EqualFold(filepath.Ext(relTry), ".gguf") {
					return relTry, nil
				}
			}

			fullGGUF := filepath.Join(modelsDir, relTry+".gguf")
			if fi, e := os.Stat(fullGGUF); e == nil && !fi.IsDir() {
				return relTry + ".gguf", nil
			}
		}
	}

	base := filepath.Base(raw)
	if base == "." || base == string(filepath.Separator) {
		return "", fmt.Errorf("некорректное имя модели")
	}

	lookup := filepath.Base(refName)
	if lookup == "." || lookup == string(filepath.Separator) {
		lookup = base
	}

	try := filepath.Join(modelsDir, lookup)
	if st, e := os.Stat(try); e == nil && !st.IsDir() {
		if strings.EqualFold(filepath.Ext(lookup), ".gguf") {
			rel, err := filepath.Rel(modelsDir, try)
			if err != nil {
				return filepath.Clean(lookup), nil
			}

			return rel, nil
		}
	}

	if filepath.Ext(lookup) == "" {
		cand := lookup + ".gguf"
		try2 := filepath.Join(modelsDir, cand)
		if _, e := os.Stat(try2); e == nil {
			return cand, nil
		}
	}

	files, err := ListGGUFBasenames(modelsDir)
	if err != nil {
		return "", err
	}

	for _, name := range files {
		if name == lookup {
			return name, nil
		}
	}

	for _, name := range files {
		if DisplayModelName(name) == lookup {
			return name, nil
		}
	}

	for _, name := range files {
		if strings.EqualFold(name, lookup) {
			return name, nil
		}
	}

	for _, name := range files {
		if strings.EqualFold(DisplayModelName(name), lookup) {
			return name, nil
		}
	}

	for _, name := range files {
		st := DisplayModelName(name)
		if filepath.Base(st) == lookup || strings.EqualFold(filepath.Base(st), lookup) {
			return name, nil
		}
	}

	return "", fmt.Errorf("модель %q не найдена", userInput)
}

func matchGGUFBasename(modelsDir, want string) (string, error) {
	want = filepath.Clean(strings.ReplaceAll(strings.TrimSpace(want), "/", string(filepath.Separator)))
	files, err := ListGGUFBasenames(modelsDir)
	if err != nil {
		return "", err
	}

	var hits []string
	for _, name := range files {
		if name == want || strings.EqualFold(name, want) {
			hits = append(hits, name)

			continue
		}

		if filepath.Base(name) == want || strings.EqualFold(filepath.Base(name), want) {
			hits = append(hits, name)
		}
	}

	keyed := make(map[string]string, len(hits))
	for _, h := range hits {
		k := strings.ToLower(filepath.Clean(h))
		keyed[k] = h
	}

	if len(keyed) == 1 {
		for _, v := range keyed {
			return v, nil
		}
	}

	if len(keyed) > 1 {
		all := make([]string, 0, len(keyed))
		for _, v := range keyed {
			all = append(all, v)
		}

		sort.Strings(all)

		return "", fmt.Errorf("несколько файлов подходят под %q: %v", want, all)
	}

	return "", fmt.Errorf("в каталоге моделей %q нет файла .gguf с именем %q (проверьте имя файла и расширение .gguf)", modelsDir, want)
}
