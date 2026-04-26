package mcpclient

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/magomedcoder/gen/internal/domain"
)

func moduleRootDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	for {
		_, err := os.Stat(filepath.Join(dir, "go.mod"))
		if err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("go.mod не найден - тест только из корня модуля gen")
		}

		dir = parent
	}
}

func exeName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}

	return base
}

func TestIntegrationMCPDemoStdioListTools(t *testing.T) {
	if testing.Short() {
		t.Skip("короткий прогон: пропуск сборки mcp-demo")
	}

	root := moduleRootDir(t)
	bin := filepath.Join(t.TempDir(), exeName("mcp-stdio-demo"))
	build := exec.Command("go", "build", "-o", bin, "./mcp-servers/mcp-demo/cmd/mcp-stdio-demo")
	build.Dir = root
	build.Env = os.Environ()
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build mcp-stdio-demo: %v\n%s", err, out)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	srv := &domain.MCPServer{
		Transport:      "stdio",
		Command:        bin,
		ArgsJSON:       `[]`,
		EnvJSON:        `{}`,
		HeadersJSON:    `{}`,
		TimeoutSeconds: 40,
	}

	tools, err := ListTools(ctx, srv)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	var names []string
	for _, x := range tools {
		if x.Name != "" {
			names = append(names, x.Name)
		}
	}

	found := slices.Contains(names, "ping")

	if !found {
		t.Fatalf("ожидался tool ping, получено: %v", names)
	}
}
