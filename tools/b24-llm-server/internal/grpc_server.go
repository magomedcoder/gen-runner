package internal

import (
	"context"

	"github.com/magomedcoder/gen/tools/b24-llm-server/api/pb/b24llmpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type b24GRPCServer struct {
	b24llmpb.UnimplementedB24LLMServer
	app *App
}

func grpcAnalyzeErr(err error) error {
	switch analyzeErrorLabel(err) {
	case "llm_error":
		return status.Error(codes.Internal, "llm_error")
	default:
		return status.Error(codes.InvalidArgument, analyzeErrorLabel(err))
	}
}

func (s *b24GRPCServer) Health(ctx context.Context, _ *b24llmpb.HealthRequest) (*b24llmpb.HealthResponse, error) {
	ok, err := s.app.llm.CheckConnection(ctx)
	if err != nil {
		return &b24llmpb.HealthResponse{
			Ok:          false,
			ErrorDetail: err.Error(),
		}, nil
	}

	return &b24llmpb.HealthResponse{Ok: ok}, nil
}

func (s *b24GRPCServer) Analyze(ctx context.Context, in *b24llmpb.AnalyzeRequest) (*b24llmpb.AnalyzeResponse, error) {
	req := pbToAnalyze(in)
	out, err := s.app.analyzeAndCache(ctx, req)
	if err != nil {
		return nil, grpcAnalyzeErr(err)
	}

	return &b24llmpb.AnalyzeResponse{Message: out}, nil
}

func (s *b24GRPCServer) AnalyzeStream(in *b24llmpb.AnalyzeRequest, stream grpc.ServerStreamingServer[b24llmpb.AnalyzeStreamChunk]) error {
	req := pbToAnalyze(in)
	ctx := stream.Context()
	return s.app.emitAnalyzeStream(ctx, req, func(ch *b24llmpb.AnalyzeStreamChunk) error {
		return stream.Send(ch)
	})
}

func (s *b24GRPCServer) SummarizeBatch(ctx context.Context, in *b24llmpb.SummarizeBatchRequest) (*b24llmpb.SummarizeBatchResponse, error) {
	internalReq := pbToSummarizeBatch(in)
	res, err := s.app.RunSummarizeBatch(ctx, &internalReq)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return summarizeBatchToPB(res), nil
}
