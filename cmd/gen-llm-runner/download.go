package main

import (
	"context"
	"github.com/magomedcoder/gen/llm-runner/config"
	"github.com/urfave/cli/v3"

	"github.com/magomedcoder/gen/llm-runner/huggingface"
)

func cmdDownload() *cli.Command {
	return &cli.Command{
		Name:    "download",
		Aliases: []string{"dl"},
		Usage:   "Скачать .gguf с Hugging Face локально (без запущенного раннера)",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "repo",
				Usage: "id репозитория Hugging Face",
			},
			&cli.StringFlag{
				Name:  "file",
				Usage: "конкретный .gguf в репо",
			},
			&cli.StringFlag{
				Name:  "revision",
				Value: "main",
				Usage: "ветка / ревизия",
			},
			&cli.StringFlag{
				Name:  "token",
				Usage: "HF token"},
			&cli.BoolFlag{
				Name:    "list",
				Aliases: []string{"l"},
				Usage:   "только список .gguf",
			},
			&cli.StringFlag{
				Name:  "as",
				Usage: "сохранить как base:tag -> base-tag.gguf",
			},
			&cli.IntFlag{
				Name:    "parallel",
				Aliases: []string{"j"},
				Value:   0,
				Usage:   "параллельных загрузок при нескольких .gguf: 0=авто (до 4), 1=по очереди, 2–8=лимит",
			},
		},
		Action: runDownload,
	}
}

func runDownload(_ context.Context, cmd *cli.Command) error {
	cfg, err := config.Load("./configs/config-llm-runner.yaml")
	if err != nil {
		return err
	}

	outDir, err := cfg.RequireAbsModelsDir()
	if err != nil {
		return err
	}

	return huggingface.Run(huggingface.Options{
		RepoID:      cmd.String("repo"),
		File:        cmd.String("file"),
		Revision:    cmd.String("revision"),
		Token:       cmd.String("token"),
		ListOnly:    cmd.Bool("list"),
		AsRef:       cmd.String("as"),
		OutDir:      outDir,
		Concurrency: cmd.Int("parallel"),
	})
}
