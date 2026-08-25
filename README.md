# Cabo Backend

Backend services for Cabo, a multiplayer card game.

## Services

Two independently deployable binaries in one repo for now (see the spine, `AD-8`, for why and when that changes):

- `cmd/authsvc` — stateless tier: login, logout, authentication, allocating a client to a room server
- `cmd/roomsvc` — stateful tier: owns one game room's live state in memory, speaks WebSocket directly to clients

## Status

Greenfield — nothing is implemented yet. The architecture spine is still `draft` and grows as more decisions are made; check its `Deferred` section for what's still open before assuming something is settled.
