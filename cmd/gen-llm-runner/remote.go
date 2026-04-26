package main

import (
	"context"
	"fmt"
	"github.com/urfave/cli/v3"
	"strings"
	"time"

	"github.com/magomedcoder/gen/api/pb/llm-runner/llmrunnerpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func cmdRemote() *cli.Command {
	return &cli.Command{
		Name:  "remote",
		Usage: "Команды к запущенному демону",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "addr",
				Aliases: []string{"a"},
				Value:   "127.0.0.1:50052",
				Usage:   "адрес раннера",
				Sources: cli.EnvVars("LLM_RUNNER_ADDR"),
			},
		},
		Commands: []*cli.Command{
			cmdRemotePing(),
			cmdRemoteModels(),
			cmdRemotePs(),
			cmdRemoteUnload(),
			cmdRemoteRun(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return cli.ShowSubcommandHelp(cmd)
		},
	}
}

func grpcConnect(addr string) (*grpc.ClientConn, llmrunnerpb.LLMRunnerServiceClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}

	return conn, llmrunnerpb.NewLLMRunnerServiceClient(conn), nil
}

func remoteAddr(cmd *cli.Command) string {
	return cmd.String("addr")
}

func cmdRemotePing() *cli.Command {
	return &cli.Command{
		Name:  "ping",
		Usage: "Проверить соединение",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			addr := remoteAddr(cmd)
			cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			conn, c, err := grpcConnect(addr)
			if err != nil {
				return err
			}
			defer conn.Close()

			r, err := c.CheckConnection(cctx, &llmrunnerpb.Empty{})
			if err != nil {
				return err
			}

			if r.GetIsConnected() {
				fmt.Println("ok: раннер отвечает")
				return nil
			}
			fmt.Println("раннер не готов (провайдер не подключён)")

			return nil
		},
	}
}

func cmdRemoteModels() *cli.Command {
	return &cli.Command{
		Name:  "models",
		Usage: "Список моделей в каталоге раннера",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			addr := remoteAddr(cmd)
			cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			conn, c, err := grpcConnect(addr)
			if err != nil {
				return err
			}
			defer conn.Close()

			r, err := c.GetModels(cctx, &llmrunnerpb.Empty{})
			if err != nil {
				return err
			}

			for _, m := range r.GetModels() {
				fmt.Println(m)
			}

			return nil
		},
	}
}

func cmdRemotePs() *cli.Command {
	return &cli.Command{
		Name:  "ps",
		Usage: "Какая модель загружена в память",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			addr := remoteAddr(cmd)
			cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			conn, c, err := grpcConnect(addr)
			if err != nil {
				return err
			}

			defer conn.Close()
			r, err := c.GetLoadedModel(cctx, &llmrunnerpb.Empty{})
			if err != nil {
				return err
			}

			if !r.GetLoaded() {
				fmt.Println("модель не загружена")
				return nil
			}

			fmt.Printf("display_name:  %s\n", r.GetDisplayName())
			fmt.Printf("gguf_basename: %s\n", r.GetGgufBasename())

			return nil
		},
	}
}

func cmdRemoteUnload() *cli.Command {
	return &cli.Command{
		Name:  "unload",
		Usage: "Выгрузить модель из памяти",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			addr := remoteAddr(cmd)
			cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			conn, c, err := grpcConnect(addr)
			if err != nil {
				return err
			}

			defer conn.Close()
			_, err = c.UnloadModel(cctx, &llmrunnerpb.Empty{})
			if err != nil {
				return err
			}

			fmt.Println("ok: выгрузка запрошена")

			return nil
		},
	}
}

func cmdRemoteRun() *cli.Command {
	return &cli.Command{
		Name:  "run",
		Usage: "Короткий запрос к раннеру",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "model",
				Usage: "имя модели (пусто - default_model раннера)",
			},
			&cli.StringFlag{
				Name:  "prompt",
				Value: "привет",
				Usage: "сообщение user",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			addr := remoteAddr(cmd)
			cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()

			conn, c, err := grpcConnect(addr)
			if err != nil {
				return err
			}
			defer conn.Close()

			req := &llmrunnerpb.SendMessageRequest{
				Model: strings.TrimSpace(cmd.String("model")),
				Messages: []*llmrunnerpb.ChatMessage{
					{
						Id:        1,
						Role:      "user",
						Content:   cmd.String("prompt"),
						CreatedAt: time.Now().Unix(),
					},
				},
			}
			stream, err := c.SendMessage(cctx, req)
			if err != nil {
				return err
			}

			for {
				msg, err := stream.Recv()
				if err != nil {
					return err
				}

				if r := msg.GetReasoningContent(); r != "" {
					fmt.Print(r)
				}

				if msg.GetContent() != "" {
					fmt.Print(msg.GetContent())
				}

				if msg.GetDone() {
					break
				}
			}

			fmt.Println()

			return nil
		},
	}
}
