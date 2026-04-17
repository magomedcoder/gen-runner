package handler

import (
	"errors"
	"testing"

	"github.com/magomedcoder/gen/internal/domain"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStatusForChatAssistantOpSentinel(t *testing.T) {
	tools := statusForChatAssistantOpSentinel(domain.ErrRegenerateToolsNotSupported)
	st, _ := status.FromError(tools)
	if st.Code() != codes.FailedPrecondition {
		t.Fatalf("code: %v", st.Code())
	}

	d0 := st.Details()[0].(*errdetails.ErrorInfo)
	if d0.GetReason() != "CHAT_REGENERATE_TOOLS_NOT_SUPPORTED" {
		t.Fatalf("reason: %q", d0.GetReason())
	}

	auth := statusForChatAssistantOpSentinel(domain.ErrUnauthorized)
	st2, _ := status.FromError(auth)
	if st2.Code() != codes.PermissionDenied {
		t.Fatalf("code: %v", st2.Code())
	}

	d1 := st2.Details()[0].(*errdetails.ErrorInfo)
	if d1.GetReason() != "CHAT_UNAUTHORIZED" {
		t.Fatalf("reason: %q", d1.GetReason())
	}

	if statusForChatAssistantOpSentinel(errors.New("other")) != nil {
		t.Fatal("expected nil for unknown error")
	}
}

func TestStatusForCreateSessionError(t *testing.T) {
	nr := statusForCreateSessionError(domain.ErrNoRunners)
	st, _ := status.FromError(nr)
	if st.Details()[0].(*errdetails.ErrorInfo).GetReason() != "CHAT_NO_RUNNERS" {
		t.Fatal("expected CHAT_NO_RUNNERS")
	}

	cfg := statusForCreateSessionError(domain.ErrRunnerChatModelNotConfigured)
	st2, _ := status.FromError(cfg)
	if st2.Details()[0].(*errdetails.ErrorInfo).GetReason() != "CHAT_RUNNER_MODEL_NOT_CONFIGURED" {
		t.Fatal("expected CHAT_RUNNER_MODEL_NOT_CONFIGURED")
	}

	if statusForCreateSessionError(errors.New("x")) != nil {
		t.Fatal("expected nil")
	}
}
