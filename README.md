# Cabo Backend

Backend services for Cabo, a multiplayer card game.

## Services

Two independently deployable binaries in one repo for now (see the spine, `AD-8`, for why and when that changes):

- `cmd/authsvc` — stateless tier: login, logout, authentication, allocating a client to a room server
- `cmd/roomsvc` — stateful tier: owns one game room's live state in memory, speaks WebSocket directly to clients

## Configuration

Both binaries read their configuration from environment variables — see `.env.example` for the full list, what each one does, and which binary reads it. Copy it to `.env` and fill in real values; the app does not load `.env` automatically, so export the values into your shell first (see the comment at the top of `.env.example` for how).

## Status

The architecture spine is still `draft` and grows as more decisions are made; check its `Deferred` section for what's still open before assuming something is settled. See `CLAUDE.md`'s "Current State" section for what's actually implemented so far.
