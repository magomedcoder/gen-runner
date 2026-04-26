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

func cmdShow() *cli.Command {
	return &cli.Command{
		Name:  "show",
		Usage: "Показать YAML-манифест модели (sidecar или алиас)",
		Description: "Ищет stem.yaml рядом с .gguf или манифест по имени алиаса (как default_model).\n" +
			"Флаг --modelfile конвертирует YAML в текст Modelfile.\n" +
			"При отсутствии YAML для чистого .gguf завершается с ошибкой (см. create).",
		UsageText: "gen-llm-runner show [options] <имя>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "path-only",
				Usage: "вывести только абсолютный путь к .yaml",
			},
			&cli.BoolFlag{
				Name:    "modelfile",
				Aliases: []string{"m"},
				Usage:   "вывести эквивалентный Modelfile (вместо YAML)",
			},
		},
		Action: runShow,
	}
}

func runShow(ctx context.Context, cmd *cli.Command) error {
	_ = ctx
	name := strings.TrimSpace(cmd.Args().First())
	if name == "" {
		return fmt.Errorf("укажите имя модели, например: myalias или Base-Q4.gguf")
	}

	cfg, err := config.Load("./configs/config-llm-runner.yaml")
	if err != nil {
		return err
	}
	absDir, err := cfg.RequireAbsModelsDir()
	if err != nil {
		return err
	}

	yamlPath, raw, weights, err := service.LoadManifestYAMLForShow(absDir, name)
	if err != nil {
		return err
	}

	absYAML, err := filepath.Abs(yamlPath)
	if err != nil {
		absYAML = yamlPath
	}

	if cmd.Bool("path-only") {
		fmt.Println(absYAML)
		return nil
	}

	if weights != "" {
		fmt.Fprintf(os.Stderr, "# манифест: %s\n# веса: %s\n", absYAML, weights)
	} else {
		fmt.Fprintf(os.Stderr, "# манифест: %s\n", absYAML)
	}

	if cmd.Bool("modelfile") {
		cfg, err := service.ParseModelYAMLData(raw)
		if err != nil {
			return fmt.Errorf("разбор YAML: %w", err)
		}

		mf, err := service.EncodeModelfile(cfg, weights)
		if err != nil {
			return err
		}

		fmt.Print(mf)
		if !strings.HasSuffix(mf, "\n") {
			fmt.Println()
		}

		return nil
	}

	_, _ = os.Stdout.Write(raw)
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		fmt.Println()
	}

	return nil
}
