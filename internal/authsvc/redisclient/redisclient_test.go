package redisclient_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/cabo/cabo-backend/internal/authsvc/redisclient"
)

func TestNew_ConnectsSuccessfully(t *testing.T) {
	mr := miniredis.RunT(t)

	client, err := redisclient.New(context.Background(), mr.Addr())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if client == nil {
		t.Fatal("New returned a nil client with no error")
	}
}

func TestNew_ReturnsErrorForUnreachableAddress(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	_, err := redisclient.New(ctx, "127.0.0.1:1")
	if err == nil {
		t.Error("expected an error connecting to an unreachable address, got nil")
	}
}
