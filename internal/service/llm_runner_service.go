package service

import (
	"context"
	"fmt"
	"github.com/magomedcoder/gen/api/pb/llmrunnerpb"
	"github.com/magomedcoder/gen/internal/domain"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"strings"
)

func mapResponseFormatToProto(in *domain.ResponseFormat) *llmrunnerpb.ResponseFormat {
	if in == nil {
		return nil
	}
	out := &llmrunnerpb.ResponseFormat{
		Type: in.Type,
	}
	if in.Schema != nil {
		out.Schema = in.Schema
	}
	return out
}

func mapGenerationParamsToProto(in *domain.GenerationParams) *llmrunnerpb.GenerationParams {
	if in == nil {
		return nil
	}
	out := &llmrunnerpb.GenerationParams{
		ResponseFormat: mapResponseFormatToProto(in.ResponseFormat),
	}
	if in.Temperature != nil {
		out.Temperature = in.Temperature
	}
	if in.MaxTokens != nil {
		out.MaxTokens = in.MaxTokens
	}
	if in.TopK != nil {
		out.TopK = in.TopK
	}
	if in.TopP != nil {
		out.TopP = in.TopP
	}
	if len(in.Tools) > 0 {
		out.Tools = make([]*llmrunnerpb.Tool, 0, len(in.Tools))
		for _, t := range in.Tools {
			out.Tools = append(out.Tools, &llmrunnerpb.Tool{
				Name:           t.Name,
				Description:    t.Description,
				ParametersJson: t.ParametersJSON,
			})
		}
	}
	return out
}

type LLMRunnerService struct {
	client llmrunnerpb.LLMRunnerServiceClient
	conn   *grpc.ClientConn
	model  string
}

func NewLLMRunnerService(address, model string) (*LLMRunnerService, error) {
	if address == "" {
		address = "localhost:50052"
	}
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("подключение к llm-runner: %w", err)
	}
	return &LLMRunnerService{
		client: llmrunnerpb.NewLLMRunnerServiceClient(conn),
		conn:   conn,
		model:  model,
	}, nil
}

func (s *LLMRunnerService) Close() error {
	return s.conn.Close()
}

func (s *LLMRunnerService) CheckConnection(ctx context.Context) (bool, error) {
	resp, err := s.client.CheckConnection(ctx, &llmrunnerpb.Empty{})
	if err != nil {
		return false, fmt.Errorf("llm-runner CheckConnection: %w", err)
	}
	return resp.IsConnected, nil
}

func (s *LLMRunnerService) GetModels(ctx context.Context) ([]string, error) {
	resp, err := s.client.GetModels(ctx, &llmrunnerpb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("llm-runner GetModels: %w", err)
	}

	if resp == nil {
		return nil, nil
	}

	return resp.Models, nil
}

func (s *LLMRunnerService) GetGpuInfo(ctx context.Context) (*llmrunnerpb.GetGpuInfoResponse, error) {
	resp, err := s.client.GetGpuInfo(ctx, &llmrunnerpb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("llm-runner GetGpuInfo: %w", err)
	}

	return resp, nil
}

func (s *LLMRunnerService) GetServerInfo(ctx context.Context) (*llmrunnerpb.ServerInfo, error) {
	resp, err := s.client.GetServerInfo(ctx, &llmrunnerpb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("llm-runner GetServerInfo: %w", err)
	}

	return resp, nil
}

func (s *LLMRunnerService) resolveRunnerModel(model string) string {
	modelName := strings.TrimSpace(model)
	if modelName == "" || modelName == "default" {
		modelName = strings.TrimSpace(s.model)
	}

	if modelName == "default" {
		modelName = ""
	}

	return modelName
}

func (s *LLMRunnerService) Embed(ctx context.Context, model, text string) ([]float32, error) {
	resp, err := s.client.Embed(ctx, &llmrunnerpb.EmbedRequest{
		Model: s.resolveRunnerModel(model),
		Text:  text,
	})
	if err != nil {
		return nil, fmt.Errorf("llm-runner Embed: %w", err)
	}

	if resp == nil {
		return nil, nil
	}

	return resp.Values, nil
}

func (s *LLMRunnerService) EmbedBatch(ctx context.Context, model string, texts []string) ([][]float32, error) {
	resp, err := s.client.EmbedBatch(ctx, &llmrunnerpb.EmbedBatchRequest{
		Model: s.resolveRunnerModel(model),
		Texts: texts,
	})
	if err != nil {
		return nil, fmt.Errorf("llm-runner EmbedBatch: %w", err)
	}

	if resp == nil {
		return nil, nil
	}

	out := make([][]float32, 0, len(resp.Embeddings))
	for _, e := range resp.Embeddings {
		if e == nil {
			out = append(out, nil)
			continue
		}

		out = append(out, e.Values)
	}

	return out, nil
}

func (s *LLMRunnerService) SendMessage(
	ctx context.Context,
	sessionID int64,
	model string,
	messages []*domain.Message,
	stopSequences []string,
	timeoutSeconds int32,
	genParams *domain.GenerationParams,
) (chan string, error) {
	modelName := s.resolveRunnerModel(model)
	req := &llmrunnerpb.SendMessageRequest{
		SessionId:        sessionID,
		Messages:         domainMessagesToProto(messages),
		Model:            modelName,
		StopSequences:    stopSequences,
		GenerationParams: mapGenerationParamsToProto(genParams),
	}
	if timeoutSeconds > 0 {
		req.TimeoutSeconds = &timeoutSeconds
	}

	stream, err := s.client.SendMessage(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("llm-runner SendMessage: %w", err)
	}

	firstMsg, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("llm-runner stream recv: %w", err)
	}

	output := make(chan string, 100)

	go func() {
		defer close(output)
		current := firstMsg
		for {
			if current.Content != "" {
				select {
				case <-ctx.Done():
					return
				case output <- current.Content:
				}
			}
			if current.Done {
				return
			}

			msg, err := stream.Recv()
			if err != nil {
				return
			}
			current = msg
		}
	}()

	return output, nil
}

func domainMessagesToProto(messages []*domain.Message) []*llmrunnerpb.ChatMessage {
	out := make([]*llmrunnerpb.ChatMessage, 0, len(messages))
	for _, m := range messages {
		if m == nil {
			continue
		}

		cm := &llmrunnerpb.ChatMessage{
			Id:        int64(len(out) + 1),
			Content:   m.Content,
			Role:      string(m.Role),
			CreatedAt: m.CreatedAt.Unix(),
		}

		if strings.TrimSpace(m.ToolCallID) != "" {
			v := m.ToolCallID
			cm.ToolCallId = &v
		}

		if strings.TrimSpace(m.ToolName) != "" {
			v := m.ToolName
			cm.ToolName = &v
		}

		if strings.TrimSpace(m.ToolCallsJSON) != "" {
			v := m.ToolCallsJSON
			cm.ToolCallsJson = &v
		}

		out = append(out, cm)
	}

	return out
}
