# Cabo Backend — Claude Code Instructions

## Project

This repository contains the backend service of the Cabo system.

Cabo is a multi-service project. The backend is developed independently
from the other services and has its own Git repository.

## Project Context

Before making changes, read `README.md` to understand the purpose,
architecture, setup, and development conventions of this service.

Keep the README up to date when changes materially affect the documented
architecture, setup, or service behavior.

## Discussion Style and Collaboration

This section applies to all future discussions in this repository. It adds
to the existing instructions below — it does not remove or weaken any of
them.

When the user proposes an idea, approach, design, implementation strategy,
or change, do not immediately jump into an implementation breakdown.
Instead, follow these steps first:

1. **Give a brief overview.**
   Summarize, at a high level, how you would approach the problem.

2. **Critically evaluate the user's suggestion.**
   - Say clearly whether the suggestion is good, reasonable, and
     technically sound.
   - Explain why you agree or disagree.
   - Point out possible errors, weaknesses, risks, edge cases, unintended
     consequences, or scalability/maintainability concerns.
   - If a better approach exists, explain it and why it may be better.
   - Do not agree with the suggestion just because the user proposed it.
     Give an honest technical assessment.

3. **Make the discussion collaborative.**
   - Treat the interaction as a technical discussion between two
     engineers, not one-way instruction-following.
   - Share your own technical reasoning and opinions when relevant.
   - Challenge the user's assumptions when you believe they may be wrong.
   - If the user challenges your suggestion, reconsider it honestly and
     say whether their counterargument changes your recommendation.
   - Be willing to change your position when there is a stronger technical
     argument.
   - For tasks with meaningful design or architectural decisions,
     encourage iterative back-and-forth discussion before implementation.

4. **Then move to implementation.**
   Once the approach is sufficiently discussed and agreed upon, give the
   implementation breakdown and proceed incrementally, following the rest
   of the instructions in this file (including the BMAD development steps
   below).

Do not unnecessarily prolong this discussion step for simple or
unambiguous tasks — use judgment about when a task is significant enough
to warrant this back-and-forth.

## Shared BMAD Context

The shared BMAD project repository is located at:

../cabo-bmad

The shared BMAD installation, agents, skills, workflows, and project-level
artifacts are maintained there.

When working on this service, use the BMAD methodology and consult the
shared BMAD artifacts when relevant.

Important shared locations:

- `../cabo-bmad/_bmad/` — BMAD framework, modules, workflows, and configuration
- `../cabo-bmad/_bmad-output/` — BMAD-generated project artifacts and shared
  project knowledge

Do not copy the BMAD installation into this repository.
Do not modify files under `../cabo-bmad/_bmad/` from this repository.

## Shared Project Context

Before making changes that affect system-level behavior, consult the
relevant artifacts in `../cabo-bmad/_bmad-output/`.

This includes changes involving:

- system architecture
- service boundaries
- game rules
- GameState
- API contracts
- WebSocket contracts
- authentication/authorization
- database ownership
- cross-service communication
- AI/backend interaction
- architectural decisions

Do not create duplicate system-level artifacts in this repository.

## BMAD Development

Follow the BMAD methodology for development work.

When a BMAD workflow or skill is appropriate for the task, use it rather
than inventing an alternative development process.

For significant changes:

1. Understand the relevant project context.
2. Identify affected requirements and architecture.
3. Check existing decisions and contracts.
4. Plan the implementation.
5. Implement the change.
6. Test the implementation.
7. Review the result against the requirements and architecture.

## Current State

**Ownership split (check before editing):** as of this writing, `internal/roomsvc/`
and `cmd/roomsvc/` are being developed by a separate tool/session from
`internal/authsvc/` and `cmd/authsvc/`. If you're working in one service,
confirm with the user before editing files under the other — do not assume
this split still holds if it looks stale (e.g. no activity on one side for
a long time), but don't assume it's gone either. Ask.

A Go toolchain (1.27.x) is available and has been used to build and test
this code. `go build ./...`, `go vet ./...`, and `go test ./... -race` all
pass as of the `registry` package landing (see below); `go mod tidy`
produces no changes. Re-verify and update this note if that ever stops
being true.

