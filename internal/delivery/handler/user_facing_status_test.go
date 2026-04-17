package handler

import (
	"testing"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStatusErrorWithReason_ErrorInfo(t *testing.T) {
	err := StatusErrorWithReason(codes.InvalidArgument, "CHAT_TEST_REASON", "user visible")
	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status")
	}

	if st.Code() != codes.InvalidArgument || st.Message() != "user visible" {
		t.Fatalf("status: code=%v msg=%q", st.Code(), st.Message())
	}

	details := st.Details()
	if len(details) != 1 {
		t.Fatalf("details len: %d", len(details))
	}

	info, ok := details[0].(*errdetails.ErrorInfo)
	if !ok || info.GetReason() != "CHAT_TEST_REASON" || info.GetDomain() != GRPCErrorDomainGen {
		t.Fatalf("ErrorInfo: %#v", details[0])
	}
}
