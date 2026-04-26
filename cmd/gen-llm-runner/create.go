package main

import (
	"context"
	"fmt"
	"github.com/magomedcoder/gen/llm-runner/config"
	"github.com/urfave/cli/v3"
	"os"
	"path/filepath"
	"strings"

	"github.com/magomedcoder/gen/llm-runner/service"
)

func cmdCreate() *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "Создать YAML-манифест модели из Modelfile",
		Description: "Пишет <каталог>/<имя>.yaml. Имя может быть с тегом: mymodel:q4 -> файл mymodel-q4.yaml.\n\n" +
			"Директивы: FROM, SYSTEM, TEMPLATE (\"\"\"), PARAMETER (temperature, top_p, top_k, min_p, max_tokens|num_predict - 0 или не задавать = до границы контекста, num_ctx, stop, repeat_last_n, repeat_penalty, seed), MESSAGE, REQUIRES (игнор).\n" +
			"TEMPLATE - Jinja для llama.cpp.",
		UsageText: "gen-llm-runner create [options] <имя>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "file",
				Aliases:  []string{"f"},
				Usage:    "путь к Modelfile",
				Required: true,
			},
			&cli.BoolFlag{
				Name:  "force",
				Usage: "перезаписать существующий .yaml",
			},
		},
		Action: runCreate,
	}
}

func runCreate(ctx context.Context, cmd *cli.Command) error {
	_ = ctx
	name := strings.TrimSpace(cmd.Args().First())
	if name == "" {
		return fmt.Errorf("укажите имя модели (как в default_model / GetModels), например: mybot или mybot:q4")
	}

	stem := service.ManifestYAMLStemForRef(name)

	appCfg, err := config.Load("./configs/config-llm-runner.yaml")
	if err != nil {
		return err
	}

	absDir, err := appCfg.RequireAbsModelsDir()
	if err != nil {
		return err
	}

	modelfilePath := strings.TrimSpace(cmd.String("file"))
	raw, err := os.ReadFile(modelfilePath)
	if err != nil {
		return fmt.Errorf("чтение Modelfile: %w", err)
	}

	cfg, err := service.ParseModelfile(strings.NewReader(string(raw)))
	if err != nil {
		return err
	}

	outPath := filepath.Join(absDir, stem+".yaml")
	if err := service.WriteModelManifest(absDir, stem, cfg, cmd.Bool("force")); err != nil {
		return err
	}

	fmt.Printf("манифест записан: %s\n", outPath)

	return nil
}