This repo is being developed under the Amelia persona from
`../cabo-bmad/.claude/skills/bmad-agent-dev/`, with team customizations in
`../cabo-bmad/_bmad/custom/bmad-agent-dev.toml` (TDD discipline, halt-for-
design-approval before coding, mandatory architecture docs per class, no
tests for logic-free code). Load that persona for consistent behavior
across sessions.

Two decisions below are placeholders, expected to change:

- **Module path** is `github.com/cabo/cabo-backend` — a placeholder; the
  real GitHub org is not yet decided. Changing it later means a repo-wide
  find-and-replace across every import line (not a config-only change,
  unlike a bundler path alias) — budget for that when it happens.
- **WebSocket library** is `github.com/coder/websocket`. This repo
  originally used `nhooyr.io/websocket`; that module is deprecated in favor
  of the same code maintained under this new path. If a dependency update
  ever suggests moving off `coder/websocket`, check whether it's another
  such rename before assuming a real migration is needed.

Implemented so far:

- `cmd/roomsvc/main.go` — roomsvc entrypoint. Constructs a `registry.Registrar`
  (`newRegistrar`) and starts it before serving, constructs an
  `authclient.Client` (`newAuthClient`) and a `game.RoomManager` wrapping
  it, constructs a `ws.PingConfig` (`newPingConfig`, reading
  `ROOMSVC_PING_INTERVAL`/`ROOMSVC_PING_TIMEOUT`, default 30s/10s — see the
  `ws` note below), starts an HTTP server on `:8081`, mounts
  `internal/roomsvc/ws.Handler` at `/ws` — `onConnect` runs `handoff.Handle`
  on every new connection (see the `handoff` note below) before falling
  through to `conn.ReadLoop`, passing an `onClose` callback that calls
  `roomManager.RemovePlayer` so a dropped connection frees its seat — and
  shuts down gracefully on SIGINT/SIGTERM, stopping the registrar as part
  of that.
- `internal/roomsvc/ws/` — WebSocket transport, no game-domain knowledge.
  `Handler` upgrades an HTTP request and hands the resulting `Connection` to
  an `OnConnect` hook. `Connection` owns read (`ReadOne`, `ReadLoop`), write
  (`Write`), and close (`Close`, `CloseNow`) for one client connection.
  `ReadOne` reads exactly one message without closing on success — added
  for `handoff` to inspect the first message before deciding what happens
  next — and shares frame-reading/error-classification with `ReadLoop` via
  a private `readFrame` helper. `ReadLoop` now also takes an `onClose
  func()` parameter, called exactly once after the loop ends for any
  reason (client close, read error, context cancellation, or a ping
  timeout — see next) — the seam for a caller to release whatever it
  associated with the connection while the loop was running (e.g. seating
  a player in a room), without `ws` needing any knowledge of what that
  association is; `ws` still has no dependency on `game`. `ReadLoop` also
  runs a ping/pong keepalive (`pingLoop`, unexported) for the life of the
  loop, configured by a `PingConfig{Interval, Timeout}` passed to
  `NewConnection`/`NewHandler`: it sends a ping every `Interval` and, if no
  pong is observed within `Timeout`, calls `CloseNow` — this is what
  detects a peer that has gone silent without a normal WebSocket close
  (e.g. a network partition), which a plain read error can't catch, since
  a dead-but-silent TCP connection blocks `Read` forever instead of
  erroring. `coder/websocket`'s `Ping` only completes once the peer's pong
  is observed by an in-progress `Read` on the same connection, so
  `pingLoop` runs as a sibling goroutine to the read loop, started and
  cancelled together with it — never before or after. `Close` performs a full close handshake
  (waits up to 5s for the peer's close frame, per `coder/websocket`'s own
  docs); `CloseNow` skips that wait entirely. This distinction is not
  cosmetic: `handoff.reject` originally called `Close` right after writing
  a rejection, and its tests hung for exactly that 5s wait, since a test
  client has no reason to send a close frame back after already being told
  why the connection is ending. Use `CloseNow` whenever the peer has
  already gotten its answer through some other channel.
