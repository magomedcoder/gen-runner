package rpcmeta

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestEnsureTraceInContext_generatesWhenMissing(t *testing.T) {
	ctx := EnsureTraceInContext(context.Background())
	id := TraceIDFromContext(ctx)
	if id == "" {
		t.Fatal("ожидался сгенерированный trace id")
	}

	out := OutgoingContext(ctx)
	md, _ := metadata.FromOutgoingContext(out)
	if len(md.Get(MDTraceID)) != 1 || md.Get(MDTraceID)[0] != id {
		t.Fatalf("metadata: %+v", md)
	}
}

func TestIncomingTraceID(t *testing.T) {
	md := metadata.Pairs(MDTraceID, "  abc  ")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	ctx = EnsureTraceInContext(ctx)
	if TraceIDFromContext(ctx) != "abc" {
		t.Fatalf("got %q", TraceIDFromContext(ctx))
	}
}
