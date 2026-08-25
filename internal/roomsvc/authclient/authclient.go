// Package authclient notifies authsvc when this roomsvc instance creates a
// new room, so authsvc can write the room-id -> server-address entry in
// Redis (spine AD-3 reserves that write for the stateless tier — roomsvc
// cannot do it directly). See the "roomsvc Registration & Heartbeat LLD"
// for the full design: why this is HTTP/JSON rather than gRPC, and why
// calls are fire-and-forget. The LLD's original design batched
// notifications, but authsvc's actual POST /rooms/register endpoint
// (internal/authsvc/api) takes one room per request, so this client sends
// one request per notification instead — matching what authsvc actually
// implements rather than a hypothetical batch contract.
package authclient

// Client tells authsvc that a room now exists. It is an interface not
// because roomsvc has more than one caller, but so RoomManager's tests can
// pass a fake instead of making a real HTTP call (a test seam) — the same
// reasoning as authsvc/roomdirectory.RoomDirectory. httpClient is the only
// real implementation.
type Client interface {
	// NotifyRoomCreated tells authsvc that roomID is now owned by this
	// roomsvc instance. Non-blocking: sends the request in the
	// background, retrying with backoff on failure. Never returns an
	// error to the caller — there is nothing a caller could usefully do
	// differently on failure, and an authsvc outage must not affect
	// roomsvc's ability to create rooms (spine AD-6).
	NotifyRoomCreated(roomID string)
}

// roomNotification is the body POSTed to authsvc's POST /rooms/register,
// matching internal/authsvc/api.registerRoomRequest.
type roomNotification struct {
	RoomID        string `json:"room_id"`
	ServerAddress string `json:"server_address"`
}