- `internal/roomsvc/game/` — game domain, package `game` (imports `ws` for
  `Player.Conn`; `ws` has no dependency back on `game`). `GameRoom` (id, `*GameState`,
  `MaxPlayers`, seated `Player`s) cannot be constructed without at least one
  player, and `NewGameRoom` rejects a `maxPlayers` outside `1..MaxPlayersPerRoom`
  with an error. `JoinRoom` seats a new player under a mutex, so the
  capacity check and the append can't race across goroutines. `Player`
  wraps a `*ws.Connection` with player identity; `NewPlayer(conn)`
  generates a random, opaque ID (`generatePlayerID`, 128 bits via
  `crypto/rand`) — not authentication, just enough to distinguish players
  within this instance's rooms (real identity is a separate, undecided
  design effort). Existing tests still build `Player{ID: "..."}` literals
  directly where a fixed ID is more convenient for assertions; `NewPlayer`
  is for real connections, via `handoff` (see below). `GameState` is an
  explicit placeholder — see Shared Project Context above; its real shape
  is deferred to the shared game-logic spec, not invented here.
  `room_code.go` generates the room's id: an 8-character, `crypto/rand`-backed
  code from an alphabet with ambiguous characters removed. Collision
  checking against other live rooms is a known, intentional gap — no room
  registry exists yet to check against. `rules.go` holds game constants
  mirrored from `../cabo-bmad/docs/cabo.md` (currently `MaxPlayersPerRoom
  = 4`) — update that doc first, this file second, if a rule ever changes.
  `RoomManager` tracks every `GameRoom` live on this instance, keyed by
  room ID (`CreateRoom`, `GetRoom`), guarded by an `RWMutex`. `GameRoom`
  now has `RemovePlayer(playerID)`, which removes a seated player and
  reports whether the room is now empty; `RoomManager.RemovePlayer(room,
  playerID)` calls it and, if the room is now empty, calls the new
  `RemoveRoom(roomID)` to drop the room from this instance's registry too.
  This closes the disconnect case of the room-removal gap: a connection
  that closes (client close, network error, or `ws.Connection.ReadLoop`'s
  `onClose` hook firing for any other reason) no longer leaves a
  permanently occupied, unreachable seat. Full match/round-ending-driven
  removal (a game ending with players still connected) is still
  undecided — `RemoveRoom` is a separate method from `RemovePlayer`
  precisely so that future trigger can call it directly. `NewRoomManager`
  also takes an `authclient.Client`; `CreateRoom` calls `NotifyRoomCreated`
  after registering the room (never before — a failed `NewGameRoom` call
  notifies nobody). Constructed in `cmd/roomsvc/main.go` and called from
  `onConnect`, via `handoff.Handle` (see below) for the create/join
  handshake and via `ws.Connection.ReadLoop`'s new `onClose` callback
  parameter for cleanup when the connection ends.
- `cmd/authsvc/main.go` — implemented, scoped to room-allocation only: no
  login/auth/user-identity work (that's a separate, undecided design
  effort — see AD-6's "login, logout, authentication" wording, not
  implemented at all yet). The room-creation handoff flow is recorded as
  AD-9 in the shared spine
  (`../cabo-bmad/_bmad-output/planning-artifacts/architecture/architecture-cabo-2026-08-22/ARCHITECTURE-SPINE.md`).
  `main.go` connects to Redis and etcd (env vars `REDIS_ADDR`,
  `ETCD_ENDPOINTS`), constructs a `RedisRoomDirectory` and an
  `EtcdServerPicker`, hands both to `api.Server`, and serves on `:8080`
  with graceful shutdown — same shape as `roomsvc/main.go`.
- `internal/roomsvc/registry/` — etcd liveness registration for this
  roomsvc instance (AD-7). `Registrar.Start` grants a lease, writes
  `/roomsvc/servers/<advertiseAddr>` → `{address, capacity}` under it, and
  renews it in the background for the life of the process; `Stop` revokes
  the lease explicitly so a graceful shutdown doesn't leave a stale entry
  for the TTL to expire. If the keepalive stream ever dies, it
  re-registers under a new lease with jittered exponential backoff rather
  than staying unregistered. Depends on a small `etcdClient` interface,
  not `*clientv3.Client` directly — `clientv3`'s `Put` takes an opaque
  `OpOption` with no way to read a lease ID back out, so tests fake
  `etcdClient` directly instead of standing up a real etcd server (the
  official integration-test package is heavy and was fragile to resolve
  at the pinned etcd version). Wired into `cmd/roomsvc/main.go`:
  `newRegistrar` reads `ETCD_ENDPOINTS`, `ROOMSVC_ADVERTISE_ADDR`
  (required), `ROOMSVC_LEASE_TTL` (default 10s), `ROOMSVC_CAPACITY`
  (default 0), and `main` calls `Start` before serving and `Stop` during
  graceful shutdown. See the "roomsvc Registration & Heartbeat LLD"
  (shared with the user as an Artifact) for the accepted design this
  implements, including the TTL=10s choice.
