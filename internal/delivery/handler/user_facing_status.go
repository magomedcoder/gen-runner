package handler

import (
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const GRPCErrorDomainGen = "gen.ai"

func StatusErrorWithReason(code codes.Code, reason, userMessage string) error {
	st := status.New(code, userMessage)
	stWith, err := st.WithDetails(&errdetails.ErrorInfo{
		Reason: reason,
		Domain: GRPCErrorDomainGen,
	})

	if err != nil {
		return st.Err()
	}

	return stWith.Err()
}
