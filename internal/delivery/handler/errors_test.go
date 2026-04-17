package handler

import (
	"errors"
	"testing"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestToStatusError_ErrorInfo(t *testing.T) {
	err := ToStatusError(codes.Internal, errors.New("secret detail"))
	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status")
	}

	if st.Message() != "внутренняя ошибка сервера" {
		t.Fatalf("message: %q", st.Message())
	}

	info := st.Details()[0].(*errdetails.ErrorInfo)
	if info.GetReason() != "GEN_INTERNAL_ERROR" || info.GetDomain() != GRPCErrorDomainGen {
		t.Fatalf("ErrorInfo: reason=%q domain=%q", info.GetReason(), info.GetDomain())
	}
}

func TestGenericReasonForCode(t *testing.T) {
	if genericReasonForCode(codes.NotFound) != "GEN_NOT_FOUND" {
		t.Fatal()
	}

	if genericReasonForCode(codes.Unauthenticated) != "GEN_UNAUTHENTICATED" {
		t.Fatal()
	}
}