- `internal/roomsvc/authclient/` — notifies authsvc when this instance
  creates a room, via `Client.NotifyRoomCreated(roomID)`. The LLD's
  original design batched notifications against an assumed
  `POST /internal/v1/rooms` array endpoint; once `internal/authsvc/api`
  was actually built with a single-object `POST /rooms/register` (`204`
  on success), batching was dropped — `httpClient` now sends one request
  per notification instead, matching the real contract rather than the
  LLD's original guess. Fire-and-forget: `NotifyRoomCreated` starts a
  goroutine and returns immediately, retrying with jittered exponential
  backoff until the request succeeds; it never returns an error; an
  authsvc outage must not block room creation (AD-6). `Client` is an
  interface purely as a test seam (same reasoning as
  `authsvc/roomdirectory.RoomDirectory`) — `RoomManager` is the only
  caller. Wired into `game.RoomManager` (`NewRoomManager` now takes a
  `Client`), and both are constructed in `cmd/roomsvc/main.go`:
  `newAuthClient` reads `ROOMSVC_ADVERTISE_ADDR` (same value `newRegistrar`
  reads — each constructor validates its own env vars independently, no
  shared config struct) and `AUTHSVC_INTERNAL_ADDR` (base URL of authsvc's
  internal API), builds the `httpClient`, and hands it to
  `game.NewRoomManager`, which `onConnect` now calls into via
  `handoff.Handle` (see below) — room assignment onto WebSocket
  connections is no longer a deferred item.
