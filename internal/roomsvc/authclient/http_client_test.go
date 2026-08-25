package authclient

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// recordingServer is a fake authsvc: it records every request POSTed to
// /rooms/register and lets a test control the response, matching
// internal/authsvc/api.handleRegisterRoom's real contract (single-object
// body, 204 No Content on success).
type recordingServer struct {
	*httptest.Server

	mu       sync.Mutex
	received []roomNotification
	failNext int // if > 0, fail this many requests before succeeding
}

func newRecordingServer(t *testing.T) *recordingServer {
	t.Helper()
	rs := &recordingServer{}
	rs.Server = httptest.NewServer(http.HandlerFunc(rs.handle))
	t.Cleanup(rs.Close)
	return rs
}

func (rs *recordingServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/rooms/register" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var n roomNotification
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.failNext > 0 {
		rs.failNext--
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	rs.received = append(rs.received, n)
	w.WriteHeader(http.StatusNoContent)
}

func (rs *recordingServer) count() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return len(rs.received)
}

func (rs *recordingServer) all() []roomNotification {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	out := make([]roomNotification, len(rs.received))
	copy(out, rs.received)
	return out
}

func (rs *recordingServer) setFailNext(n int) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.failNext = n
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

func newTestClient(baseURL string) *httpClient {
	return NewHTTPClient(baseURL, "10.0.4.12:8081", testLogger())
}

func TestHTTPClient_NotifyRoomCreated_SendsRoomIDAndOwnAddress(t *testing.T) {
	server := newRecordingServer(t)
	client := newTestClient(server.URL)

	client.NotifyRoomCreated("K7XQPT9M")

	waitFor(t, time.Second, func() bool { return server.count() == 1 })

	got := server.all()
	if got[0].RoomID != "K7XQPT9M" {
		t.Errorf("RoomID = %q, want %q", got[0].RoomID, "K7XQPT9M")
	}
	if got[0].ServerAddress != "10.0.4.12:8081" {
		t.Errorf("ServerAddress = %q, want %q", got[0].ServerAddress, "10.0.4.12:8081")
	}
}

func TestHTTPClient_NotifyRoomCreated_DoesNotBlockCaller(t *testing.T) {
	server := newRecordingServer(t)
	server.setFailNext(1000) // authsvc effectively unreachable

	client := newTestClient(server.URL)

	done := make(chan struct{})
	go func() {
		client.NotifyRoomCreated("SLOWROOM")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("NotifyRoomCreated blocked the caller")
	}
}

func TestHTTPClient_NotifyRoomCreated_SendsEachRoomAsItsOwnRequest(t *testing.T) {
	server := newRecordingServer(t)
	client := newTestClient(server.URL)

	client.NotifyRoomCreated("ROOM-1")
	client.NotifyRoomCreated("ROOM-2")

	waitFor(t, time.Second, func() bool { return server.count() == 2 })

	got := server.all()
	ids := map[string]bool{got[0].RoomID: true, got[1].RoomID: true}
	if !ids["ROOM-1"] || !ids["ROOM-2"] {
		t.Errorf("got %+v, want requests for both ROOM-1 and ROOM-2", got)
	}
}

func TestHTTPClient_RetriesOnFailureUntilItSucceeds(t *testing.T) {
	server := newRecordingServer(t)
	server.setFailNext(2)
	client := newTestClient(server.URL)

	client.NotifyRoomCreated("RETRY-ROOM")

	// Two retries at up to ~1.5s and ~3s (jittered exponential backoff,
	// base 1s) — 8s gives comfortable headroom without the test itself
	// dictating the backoff schedule.
	waitFor(t, 8*time.Second, func() bool { return server.count() == 1 })

	got := server.all()
	if len(got) != 1 || got[0].RoomID != "RETRY-ROOM" {
		t.Errorf("got %+v, want a single RETRY-ROOM notification after retries succeed", got)
	}
}
