// Command roomsvc is the stateful tier of the Cabo backend: it owns one
// game room's live state in memory per instance and speaks WebSocket
// directly to clients for the room's lifetime (spine AD-2, AD-5).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/cabo/cabo-backend/internal/roomsvc/authclient"
	"github.com/cabo/cabo-backend/internal/roomsvc/game"
	"github.com/cabo/cabo-backend/internal/roomsvc/handoff"
	"github.com/cabo/cabo-backend/internal/roomsvc/registry"
	"github.com/cabo/cabo-backend/internal/roomsvc/ws"
)

const (
	listenAddr             = ":8081"
	shutdownTimeout        = 10 * time.Second
	defaultLeaseTTL        = 10 * time.Second
	etcdDialTimeout        = 5 * time.Second
	registrationStartupTTL = 10 * time.Second
	defaultPingInterval    = 30 * time.Second
	defaultPingTimeout     = 10 * time.Second
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	registrar, err := newRegistrar(log)
	if err != nil {
		log.Error("failed to configure registry", "error", err)
		os.Exit(1)
	}

	startCtx, cancelStart := context.WithTimeout(context.Background(), registrationStartupTTL)
	defer cancelStart()
	if err := registrar.Start(startCtx); err != nil {
		log.Error("failed to register with etcd", "error", err)
		os.Exit(1)
	}

	authClient, err := newAuthClient(log)
	if err != nil {
		log.Error("failed to configure authclient", "error", err)
		os.Exit(1)
	}

	roomManager := game.NewRoomManager(log, authClient)

	pingConfig, err := newPingConfig()
	if err != nil {
		log.Error("failed to configure ping keepalive", "error", err)
		os.Exit(1)
	}

	handler := ws.NewHandler(log, pingConfig, onConnect(log, roomManager))

	mux := http.NewServeMux()
	mux.Handle("/ws", handler)

	server := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		log.Info("roomsvc listening", "addr", listenAddr)
		serverErr <- server.ListenAndServe()
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
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Error("graceful shutdown failed", "error", err)
			os.Exit(1)
		}
		if err := registrar.Stop(shutdownCtx); err != nil {
			log.Error("failed to revoke etcd registration", "error", err)
		}
		log.Info("roomsvc stopped")
	}
}

// newRegistrar builds a registry.Registrar from environment configuration.
// See the "roomsvc Registration & Heartbeat LLD" for why these values are
// config-injected rather than auto-detected (ROOMSVC_ADVERTISE_ADDR in
// particular — a process cannot reliably determine its own
// externally-routable address behind NAT/containers/load balancers).
func newRegistrar(log *slog.Logger) (*registry.Registrar, error) {
	endpoints := os.Getenv("ETCD_ENDPOINTS")
	if endpoints == "" {
		return nil, errors.New("ETCD_ENDPOINTS is required (comma-separated etcd endpoints)")
	}

	advertiseAddr := os.Getenv("ROOMSVC_ADVERTISE_ADDR")
	if advertiseAddr == "" {
		return nil, errors.New("ROOMSVC_ADVERTISE_ADDR is required (address other services use to reach this instance)")
	}

	leaseTTL := defaultLeaseTTL
	if v := os.Getenv("ROOMSVC_LEASE_TTL"); v != "" {
		parsed, err := time.ParseDuration(v)
		if err != nil {
			return nil, errors.New("ROOMSVC_LEASE_TTL must be a valid duration, e.g. \"10s\"")
		}
		leaseTTL = parsed
	}

	capacity := 0
	if v := os.Getenv("ROOMSVC_CAPACITY"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return nil, errors.New("ROOMSVC_CAPACITY must be an integer")
		}
		capacity = parsed
	}

	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   strings.Split(endpoints, ","),
		DialTimeout: etcdDialTimeout,
	})
	if err != nil {
		return nil, err
	}

	return registry.NewRegistrar(registry.NewClientv3Adapter(etcdClient), advertiseAddr, capacity, leaseTTL, log), nil
}

// newAuthClient builds an authclient.Client from environment
// configuration. See internal/roomsvc/authclient's package doc for the
// contract it calls (authsvc's POST /rooms/register) and why
// ROOMSVC_ADVERTISE_ADDR is read again here rather than threaded in from
// newRegistrar: each constructor validates its own inputs independently,
// consistent with the rest of this file.
func newAuthClient(log *slog.Logger) (authclient.Client, error) {
	advertiseAddr := os.Getenv("ROOMSVC_ADVERTISE_ADDR")
	if advertiseAddr == "" {
		return nil, errors.New("ROOMSVC_ADVERTISE_ADDR is required (address other services use to reach this instance)")
	}

	authsvcAddr := os.Getenv("AUTHSVC_INTERNAL_ADDR")
	if authsvcAddr == "" {
		return nil, errors.New("AUTHSVC_INTERNAL_ADDR is required (base URL of authsvc's internal API, e.g. \"http://10.0.4.5:8080\")")
	}

	return authclient.NewHTTPClient(authsvcAddr, advertiseAddr, log), nil
}

// newPingConfig builds a ws.PingConfig from environment configuration (or
// the defaults above if unset). This is the ping/pong keepalive every
// accepted connection's ReadLoop runs to detect a peer that has gone
// silent without a normal WebSocket close — e.g. a network partition,
// which a plain read error can't detect since the underlying TCP
// connection just blocks forever instead of erroring.
func newPingConfig() (ws.PingConfig, error) {
	interval := defaultPingInterval
	if v := os.Getenv("ROOMSVC_PING_INTERVAL"); v != "" {
		parsed, err := time.ParseDuration(v)
		if err != nil {
			return ws.PingConfig{}, errors.New("ROOMSVC_PING_INTERVAL must be a valid duration, e.g. \"30s\"")
		}
		interval = parsed
	}

	timeout := defaultPingTimeout
	if v := os.Getenv("ROOMSVC_PING_TIMEOUT"); v != "" {
		parsed, err := time.ParseDuration(v)
		if err != nil {
			return ws.PingConfig{}, errors.New("ROOMSVC_PING_TIMEOUT must be a valid duration, e.g. \"10s\"")
		}
		timeout = parsed
	}

	if timeout >= interval {
		return ws.PingConfig{}, errors.New("ROOMSVC_PING_TIMEOUT must be less than ROOMSVC_PING_INTERVAL")
	}

	return ws.PingConfig{Interval: interval, Timeout: timeout}, nil
}

// onConnect runs the create/join handshake (spine AD-9; see
// internal/roomsvc/handoff and internal/roomsvc/ARCHITECTURE.md for the
// message contract) on every new connection, then hands off to the
// normal read loop for whatever comes next. Real auth validation is still
// a separate, undecided design effort (see CLAUDE.md's Current State) —
// this only decides which room a connection belongs to, not who the
// player is.
func onConnect(log *slog.Logger, roomManager *game.RoomManager) func(*ws.Connection) {
	return func(conn *ws.Connection) {
		ctx := context.Background()

		player, room, err := handoff.Handle(ctx, conn, roomManager, log)
		if err != nil {
			log.Info("handoff failed", "remote_addr", conn.RemoteAddr(), "error", err)
			return
		}

		log.Info("player joined room", "remote_addr", conn.RemoteAddr(), "player_id", player.ID, "room_id", room.ID)

		conn.ReadLoop(ctx, func(data []byte) {
			log.Info("message received", "player_id", player.ID, "room_id", room.ID, "bytes", len(data))
		}, func() {
			log.Info("connection closed, removing player from room", "player_id", player.ID, "room_id", room.ID)
			roomManager.RemovePlayer(room, player.ID)
		})
	}
}
