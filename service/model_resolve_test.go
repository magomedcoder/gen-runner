package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDisplayModelName(t *testing.T) {
	if g := DisplayModelName("Qwen-7B.Q4.gguf"); g != "Qwen-7B.Q4" {
		t.Fatalf("DisplayModelName: получено %q, ожидалось Qwen-7B.Q4", g)
	}

	if g := DisplayModelName("LOWER.GGUF"); g != "LOWER" {
		t.Fatalf("DisplayModelName: получено %q, ожидалось LOWER", g)
	}

	sub := filepath.Join("org", "repo", "w.gguf")
	if g := DisplayModelName(sub); g != filepath.Join("org", "repo", "w") {
		t.Fatalf("DisplayModelName nested: получено %q", g)
	}
}

func TestResolveGGUFFile_nested(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "org", "repo")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	p := filepath.Join(sub, "w.gguf")
	if err := os.WriteFile(p, []byte{0}, 0o644); err != nil {
		t.Fatal(err)
	}

	rel := filepath.Join("org", "repo", "w.gguf")
	got, err := ResolveGGUFFile(dir, rel)
	if err != nil || got != rel {
		t.Fatalf("ResolveGGUFFile nested by path: %q err=%v", got, err)
	}

	got2, err := ResolveGGUFFile(dir, "w")
	if err != nil || got2 != rel {
		t.Fatalf("ResolveGGUFFile nested by stem: %q err=%v", got2, err)
	}
}

func TestResolveGGUFFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "MyModel-Q4.gguf"), []byte{0}, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveGGUFFile(dir, "MyModel-Q4")
	if err != nil || got != "MyModel-Q4.gguf" {
		t.Fatalf("ResolveGGUFFile: получено %q, ошибка %v", got, err)
	}

	got, err = ResolveGGUFFile(dir, "MyModel-Q4.gguf")
	if err != nil || got != "MyModel-Q4.gguf" {
		t.Fatalf("ResolveGGUFFile (с суффиксом .gguf): получено %q, ошибка %v", got, err)
	}

	got, err = ResolveGGUFFile(dir, "mymodel-q4")
	if err != nil || got != "MyModel-Q4.gguf" {
		t.Fatalf("ResolveGGUFFile (регистронезависимо): получено %q, ошибка %v", got, err)
	}

	if _, err := ResolveGGUFFile(dir, "missing"); err == nil {
		t.Fatal("ожидалась ошибка для отсутствующей модели")
	}
}

func TestSortedDisplayModelNames(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "b.gguf"), []byte{}, 0o644)
	_ = os.WriteFile(filepath.Join(dir, "a.gguf"), []byte{}, 0o644)
	got, err := SortedDisplayModelNames(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("ожидался порядок [a b], получено %v", got)
	}
}

func TestSortedDisplayModelNames_nestedShowsOnlyFinalName(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "vendor", "qwen")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "qwen3-14b-q4.gguf"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := SortedDisplayModelNames(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Fatalf("ожидались 2 записи (stem + tag), получено %d: %v", len(got), got)
	}
	if got[0] != "qwen3-14b-q4" || got[1] != "qwen3-14b:q4" {
		t.Fatalf("ожидались только конечные имена без папок, получено %v", got)
	}
}

func TestSplitModelRef(t *testing.T) {
	n, tg := SplitModelRef("phi:q4")
	if n != "phi" || tg != "q4" {
		t.Fatalf("SplitModelRef: получено имя %q тег %q", n, tg)
	}

	n, tg = SplitModelRef("phi:latest")
	if n != "phi" || tg != "latest" {
		t.Fatalf("SplitModelRef (latest): получено имя %q тег %q", n, tg)
	}

	n, tg = SplitModelRef("org/model/file")
	if n != "org/model/file" || tg != "" {
		t.Fatalf("путь с / не должен делиться на имя и тег: %q %q", n, tg)
	}
}

func TestResolveGGUFFile_tagRef(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Phi-3-Q4.gguf"), []byte{0}, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveGGUFFile(dir, "Phi-3:Q4")
	if err != nil || got != "Phi-3-Q4.gguf" {
		t.Fatalf("ResolveGGUFFile с тегом: получено %q, ошибка %v", got, err)
	}

	if _, err := ResolveGGUFFile(dir, "Phi-3:missing"); err == nil {
		t.Fatal("ожидалась ошибка для несуществующего тега")
	}

	_ = os.WriteFile(filepath.Join(dir, "Phi-3.gguf"), []byte{0}, 0o644)
	if _, err := ResolveGGUFFile(dir, "Phi-3:missing"); err == nil {
		t.Fatal("при запросе с тегом нельзя подставлять файл без тега")
	}
}

func TestCatalogModelNames_tagAlias(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "Phi-3-Q4.gguf"), []byte{}, 0o644)
	got, err := CatalogModelNames(dir)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{"Phi-3-Q4": false, "Phi-3:Q4": false}
	for _, s := range got {
		if _, ok := want[s]; ok {
			want[s] = true
		}
	}

	for k, v := range want {
		if !v {
			t.Errorf("в каталоге нет записи %q, каталог: %v", k, got)
		}
	}
}

func TestCatalogModelNames_deduplicatesCaseVariants(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "Model-Q4.gguf"), []byte{}, 0o644)
	_ = os.WriteFile(filepath.Join(dir, "model-q4.gguf"), []byte{}, 0o644)

	got, err := CatalogModelNames(dir)
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for _, s := range got {
		if strings.EqualFold(s, "model-q4") {
			count++
		}
	}

	if count != 1 {
		t.Fatalf("ожидался один вариант model-q4, получено %d; каталог: %v", count, got)
	}
}
