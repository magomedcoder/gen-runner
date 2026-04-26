package huggingface

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/magomedcoder/gen/llm-runner/service"
	"golang.org/x/sync/errgroup"
)

type Options struct {
	RepoID      string
	File        string
	Revision    string
	Token       string
	ListOnly    bool
	OutDir      string
	AsRef       string
	LogOutput   io.Writer
	Context     context.Context
	Concurrency int
	progressMu  *sync.Mutex
}

func logf(opts Options, format string, args ...any) {
	w := opts.LogOutput
	if w == nil {
		w = os.Stdout
	}

	_, _ = fmt.Fprintf(w, format, args...)
}

func formatBytes(n int64) string {
	if n < 0 {
		return "?"
	}

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

func optsContext(opts Options) context.Context {
	if opts.Context != nil {
		return opts.Context
	}

	return context.Background()
}

func effectiveParallelDownload(requested, nFiles int) int {
	if nFiles <= 1 {
		return 1
	}

	if requested == 1 {
		return 1
	}

	limit := 4
	if requested > 1 {
		limit = requested
	}

	if limit > 8 {
		limit = 8
	}

	if limit > nFiles {
		limit = nFiles
	}

	return limit
}

func downloadFile(opts Options, client *Client, repoID, revision, filename, outDir string) error {
	ctx := optsContext(opts)
	dstPath := filepath.Join(outDir, filename)
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}

	dp := newDownloadProgress(opts.LogOutput, filename, opts.progressMu)
	n, total, err := client.DownloadToPath(ctx, repoID, revision, filename, dstPath, dp.report)
	dp.finish(n, total)
	if err != nil {
		if st, e := os.Stat(dstPath); e == nil && st.Size() == 0 {
			_ = os.Remove(dstPath)
		}

		return err
	}

	if n == 0 {
		_ = os.Remove(dstPath)
		return fmt.Errorf("%s: пустой ответ", filename)
	}

	return nil
}

func Run(opts Options) error {
	if strings.TrimSpace(opts.RepoID) == "" {
		return fmt.Errorf("укажите repo")
	}

	if opts.Revision == "" {
		opts.Revision = "main"
	}

	if strings.TrimSpace(opts.OutDir) == "" {
		return fmt.Errorf("не задан каталог для сохранения (OutDir)")
	}

	ctx := optsContext(opts)
	client := NewClient(opts.Token)
	info, err := client.ModelInfo(ctx, opts.RepoID)
	if err != nil {
		return fmt.Errorf("информация о модели: %w", err)
	}

	ggufFiles := info.GGFFFilenames()
	if len(ggufFiles) == 0 {
		return fmt.Errorf("в репозитории нет .gguf файлов")
	}

	modelDir := filepath.Join(opts.OutDir, RepoDownloadSubdir(opts.RepoID))

	if opts.ListOnly {
		logf(opts, "Доступные .gguf файлы:\n")
		for _, name := range ggufFiles {
			logf(opts, " %s\n", name)
		}

		logf(opts, "После скачивания файлы будут в: %s\n", modelDir)

		return nil
	}

	toDownload := ggufFiles
	if opts.File != "" {
		found := false
		for _, f := range ggufFiles {
			if f == opts.File {
				found = true
				toDownload = []string{f}
				break
			}
		}

		if !found {
			return fmt.Errorf("файл %q не найден в репозитории (используйте -list)", opts.File)
		}
	}

	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		return fmt.Errorf("каталог %s: %w", modelDir, err)
	}

	logf(opts, "Каталог модели: %s\n", modelDir)

	asTrim := strings.TrimSpace(opts.AsRef)
	if asTrim != "" && len(toDownload) != 1 {
		return fmt.Errorf("-as допустим только при одном файле (укажите -file)")
	}

	conc := effectiveParallelDownload(opts.Concurrency, len(toDownload))
	if conc > 1 {
		logf(opts, "Параллельных загрузок: %d\n", conc)
		var mu sync.Mutex
		opts.progressMu = &mu

		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(conc)

		for _, name := range toDownload {
			g.Go(func() error {
				o := opts
				o.Context = gctx
				if err := downloadFile(o, client, opts.RepoID, opts.Revision, name, modelDir); err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}

				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return err
		}
	} else {
		for _, name := range toDownload {
			if err := ctx.Err(); err != nil {
				return err
			}

			if err := downloadFile(opts, client, opts.RepoID, opts.Revision, name, modelDir); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}

	if asTrim != "" && len(toDownload) == 1 {
		name := toDownload[0]
		base, tag := service.SplitModelRef(asTrim)
		if base == "" {
			return fmt.Errorf("-as: укажите имя, например phi-3:Q4_K_M")
		}

		var newName string
		if tag == "" || strings.EqualFold(tag, "latest") {
			newName = base + ".gguf"
		} else {
			newName = fmt.Sprintf("%s-%s.gguf", base, tag)
		}

		oldPath := filepath.Join(modelDir, name)
		newPath := filepath.Join(modelDir, newName)
		if filepath.Clean(oldPath) != filepath.Clean(newPath) {
			if err := os.Rename(oldPath, newPath); err != nil {
				return fmt.Errorf("переименование в %s: %w", newName, err)
			}

			logf(opts, "сохранено как %s\n", newName)
		}
	}

	return nil
}
