// Package registry registers this roomsvc instance's liveness in etcd
// (spine AD-7): a lease-backed key under /roomsvc/servers/<address> that
// etcd deletes automatically if the process stops renewing it. authsvc
// reads this roster to pick a server for a new room; it never
// health-checks roomsvc directly. See the "roomsvc Registration &
// Heartbeat LLD" for the full design and TTL/keepalive reasoning.
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"time"
)

const keyPrefix = "/roomsvc/servers/"

const (
	reRegisterBaseDelay = time.Second
	reRegisterMaxDelay  = 30 * time.Second
)

// registration is the JSON value written under this instance's etcd key.
type registration struct {
	Address  string `json:"address"`
	Capacity int    `json:"capacity"`
}

// Registrar keeps exactly one etcd lease alive for the life of this
// roomsvc process and re-registers under a new lease if the keepalive
// stream ever dies. It has no knowledge of GameRoom or Player.
type Registrar struct {
	client        etcdClient
	advertiseAddr string
	capacity      int
	leaseTTL      time.Duration
	log           *slog.Logger

	mu      sync.Mutex
	leaseID leaseID
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewRegistrar creates a Registrar. advertiseAddr is what other services
// should dial to reach this instance (never the bare bind address — see
// the LLD's "advertise address" section); capacity is an opaque value
// written alongside it for a future placement policy to consume.
func NewRegistrar(client etcdClient, advertiseAddr string, capacity int, leaseTTL time.Duration, log *slog.Logger) *Registrar {
	return &Registrar{
		client:        client,
		advertiseAddr: advertiseAddr,
		capacity:      capacity,
		leaseTTL:      leaseTTL,
		log:           log,
	}
}

// Start grants a lease, registers this instance's address under it, and
// begins renewing it in the background for as long as the process runs
// (until Stop is called). It returns once the first registration
// succeeds; it does not wait for the background keepalive loop.
func (r *Registrar) Start(ctx context.Context) error {
	id, err := r.registerUnderNewLease(ctx)
	if err != nil {
		return err
	}

	loopCtx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.leaseID = id
	r.cancel = cancel
	r.done = make(chan struct{})
	r.mu.Unlock()

	go r.keepAliveLoop(loopCtx, id)

	return nil
}

// Stop revokes the current lease, deleting this instance's registration
// immediately instead of waiting for the TTL to lapse, and stops the
// background renewal loop.
func (r *Registrar) Stop(ctx context.Context) error {
	r.mu.Lock()
	id := r.leaseID
	cancel := r.cancel
	done := r.done
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}

	if err := r.client.Revoke(ctx, id); err != nil {
		return fmt.Errorf("registry: failed to revoke lease %d: %w", id, err)
	}

	r.log.Info("registration revoked", "advertise_addr", r.advertiseAddr, "lease_id", id)
	return nil
}

// registerUnderNewLease grants a fresh lease and writes this instance's
// registration under it, returning the new lease ID.
func (r *Registrar) registerUnderNewLease(ctx context.Context) (leaseID, error) {
	id, err := r.client.Grant(ctx, int64(r.leaseTTL.Seconds()))
	if err != nil {
		return 0, fmt.Errorf("registry: failed to grant lease: %w", err)
	}

	val, err := json.Marshal(registration{Address: r.advertiseAddr, Capacity: r.capacity})
	if err != nil {
		return 0, fmt.Errorf("registry: failed to encode registration: %w", err)
	}

	key := keyPrefix + r.advertiseAddr
	if err := r.client.Put(ctx, key, string(val), id); err != nil {
		return 0, fmt.Errorf("registry: failed to register %q: %w", key, err)
	}

	r.log.Info("registered with etcd", "advertise_addr", r.advertiseAddr, "lease_id", id)
	return id, nil
}

// keepAliveLoop renews leaseID until ctx is cancelled (Stop was called).
// If the keepalive stream ever dies for any other reason, it re-registers
// under a new lease with exponential backoff instead of leaving this
// instance permanently unregistered.
func (r *Registrar) keepAliveLoop(ctx context.Context, id leaseID) {
	defer close(r.done)

	for {
		ch, err := r.client.KeepAlive(ctx, id)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			r.log.Error("keepalive failed to start", "lease_id", id, "error", err)
			id = r.reRegisterWithBackoff(ctx)
			if id == 0 {
				return // ctx was cancelled while retrying
			}
			continue
		}

		r.drainUntilClosed(ctx, ch)

		if ctx.Err() != nil {
			return
		}

		r.log.Info("keepalive stream closed, re-registering", "lease_id", id)
		id = r.reRegisterWithBackoff(ctx)
		if id == 0 {
			return // ctx was cancelled while retrying
		}

		r.mu.Lock()
		r.leaseID = id
		r.mu.Unlock()
	}
}

// drainUntilClosed reads keepalive signals until the channel closes or
// ctx is cancelled.
func (r *Registrar) drainUntilClosed(ctx context.Context, ch <-chan struct{}) {
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// reRegisterWithBackoff retries registerUnderNewLease with jittered
// exponential backoff until it succeeds or ctx is cancelled, in which case
// it returns 0.
func (r *Registrar) reRegisterWithBackoff(ctx context.Context) leaseID {
	delay := reRegisterBaseDelay

	for {
		id, err := r.registerUnderNewLease(ctx)
		if err == nil {
			return id
		}

		r.log.Error("re-registration failed, retrying", "error", err, "retry_in", delay)

		jittered := delay/2 + time.Duration(rand.Int63n(int64(delay)))
		select {
		case <-time.After(jittered):
		case <-ctx.Done():
			return 0
		}

		delay *= 2
		if delay > reRegisterMaxDelay {
			delay = reRegisterMaxDelay
		}
	}
}
