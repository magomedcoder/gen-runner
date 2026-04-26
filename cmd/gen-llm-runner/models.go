package main

import (
	"context"
	"fmt"
	"github.com/magomedcoder/gen/llm-runner/config"
	"github.com/urfave/cli/v3"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func cmdModels() *cli.Command {
	return &cli.Command{
		Name:    "models",
		Aliases: []string{"ls"},
		Usage:   "Список скачанных локальных .gguf (каталог из model_path в config)",
		Action:  runListModels,
	}
}

type localModel struct {
	rel  string
	size int64
}

func runListModels(_ context.Context, _ *cli.Command) error {
	cfg, err := config.Load("./configs/config-llm-runner.yaml")
	if err != nil {
		return err
	}
	absDir, err := cfg.RequireAbsModelsDir()
	if err != nil {
		return fmt.Errorf("каталог: %w", err)
	}

	if st, err := os.Stat(absDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("каталог не найден: %s", absDir)
		}

		return fmt.Errorf("каталог %s: %w", absDir, err)
	} else if !st.IsDir() {
		return fmt.Errorf("не каталог: %s", absDir)
	}

	var list []localModel
	err = filepath.WalkDir(absDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		if !strings.EqualFold(filepath.Ext(path), ".gguf") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(absDir, path)
		if err != nil {
			rel = filepath.Base(path)
		}

		list = append(list, localModel{rel: rel, size: info.Size()})
		return nil
	})
	if err != nil {
		return err
	}

	sort.Slice(list, func(i, j int) bool {
		return strings.ToLower(list[i].rel) < strings.ToLower(list[j].rel)
	})

	if len(list) == 0 {
		fmt.Println("(нет .gguf файлов)")
		return nil
	}

	for _, m := range list {
		fmt.Printf("%s\t%s\n", formatBytesHuman(m.size), m.rel)
	}

	return nil
}

func formatBytesHuman(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}

	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