- `internal/roomsvc/handoff/` — the WebSocket create/join handshake every
  new connection performs before joining the normal game message flow
  (spine AD-9's roomsvc-side half). The exact message shape was previously
  in the shared spine's "Deferred" list ("Exact WebSocket message/protocol
  contract between client and backend"); it's now decided pragmatically
  (no client existed yet to have assumed something else) and recorded in
  `internal/roomsvc/ARCHITECTURE.md`: the client sends one JSON message —
  `{"action":"create_room"}` or `{"action":"join_room","room_id":"..."}` —
  and `Handle` replies with one JSON message — `{"status":"ok","room_id":"..."}`
  or `{"status":"error","message":"..."}`. `Handle(ctx, conn, roomManager,
  log) (*Player, *GameRoom, error)` reads exactly one message via
  `Connection.ReadOne`, dispatches to `RoomManager.CreateRoom` or
  `RoomManager.GetRoom`+`GameRoom.JoinRoom`, and writes back the outcome.
  On failure it closes the connection with `CloseNow` (not `Close` — see
  the `ws` note above for why: the client already has its answer in the
  JSON body, so there's nothing to wait for). Depends on both `ws` and
  `game`; neither depends back on it. Tested against a real WebSocket
  server via `httptest`, same pattern as `ws`'s own tests. Wired into
  `cmd/roomsvc/main.go`'s `onConnect`: on success, `onConnect` logs the
  player/room and continues with `conn.ReadLoop`; on failure, it logs and
  returns (the connection is already closed).
- `internal/authsvc/roomdirectory/` — the Redis half of the flow above.
  `RoomDirectory` is an interface (`RegisterRoom`, `LookupRoom`); callers
  depend only on it, never on Redis directly, so the backing store can be
  swapped later by writing a new implementation and changing one
  constructor call. `RedisRoomDirectory` is the current implementation —
  every entry gets a TTL (a safety net against orphaned entries, since
  nothing cleans them up yet) and translates a Redis miss into
  `ErrRoomNotFound` so callers never need to import `go-redis`. Tested
  against `miniredis` (an in-process fake), not a real Redis instance.
- `internal/authsvc/redisclient/` — bare Redis connection setup
  (`New(ctx, addr)`), kept separate from `roomdirectory` so "how to
  connect" and "what operations we need" stay independent.
- `internal/authsvc/serverpicker/` — `ServerPicker` interface
  (`PickServer`); callers never depend on etcd directly. `EtcdServerPicker`
  reads the same `/roomsvc/servers/` roster roomsvc's `Registrar` writes
  (AD-7) and returns the first registration found — no placement policy is
  decided yet. Tested against a fake `etcdReader`, not a real etcd server.
- `internal/authsvc/api/` — the HTTP controller layer implementing AD-9:
  `POST /rooms/allocate` (pick a server, AD-9 step 1), `POST
  /rooms/register` (roomsvc reports a new room, AD-9 steps 3-4), `GET
  /rooms/{roomID}` (second player looks up a room's server). Depends only
  on `RoomDirectory` and `ServerPicker`, never on Redis/etcd directly. The
  register-room request shape (`{room_id, server_address}`, `204` on
  success) is now the contract `internal/roomsvc/authclient` builds
  against as-is — no batching, one request per room. If this shape ever
  needs to change (e.g. to accept a batch), that's a coordinated change on
  both sides, not a roomsvc-only one.

For how these pieces connect, read `internal/roomsvc/ARCHITECTURE.md` and
`internal/authsvc/ARCHITECTURE.md` (class diagrams per service) before
making structural changes — the fastest way to get oriented. Per-class
purpose and method docs live in
`../cabo-bmad/_bmad-output/implementation-artifacts/architecture/`.

Nothing here is authoritative over the shared spine
(`../cabo-bmad/_bmad-output/planning-artifacts/architecture/`) — if this
section and the spine ever disagree, the spine wins and this section is
stale and should be corrected.

## Running Locally

Neither service bundles its dependencies — etcd and Redis are separate
processes you must already have running before either `main.go` will
start (both dial/ping at startup and exit on failure). Both `authsvc` and
`roomsvc` must point at the **same** etcd cluster: `roomsvc` writes its
liveness there (AD-7), `authsvc` only reads it.

Minimal local setup (illustrative, not verified end-to-end as an
installation guide):

```bash
docker run -d -p 2379:2379 --name etcd quay.io/coreos/etcd:v3.7.1 \
  etcd --advertise-client-urls http://0.0.0.0:2379 \
       --listen-client-urls http://0.0.0.0:2379
docker run -d -p 6379:6379 --name redis redis:8
```

Required env vars:

- `authsvc`: `REDIS_ADDR`, `ETCD_ENDPOINTS` (comma-separated).
- `roomsvc`: `ETCD_ENDPOINTS`, `ROOMSVC_ADVERTISE_ADDR` (the address other
  services should use to reach this instance — not necessarily its bind
  address; matters behind Docker/NAT/a load balancer), `AUTHSVC_INTERNAL_ADDR`
  (authsvc's base URL, e.g. `http://10.0.4.5:8080`). Optional:
  `ROOMSVC_LEASE_TTL` (default 10s), `ROOMSVC_CAPACITY` (default 0),
  `ROOMSVC_PING_INTERVAL` (default 30s), `ROOMSVC_PING_TIMEOUT` (default
  10s, must be less than `ROOMSVC_PING_INTERVAL`).

If running both services as separate containers, `ROOMSVC_ADVERTISE_ADDR`
must be reachable from `authsvc`'s container — `localhost` will not
resolve to anything useful across containers.

## Testing

Do not write a unit test for code that has no logic to break (e.g. a plain
data struct with no methods or behavior). Add tests once the code has
behavior worth verifying.

## Backend Responsibility

The backend is responsible for the authoritative game state and server-side
game logic.

Client-provided state must not be treated as authoritative.

All player actions must be validated by the backend before they can mutate
authoritative game state.

## Cross-Service Changes

If a backend change requires changes to another Cabo service or changes a
shared contract:

1. Identify the impact.
2. Consult the shared BMAD context.
3. Update the relevant shared artifact/contract through the appropriate
   BMAD workflow.
4. Implement the backend changes.
5. Clearly identify the changes required by other services.

## Scope

Keep backend-specific implementation details in this repository.

Keep system-wide requirements, architecture, decisions, and contracts in
the shared BMAD project repository.