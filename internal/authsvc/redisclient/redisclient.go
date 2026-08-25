// Package redisclient connects to Redis. It has no knowledge of what the
// connection is used for — see roomdirectory for the room-directory logic
// built on top of it.
package redisclient

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// New connects to the Redis instance at addr and verifies the connection
// with a ping before returning.
func New(ctx context.Context, addr string) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redisclient: failed to connect to %s: %w", addr, err)
	}

	return client, nil
}
