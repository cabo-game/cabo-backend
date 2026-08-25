package serverpicker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// keyPrefix must match internal/roomsvc/registry's keyPrefix — this reads
// the same roster that package writes.
const keyPrefix = "/roomsvc/servers/"

// registration mirrors internal/roomsvc/registry's registration: the JSON
// value written under each server's etcd key.
type registration struct {
	Address  string `json:"address"`
	Capacity int    `json:"capacity"`
}

// etcdReader is the small slice of etcd functionality EtcdServerPicker
// needs: list every value under a key prefix. EtcdServerPicker depends on
// this interface, never on *clientv3.Client directly, so tests can fake it
// without a real etcd server (same pattern as
// roomsvc/registry.etcdClient and authsvc/roomdirectory.RoomDirectory).
type etcdReader interface {
	// GetByPrefix returns the value of every key under prefix.
	GetByPrefix(ctx context.Context, prefix string) ([]string, error)
}

// EtcdServerPicker is a ServerPicker backed by etcd's roomsvc roster
// (spine AD-7).
type EtcdServerPicker struct {
	client etcdReader
	log    *slog.Logger
}

// NewEtcdServerPicker creates an EtcdServerPicker using the given etcdReader.
func NewEtcdServerPicker(client etcdReader, log *slog.Logger) *EtcdServerPicker {
	return &EtcdServerPicker{
		client: client,
		log:    log,
	}
}

// TODO: this always returns the first registration found, so one server
// absorbs every new room while the rest sit idle. capacity is decoded but
// never read. Replace with a real placement policy (spine AD-7 leaves this
// explicitly undecided) — e.g. round-robin across the roster, or track
// live room counts per server in authsvc by watching room-created (already
// reported via authclient) and room-closed (coming soon) events, without
// needing roomsvc's registration itself to change.
//
// PickServer returns the address of any one server currently registered in
// etcd's roomsvc roster. There is no placement policy yet (spine AD-7
// explicitly leaves this undecided) — this returns the first registration
// found.
func (p *EtcdServerPicker) PickServer(ctx context.Context) (string, error) {
	values, err := p.client.GetByPrefix(ctx, keyPrefix)
	if err != nil {
		return "", fmt.Errorf("serverpicker: failed to read roomsvc roster: %w", err)
	}

	if len(values) == 0 {
		p.log.Info("no live roomsvc servers found in roster")
		return "", ErrNoServersAvailable
	}

	var reg registration
	if err := json.Unmarshal([]byte(values[0]), &reg); err != nil {
		return "", fmt.Errorf("serverpicker: failed to decode registration: %w", err)
	}

	p.log.Info("picked server", "address", reg.Address, "candidates", len(values))
	return reg.Address, nil
}

// clientv3Adapter adapts a real *clientv3.Client to etcdReader.
type clientv3Adapter struct {
	client *clientv3.Client
}

// NewClientv3Adapter wraps client so it satisfies etcdReader.
func NewClientv3Adapter(client *clientv3.Client) etcdReader {
	return &clientv3Adapter{client: client}
}

func (a *clientv3Adapter) GetByPrefix(ctx context.Context, prefix string) ([]string, error) {
	resp, err := a.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	values := make([]string, len(resp.Kvs))
	for i, kv := range resp.Kvs {
		values[i] = string(kv.Value)
	}
	return values, nil
}
