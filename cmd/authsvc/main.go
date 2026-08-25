// Command authsvc is the stateless tier of the Cabo backend. Scoped, for
// now, to room-allocation only (spine AD-9): picking a live roomsvc server
// and maintaining the room-id -> server-address directory (AD-3). No
// login/auth/user-identity work yet — that is a separate, undecided
// design effort (spine AD-6).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/cabo/cabo-backend/internal/authsvc/api"
	"github.com/cabo/cabo-backend/internal/authsvc/redisclient"
	"github.com/cabo/cabo-backend/internal/authsvc/roomdirectory"
	"github.com/cabo/cabo-backend/internal/authsvc/serverpicker"
)

const (
	listenAddr       = ":8080"
	shutdownTimeout  = 10 * time.Second
	etcdDialTimeout  = 5 * time.Second
	redisDialTimeout = 5 * time.Second
	roomEntryTTL     = 24 * time.Hour
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	directory, err := newRoomDirectory(log)
	if err != nil {
		log.Error("failed to configure room directory", "error", err)
		os.Exit(1)
	}

	picker, err := newServerPicker(log)
	if err != nil {
		log.Error("failed to configure server picker", "error", err)
		os.Exit(1)
	}

	server := api.NewServer(directory, picker, log)

	httpServer := &http.Server{
		Addr:    listenAddr,
		Handler: server.Routes(),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		log.Info("authsvc listening", "addr", listenAddr)
		serverErr <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		log.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Error("graceful shutdown failed", "error", err)
			os.Exit(1)
		}
		log.Info("authsvc stopped")
	}
}

// newRoomDirectory builds a Redis-backed RoomDirectory from environment
// configuration.
func newRoomDirectory(log *slog.Logger) (roomdirectory.RoomDirectory, error) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		return nil, errors.New("REDIS_ADDR is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), redisDialTimeout)
	defer cancel()

	client, err := redisclient.New(ctx, addr)
	if err != nil {
		return nil, err
	}

	return roomdirectory.NewRedisRoomDirectory(client, roomEntryTTL, log), nil
}

// newServerPicker builds an etcd-backed ServerPicker from environment
// configuration.
func newServerPicker(log *slog.Logger) (serverpicker.ServerPicker, error) {
	endpoints := os.Getenv("ETCD_ENDPOINTS")
	if endpoints == "" {
		return nil, errors.New("ETCD_ENDPOINTS is required (comma-separated etcd endpoints)")
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   strings.Split(endpoints, ","),
		DialTimeout: etcdDialTimeout,
	})
	if err != nil {
		return nil, err
	}

	return serverpicker.NewEtcdServerPicker(serverpicker.NewClientv3Adapter(client), log), nil
}
