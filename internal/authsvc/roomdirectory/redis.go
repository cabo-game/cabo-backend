package roomdirectory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisRoomDirectory is a RoomDirectory backed by Redis (spine AD-3): a
// room-id -> server-address lookup.
type RedisRoomDirectory struct {
	client *redis.Client
	ttl    time.Duration
	log    *slog.Logger
}

// NewRedisRoomDirectory creates a RedisRoomDirectory. ttl is how long a
// registered room entry survives before Redis expires it automatically —
// a safety net against orphaned entries, since nothing refreshes or
// removes them yet (room cleanup is not decided; see GameRoom/RoomManager
// notes in roomsvc).
func NewRedisRoomDirectory(client *redis.Client, ttl time.Duration, log *slog.Logger) *RedisRoomDirectory {
	return &RedisRoomDirectory{
		client: client,
		ttl:    ttl,
		log:    log,
	}
}

// RegisterRoom records that roomID's server is serverAddress, expiring the
// entry after d.ttl.
func (d *RedisRoomDirectory) RegisterRoom(ctx context.Context, roomID, serverAddress string) error {
	if err := d.client.Set(ctx, roomID, serverAddress, d.ttl).Err(); err != nil {
		return fmt.Errorf("roomdirectory: failed to register room %q: %w", roomID, err)
	}

	d.log.Info("room registered in directory", "room_id", roomID, "server_address", serverAddress)
	return nil
}

// LookupRoom returns the server address registered for roomID, or
// ErrRoomNotFound if none is registered (or the entry expired).
func (d *RedisRoomDirectory) LookupRoom(ctx context.Context, roomID string) (string, error) {
	serverAddress, err := d.client.Get(ctx, roomID).Result()
	if errors.Is(err, redis.Nil) {
		d.log.Info("room directory lookup missed", "room_id", roomID)
		return "", ErrRoomNotFound
	}
	if err != nil {
		return "", fmt.Errorf("roomdirectory: failed to look up room %q: %w", roomID, err)
	}

	return serverAddress, nil
}
