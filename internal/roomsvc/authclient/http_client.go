package authclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"time"
)

const (
	registerPath = "/rooms/register"

	sendTimeout    = 5 * time.Second
	retryBaseDelay = time.Second
	retryMaxDelay  = 30 * time.Second
)

// httpClient is the real Client implementation: it POSTs a room
// notification to authsvc's POST /rooms/register over plain HTTP/JSON
// (see the package doc for why HTTP rather than gRPC, and why this sends
// one request per notification rather than batching).
type httpClient struct {
	baseURL       string
	advertiseAddr string
	httpClient    *http.Client
	log           *slog.Logger
}

// NewHTTPClient creates a Client that reports rooms created by this
// instance to authsvc at baseURL. advertiseAddr is this roomsvc instance's
// own externally-reachable address (the same value passed to
// registry.NewRegistrar) — it is sent alongside every room ID so authsvc
// knows which server now owns the room.
func NewHTTPClient(baseURL, advertiseAddr string, log *slog.Logger) *httpClient {
	return &httpClient{
		baseURL:       baseURL,
		advertiseAddr: advertiseAddr,
		httpClient:    &http.Client{Timeout: sendTimeout},
		log:           log,
	}
}

// NotifyRoomCreated implements Client.
func (c *httpClient) NotifyRoomCreated(roomID string) {
	n := roomNotification{RoomID: roomID, ServerAddress: c.advertiseAddr}
	go c.sendWithRetry(n)
}

// sendWithRetry POSTs n to authsvc, retrying with jittered exponential
// backoff until it succeeds. It always runs on its own goroutine (started
// by NotifyRoomCreated), never on the caller.
func (c *httpClient) sendWithRetry(n roomNotification) {
	delay := retryBaseDelay

	for {
		if err := c.send(n); err == nil {
			return
		} else {
			c.log.Error("failed to notify authsvc of new room, retrying", "error", err, "retry_in", delay, "room_id", n.RoomID)
		}

		jittered := delay/2 + time.Duration(rand.Int63n(int64(delay)))
		time.Sleep(jittered)

		delay *= 2
		if delay > retryMaxDelay {
			delay = retryMaxDelay
		}
	}
}

func (c *httpClient) send(n roomNotification) error {
	body, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("authclient: failed to encode notification: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+registerPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("authclient: failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("authclient: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("authclient: authsvc returned status %d", resp.StatusCode)
	}

	return nil
}
