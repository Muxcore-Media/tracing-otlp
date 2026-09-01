package internal

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestAuthorizeTracingRPC(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-caller-id", "mod-a"))
	if err := authorizeTracingRPC(ctx, "secret"); err != nil {
		t.Fatalf("mesh identity: %v", err)
	}

	tokenCtx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer secret"))
	if err := authorizeTracingRPC(tokenCtx, "secret"); err != nil {
		t.Fatalf("bearer token: %v", err)
	}

	if err := authorizeTracingRPC(context.Background(), "secret"); err == nil {
		t.Fatal("expected unauthenticated without metadata")
	}
}
