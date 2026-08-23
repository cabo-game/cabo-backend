# Cabo Backend

Backend services for Cabo, a multiplayer card game.

## Architecture

The architecture decisions for this backend are not duplicated here — they live in the shared BMAD project repository, kept in sync as the design evolves:

- **Architecture spine:** [`../cabo-bmad/_bmad-output/planning-artifacts/architecture/architecture-cabo-2026-08-22/ARCHITECTURE-SPINE.md`](../cabo-bmad/_bmad-output/planning-artifacts/architecture/architecture-cabo-2026-08-22/ARCHITECTURE-SPINE.md)

Read it before touching service boundaries, server tiers, room ownership, or discovery/coordination. See [`CLAUDE.md`](CLAUDE.md) for the full rules on working with the shared BMAD context.

## Services

Two independently deployable binaries in one repo for now (see the spine, `AD-8`, for why and when that changes):

- `cmd/authsvc` — stateless tier: login, logout, authentication, allocating a client to a room server
- `cmd/roomsvc` — stateful tier: owns one game room's live state in memory, speaks WebSocket directly to clients

## Status

Greenfield — nothing is implemented yet. The architecture spine is still `draft` and grows as more decisions are made; check its `Deferred` section for what's still open before assuming something is settled.
