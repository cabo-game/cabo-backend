package registry

import (
	"context"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// leaseID identifies an etcd lease. It mirrors clientv3.LeaseID so
// Registrar's own interface (etcdClient) never has to expose etcd's
// opaque Op/OpOption types — see clientv3Adapter for the translation.
type leaseID int64

// etcdClient is the small slice of etcd functionality Registrar needs:
// grant a lease, put a value under it, keep it alive, and revoke it. It
// intentionally does not mirror clientv3.Lease/clientv3.KV's full
// signatures (e.g. Put's variadic OpOption) because those types have no
// public way to read a lease ID back out for testing — see
// registrar_test.go's fakeEtcdClient. Registrar depends on this
// interface, never on *clientv3.Client directly, so tests can fake it
// without a real etcd server (same pattern as
// authsvc/roomdirectory.RoomDirectory).
type etcdClient interface {
	// Grant creates a new lease with the given TTL, in seconds.
	Grant(ctx context.Context, ttlSeconds int64) (leaseID, error)

	// Put writes val under key, attached to the given lease so it expires
	// when the lease does.
	Put(ctx context.Context, key, val string, lease leaseID) error

	// KeepAlive renews the given lease until the returned channel closes
	// (etcd unreachable, lease lost, or ctx is done) or the caller stops
	// reading from it.
	KeepAlive(ctx context.Context, lease leaseID) (<-chan struct{}, error)

	// Revoke revokes the given lease immediately, deleting whatever was
	// registered under it without waiting for the TTL to lapse.
	Revoke(ctx context.Context, lease leaseID) error
}

// clientv3Adapter adapts a real *clientv3.Client to etcdClient.
type clientv3Adapter struct {
	client *clientv3.Client
}

// NewClientv3Adapter wraps client so it satisfies etcdClient.
func NewClientv3Adapter(client *clientv3.Client) etcdClient {
	return &clientv3Adapter{client: client}
}

func (a *clientv3Adapter) Grant(ctx context.Context, ttlSeconds int64) (leaseID, error) {
	resp, err := a.client.Grant(ctx, ttlSeconds)
	if err != nil {
		return 0, err
	}
	return leaseID(resp.ID), nil
}

func (a *clientv3Adapter) Put(ctx context.Context, key, val string, lease leaseID) error {
	_, err := a.client.Put(ctx, key, val, clientv3.WithLease(clientv3.LeaseID(lease)))
	return err
}

func (a *clientv3Adapter) KeepAlive(ctx context.Context, lease leaseID) (<-chan struct{}, error) {
	respCh, err := a.client.KeepAlive(ctx, clientv3.LeaseID(lease))
	if err != nil {
		return nil, err
	}

	// Registrar only needs to know "still alive" vs. "channel closed" — it
	// never inspects individual keepalive responses — so translate to a
	// signal-only channel and drop each response after forwarding that.
	signal := make(chan struct{})
	go func() {
		defer close(signal)
		for range respCh {
			select {
			case signal <- struct{}{}:
			default:
			}
		}
	}()

	return signal, nil
}

func (a *clientv3Adapter) Revoke(ctx context.Context, lease leaseID) error {
	_, err := a.client.Revoke(ctx, clientv3.LeaseID(lease))
	return err
}
