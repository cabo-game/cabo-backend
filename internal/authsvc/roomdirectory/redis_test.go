package roomdirectory_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/cabo/cabo-backend/internal/authsvc/roomdirectory"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestDirectory(t *testing.T) *roomdirectory.RedisRoomDirectory {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return roomdirectory.NewRedisRoomDirectory(client, time.Hour, testLogger())
}

func TestRedisRoomDirectory_RegisterThenLookup_ReturnsSameAddress(t *testing.T) {
	dir := newTestDirectory(t)
	ctx := context.Background()

	if err := dir.RegisterRoom(ctx, "room-1", "10.0.0.1:8081"); err != nil {
		t.Fatalf("RegisterRoom returned error: %v", err)
	}

	got, err := dir.LookupRoom(ctx, "room-1")
	if err != nil {
		t.Fatalf("LookupRoom returned error: %v", err)
	}
	if got != "10.0.0.1:8081" {
		t.Errorf("LookupRoom = %q, want %q", got, "10.0.0.1:8081")
	}
}

func TestRedisRoomDirectory_LookupRoom_ReturnsErrRoomNotFoundForUnknownID(t *testing.T) {
	dir := newTestDirectory(t)

	_, err := dir.LookupRoom(context.Background(), "does-not-exist")
	if !errors.Is(err, roomdirectory.ErrRoomNotFound) {
		t.Errorf("LookupRoom error = %v, want ErrRoomNotFound", err)
	}
}

func TestRedisRoomDirectory_RegisterRoom_SetsExpiry(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	dir := roomdirectory.NewRedisRoomDirectory(client, time.Hour, testLogger())

	if err := dir.RegisterRoom(context.Background(), "room-1", "10.0.0.1:8081"); err != nil {
		t.Fatalf("RegisterRoom returned error: %v", err)
	}

	ttl := mr.TTL("room-1")
	if ttl <= 0 {
		t.Errorf("TTL for room-1 = %v, want a positive TTL", ttl)
	}
}
