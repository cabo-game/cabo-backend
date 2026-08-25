package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cabo/cabo-backend/internal/authsvc/api"
	"github.com/cabo/cabo-backend/internal/authsvc/roomdirectory"
	"github.com/cabo/cabo-backend/internal/authsvc/serverpicker"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeRoomDirectory struct {
	rooms       map[string]string
	registerErr error
}

func newFakeRoomDirectory() *fakeRoomDirectory {
	return &fakeRoomDirectory{rooms: make(map[string]string)}
}

func (f *fakeRoomDirectory) RegisterRoom(ctx context.Context, roomID, serverAddress string) error {
	if f.registerErr != nil {
		return f.registerErr
	}
	f.rooms[roomID] = serverAddress
	return nil
}

func (f *fakeRoomDirectory) LookupRoom(ctx context.Context, roomID string) (string, error) {
	address, ok := f.rooms[roomID]
	if !ok {
		return "", roomdirectory.ErrRoomNotFound
	}
	return address, nil
}

type fakeServerPicker struct {
	address string
	err     error
}

func (f *fakeServerPicker) PickServer(ctx context.Context) (string, error) {
	return f.address, f.err
}

func TestHandleAllocateServer_ReturnsPickedAddress(t *testing.T) {
	picker := &fakeServerPicker{address: "10.0.0.1:8081"}
	server := api.NewServer(newFakeRoomDirectory(), picker, testLogger())
	ts := httptest.NewServer(server.Routes())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/rooms/allocate", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /rooms/allocate failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		ServerAddress string `json:"server_address"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.ServerAddress != "10.0.0.1:8081" {
		t.Errorf("server_address = %q, want %q", body.ServerAddress, "10.0.0.1:8081")
	}
}

func TestHandleAllocateServer_ReturnsServiceUnavailableWhenNoServers(t *testing.T) {
	picker := &fakeServerPicker{err: serverpicker.ErrNoServersAvailable}
	server := api.NewServer(newFakeRoomDirectory(), picker, testLogger())
	ts := httptest.NewServer(server.Routes())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/rooms/allocate", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /rooms/allocate failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
}

func TestHandleRegisterRoom_RegistersRoomAndReturnsNoContent(t *testing.T) {
	directory := newFakeRoomDirectory()
	server := api.NewServer(directory, &fakeServerPicker{}, testLogger())
	ts := httptest.NewServer(server.Routes())
	defer ts.Close()

	body := `{"room_id":"ABCD1234","server_address":"10.0.0.1:8081"}`
	resp, err := http.Post(ts.URL+"/rooms/register", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /rooms/register failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	if directory.rooms["ABCD1234"] != "10.0.0.1:8081" {
		t.Errorf("directory.rooms[%q] = %q, want %q", "ABCD1234", directory.rooms["ABCD1234"], "10.0.0.1:8081")
	}
}

func TestHandleRegisterRoom_ReturnsBadRequestForMissingFields(t *testing.T) {
	server := api.NewServer(newFakeRoomDirectory(), &fakeServerPicker{}, testLogger())
	ts := httptest.NewServer(server.Routes())
	defer ts.Close()

	body := `{"room_id":"ABCD1234"}`
	resp, err := http.Post(ts.URL+"/rooms/register", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /rooms/register failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestHandleRegisterRoom_ReturnsBadRequestForInvalidJSON(t *testing.T) {
	server := api.NewServer(newFakeRoomDirectory(), &fakeServerPicker{}, testLogger())
	ts := httptest.NewServer(server.Routes())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/rooms/register", "application/json", bytes.NewBufferString("not json"))
	if err != nil {
		t.Fatalf("POST /rooms/register failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestHandleLookupRoom_ReturnsRegisteredAddress(t *testing.T) {
	directory := newFakeRoomDirectory()
	directory.rooms["ABCD1234"] = "10.0.0.1:8081"
	server := api.NewServer(directory, &fakeServerPicker{}, testLogger())
	ts := httptest.NewServer(server.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rooms/ABCD1234")
	if err != nil {
		t.Fatalf("GET /rooms/ABCD1234 failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		ServerAddress string `json:"server_address"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.ServerAddress != "10.0.0.1:8081" {
		t.Errorf("server_address = %q, want %q", body.ServerAddress, "10.0.0.1:8081")
	}
}

func TestHandleLookupRoom_ReturnsNotFoundForUnknownRoom(t *testing.T) {
	server := api.NewServer(newFakeRoomDirectory(), &fakeServerPicker{}, testLogger())
	ts := httptest.NewServer(server.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rooms/does-not-exist")
	if err != nil {
		t.Fatalf("GET /rooms/does-not-exist failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}
