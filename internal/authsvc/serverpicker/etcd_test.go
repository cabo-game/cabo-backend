package serverpicker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeEtcdReader struct {
	values []string
	err    error
}

func (f *fakeEtcdReader) GetByPrefix(ctx context.Context, prefix string) ([]string, error) {
	return f.values, f.err
}

func TestEtcdServerPicker_PickServer_ReturnsAddressFromRoster(t *testing.T) {
	reader := &fakeEtcdReader{
		values: []string{`{"address":"10.0.0.1:8081","capacity":0}`},
	}
	picker := NewEtcdServerPicker(reader, testLogger())

	got, err := picker.PickServer(context.Background())
	if err != nil {
		t.Fatalf("PickServer returned error: %v", err)
	}
	if got != "10.0.0.1:8081" {
		t.Errorf("PickServer = %q, want %q", got, "10.0.0.1:8081")
	}
}

func TestEtcdServerPicker_PickServer_ReturnsErrNoServersAvailableWhenRosterEmpty(t *testing.T) {
	reader := &fakeEtcdReader{values: []string{}}
	picker := NewEtcdServerPicker(reader, testLogger())

	_, err := picker.PickServer(context.Background())
	if !errors.Is(err, ErrNoServersAvailable) {
		t.Errorf("PickServer error = %v, want ErrNoServersAvailable", err)
	}
}

func TestEtcdServerPicker_PickServer_PropagatesReadError(t *testing.T) {
	reader := &fakeEtcdReader{err: errors.New("etcd unreachable")}
	picker := NewEtcdServerPicker(reader, testLogger())

	_, err := picker.PickServer(context.Background())
	if err == nil {
		t.Error("expected an error when the etcd read fails, got nil")
	}
}

func TestEtcdServerPicker_PickServer_ReturnsErrorForMalformedRegistration(t *testing.T) {
	reader := &fakeEtcdReader{values: []string{"not valid json"}}
	picker := NewEtcdServerPicker(reader, testLogger())

	_, err := picker.PickServer(context.Background())
	if err == nil {
		t.Error("expected an error for a malformed registration value, got nil")
	}
}
