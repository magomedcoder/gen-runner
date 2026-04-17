package handler

import (
	"context"
	"github.com/magomedcoder/gen/api/pb/commonpb"
	"github.com/magomedcoder/gen/api/pb/editorpb"
	"github.com/magomedcoder/gen/internal/rpcmeta"
	"github.com/magomedcoder/gen/internal/usecase"
	"github.com/magomedcoder/gen/pkg/logger"
	"google.golang.org/grpc/codes"
)

type EditorHandler struct {
	editorpb.UnimplementedEditorServiceServer
	editorUseCase *usecase.EditorUseCase
	authUseCase   *usecase.AuthUseCase
}

func (e *EditorHandler) SaveHistory(ctx context.Context, req *editorpb.SaveHistoryRequest) (*commonpb.Empty, error) {
	user, err := GetUserFromContext(ctx, e.authUseCase)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return &commonpb.Empty{}, nil
	}
	if err := e.editorUseCase.SaveHistory(ctx, user.Id, req.GetRunner(), req.GetText()); err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}
	return &commonpb.Empty{}, nil
}

func NewEditorHandler(editorUseCase *usecase.EditorUseCase, authUseCase *usecase.AuthUseCase) *EditorHandler {
	return &EditorHandler{
		editorUseCase: editorUseCase,
		authUseCase:   authUseCase,
	}
}

func (e *EditorHandler) Transform(ctx context.Context, req *editorpb.TransformRequest) (*editorpb.TransformResponse, error) {
	user, err := GetUserFromContext(ctx, e.authUseCase)
	if err != nil {
		return nil, err
	}

	if req == nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "EDITOR_EMPTY_REQUEST", "пустой запрос")
	}

	if req.Text == "" {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "EDITOR_TEXT_EMPTY", "текст не предоставлен")
	}

	ctx = rpcmeta.EnsureTraceInContext(ctx)
	logger.D("EditorHandler: transform type=%v trace_id=%s", req.GetType(), rpcmeta.TraceIDFromContext(ctx))

	out, err := e.editorUseCase.Transform(
		ctx,
		user.Id,
		req.GetText(),
		req.GetType(),
		req.GetPreserveMarkdown(),
	)
	if err != nil {
		if mapped := statusForModelResolutionError(err); mapped != nil {
			return nil, mapped
		}

		return nil, ToStatusError(codes.Internal, err)
	}

	return &editorpb.TransformResponse{
		Text: out,
	}, nil
}
