package registry

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeEtcdClient is a hand-written stand-in for etcdClient. It never talks
// to a real etcd server — see the registry LLD for why: the official
// integration test package pulls in etcd's server internals as a
// dependency and was fragile to resolve at the pinned version, so
// Registrar depends on the small etcdClient interface instead, faked
// directly here (same pattern as authsvc/roomdirectory.RoomDirectory).
type fakeEtcdClient struct {
	mu sync.Mutex

	nextLeaseID  leaseID
	grantErr     error
	putErr       error
	keepAliveErr error
	revokeErr    error

	grants  []int64 // ttl seconds passed to each Grant call
	puts    []fakePut
	revokes []leaseID
	kaChans map[leaseID]chan struct{} // closed to simulate a dead keepalive stream
}

type fakePut struct {
	key     string
	val     string
	leaseID leaseID
}

func newFakeEtcdClient() *fakeEtcdClient {
	return &fakeEtcdClient{
		nextLeaseID: 1,
		kaChans:     make(map[leaseID]chan struct{}),
	}
}

func (f *fakeEtcdClient) Grant(ctx context.Context, ttlSeconds int64) (leaseID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.grantErr != nil {
		return 0, f.grantErr
	}

	id := f.nextLeaseID
	f.nextLeaseID++
	f.grants = append(f.grants, ttlSeconds)

	return id, nil
}

func (f *fakeEtcdClient) Put(ctx context.Context, key, val string, lease leaseID) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.putErr != nil {
		return f.putErr
	}

	f.puts = append(f.puts, fakePut{key: key, val: val, leaseID: lease})
	return nil
}

func (f *fakeEtcdClient) KeepAlive(ctx context.Context, lease leaseID) (<-chan struct{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.keepAliveErr != nil {
		return nil, f.keepAliveErr
	}

	ch := make(chan struct{})
	f.kaChans[lease] = ch

	return ch, nil
}

func (f *fakeEtcdClient) Revoke(ctx context.Context, lease leaseID) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.revokeErr != nil {
		return f.revokeErr
	}

	f.revokes = append(f.revokes, lease)
	return nil
}

// closeKeepAlive simulates the keepalive stream dying (etcd unreachable,
// lease lost) the way the real client does: by closing the channel.
func (f *fakeEtcdClient) closeKeepAlive(lease leaseID) {
	f.mu.Lock()
	ch, ok := f.kaChans[lease]
	delete(f.kaChans, lease)
	f.mu.Unlock()
	if ok {
		close(ch)
	}
}

func (f *fakeEtcdClient) putCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.puts)
}

// hasKeepAlive reports whether KeepAlive has been called for lease and
// its channel is still open — i.e. the Registrar's background loop has
// reached the point of watching this lease.
func (f *fakeEtcdClient) hasKeepAlive(lease leaseID) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.kaChans[lease]
	return ok
}

func (f *fakeEtcdClient) lastPut() fakePut {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.puts[len(f.puts)-1]
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("condition not met within %v", timeout)
}

func TestRegistrar_Start_GrantsLeaseAndRegistersAddress(t *testing.T) {
	fake := newFakeEtcdClient()
	r := NewRegistrar(fake, "10.0.4.12:8081", 40, 10*time.Second, testLogger())

	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer r.Stop(context.Background())

	if got, want := len(fake.grants), 1; got != want {
		t.Fatalf("Grant called %d times, want %d", got, want)
	}
	if got, want := fake.grants[0], int64(10); got != want {
		t.Errorf("Grant ttl = %d, want %d", got, want)
	}

	put := fake.lastPut()
	if put.key != "/roomsvc/servers/10.0.4.12:8081" {
		t.Errorf("Put key = %q, want %q", put.key, "/roomsvc/servers/10.0.4.12:8081")
	}
	if put.val == "" {
		t.Error("Put val is empty, want a JSON-encoded registration value")
	}
	if put.leaseID != 1 {
		t.Errorf("Put leaseID = %d, want the granted lease id 1", put.leaseID)
	}
}

func TestRegistrar_Start_RegistersValueAsJSONWithAddressAndCapacity(t *testing.T) {
	fake := newFakeEtcdClient()
	r := NewRegistrar(fake, "10.0.4.12:8081", 40, 10*time.Second, testLogger())

	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer r.Stop(context.Background())

	put := fake.lastPut()
	if !strings.Contains(put.val, `"address":"10.0.4.12:8081"`) || !strings.Contains(put.val, `"capacity":40`) {
		t.Errorf("Put val = %q, want it to contain address and capacity fields", put.val)
	}
}

func TestRegistrar_Start_ReturnsErrorWhenGrantFails(t *testing.T) {
	fake := newFakeEtcdClient()
	fake.grantErr = errors.New("etcd unreachable")
	r := NewRegistrar(fake, "10.0.4.12:8081", 40, 10*time.Second, testLogger())

	if err := r.Start(context.Background()); err == nil {
		t.Fatal("expected an error when Grant fails, got nil")
	}
}

func TestRegistrar_Start_ReturnsErrorWhenPutFails(t *testing.T) {
	fake := newFakeEtcdClient()
	fake.putErr = errors.New("etcd unreachable")
	r := NewRegistrar(fake, "10.0.4.12:8081", 40, 10*time.Second, testLogger())

	if err := r.Start(context.Background()); err == nil {
		t.Fatal("expected an error when Put fails, got nil")
	}
}

func TestRegistrar_Stop_RevokesTheLease(t *testing.T) {
	fake := newFakeEtcdClient()
	r := NewRegistrar(fake, "10.0.4.12:8081", 40, 10*time.Second, testLogger())

	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if err := r.Stop(context.Background()); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}

	if got, want := len(fake.revokes), 1; got != want {
		t.Fatalf("Revoke called %d times, want %d", got, want)
	}
	if fake.revokes[0] != 1 {
		t.Errorf("Revoke leaseID = %d, want 1", fake.revokes[0])
	}
}

func TestRegistrar_KeepAliveChannelClosing_TriggersReRegistrationUnderNewLease(t *testing.T) {
	fake := newFakeEtcdClient()
	r := NewRegistrar(fake, "10.0.4.12:8081", 40, 10*time.Second, testLogger())

	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer r.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return fake.hasKeepAlive(1) })

	// Simulate losing the lease: the keepalive stream dies.
	fake.closeKeepAlive(1)

	// Registrar should notice and re-register under a new lease.
	waitFor(t, 2*time.Second, func() bool { return fake.putCount() == 2 })

	if got, want := len(fake.grants), 2; got != want {
		t.Fatalf("Grant called %d times after lease loss, want %d", got, want)
	}
	if got := fake.lastPut().leaseID; got != 2 {
		t.Errorf("re-registration used leaseID %d, want the new lease id 2", got)
	}
}

func TestRegistrar_KeepAliveOpen_DoesNotReRegister(t *testing.T) {
	fake := newFakeEtcdClient()
	r := NewRegistrar(fake, "10.0.4.12:8081", 40, 10*time.Second, testLogger())

	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer r.Stop(context.Background())

	waitFor(t, time.Second, func() bool { return fake.hasKeepAlive(1) })

	// Give the Registrar's background goroutine a moment to (not) act,
	// while the keepalive channel stays open (the healthy case).
	time.Sleep(50 * time.Millisecond)

	if got, want := fake.putCount(), 1; got != want {
		t.Errorf("Put called %d times while keepalive is healthy, want %d (no re-registration)", got, want)
	}
}
