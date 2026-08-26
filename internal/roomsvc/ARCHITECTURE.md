# roomsvc — Class Diagrams

One consolidated view of every struct in roomsvc so far and how they link
together. Individual per-class docs (purpose + method descriptions) live in
`cabo-bmad/_bmad-output/implementation-artifacts/architecture/`.

roomsvc is split into five packages: `ws` (WebSocket transport), `game`
(game domain), `registry` (etcd liveness), `authclient` (authsvc
notification), and `handoff` (the WebSocket create/join handshake, layered
above both `ws` and `game`). Dependencies only point one way — `game`
imports `ws` for `Player.Conn`; `handoff` imports both `ws` and `game`;
none of `ws`, `game`, `registry`, or `authclient` import `handoff`.

## internal/roomsvc/ws — WebSocket transport

Has no knowledge of game rules or room state. `Handler` initiates a
connection; `Connection` owns everything about it afterward (read, write,
close).

```mermaid
classDiagram
    class Handler {
        -log *slog.Logger
        +OnConnect func(*Connection)
        +NewHandler(log, onConnect) *Handler
        +ServeHTTP(w, r)
    }

    class Connection {
        -conn *websocket.Conn
        -remoteAddr string
        -log *slog.Logger
        -writeMu sync.Mutex
        +NewConnection(conn, remoteAddr, log) *Connection
        +RemoteAddr() string
        +ReadOne(ctx) ([]byte, error)
        +ReadLoop(ctx, onMessage)
        +Write(ctx, data) error
        +Close(code, reason)
        +CloseNow()
    }

    Handler --> Connection : creates on upgrade, passes to OnConnect
```

`ReadOne` reads exactly one message and returns it, without closing the
connection on success — added for `handoff` (below) to inspect the first
message before deciding what happens next. It shares frame-reading and
error-classification with `ReadLoop` via a private `readFrame` helper, so
both apply the same context-cancellation handling (see `ReadLoop`'s doc
comment: `coder/websocket`'s `Read` doesn't reliably unblock on `ctx`
cancellation, so both watch `ctx` and force-close instead).

`Close` performs a full WebSocket close handshake and waits up to 5s for
the peer to send back its own close frame — appropriate when ending a
connection the peer might still want to gracefully wind down (e.g. server
shutdown). `CloseNow` skips that handshake entirely; use it when the peer
already has no reason to cooperate — e.g. it was just told why the
connection is ending through an application-level message, so there's
nothing to wait for. This distinction mattered in practice: `handoff`
(below) originally called `Close` right after writing a rejection, and its
tests hung for exactly the wait `Close` documents, because the test client
never sent a close frame back.

## internal/roomsvc/game — game domain

`GameRoom` is one game's authoritative state. It cannot be constructed
without at least one `Player` already seated. `Player` wraps a `Connection`
with player identity, keeping transport and identity separate. `GameState`
is a placeholder pending the game-logic spec (deck rules, values, powers —
deferred in the architecture spine).

```mermaid
classDiagram
    class GameRoom {
        +ID string
        +State *GameState
        +MaxPlayers int
        +Players []*Player
        -log *slog.Logger
        -mu sync.Mutex
        +NewGameRoom(firstPlayer, maxPlayers, log) (*GameRoom, error)
        +JoinRoom(player) error
    }

    class Player {
        +ID string
        +Conn *ws.Connection
        +NewPlayer(conn) *Player
    }

    class GameState {
        <<placeholder>>
    }

    GameRoom "1" --> "1" GameState : holds
    GameRoom "1" --> "1..*" Player : Players
    Player --> Connection : Conn
```

`NewPlayer` generates a random, opaque ID for the player (`generatePlayerID`,
128 bits via `crypto/rand`) — not authentication, just enough to
distinguish players within this instance's rooms. Real player identity is
a separate, undecided design effort. Existing tests still construct
`Player{ID: "..."}` literals directly where a fixed, predictable ID is
more convenient for assertions; `NewPlayer` is for real connections, via
`handoff` (below).

`RoomManager` holds every `GameRoom` currently live on this instance, keyed
by room ID — it is how a room created for player 1 gets found again when
player 2 joins.

```mermaid
classDiagram
    class RoomManager {
        -mu sync.RWMutex
        -rooms map~string, GameRoom~
        -log *slog.Logger
        +NewRoomManager(log) *RoomManager
        +CreateRoom(firstPlayer, maxPlayers) (*GameRoom, error)
        +GetRoom(roomID) (*GameRoom, error)
    }

    RoomManager "1" --> "0..*" GameRoom : rooms
```

## internal/roomsvc/registry — etcd liveness registration

Wraps etcd's lease mechanism so the rest of roomsvc never touches etcd
directly (spine AD-7). No knowledge of `GameRoom` or `Player`. `Registrar`
depends on the small `etcdClient` interface, not `*clientv3.Client`
directly — `clientv3.Lease`/`clientv3.KV`'s `Put` takes an opaque
`OpOption` with no public way to read a lease ID back out, which makes it
untestable without a real etcd server. `etcdClient` instead takes a plain
`leaseID` argument; `clientv3Adapter` is the only real implementation,
translating to `clientv3.WithLease` underneath. Tests fake `etcdClient`
directly — no etcd server involved (same pattern as
`authsvc/roomdirectory.RoomDirectory`).

One `Registrar` per process: grants a lease, writes this instance's
address+capacity under it, and keeps the lease alive for as long as the
process runs. If the keepalive stream ever closes (etcd unreachable, lease
lost), it re-registers under a new lease with jittered exponential backoff
instead of leaving the instance permanently unregistered. See the shared
LLD (`roomsvc Registration & Heartbeat LLD`) for the full lease lifecycle
and TTL choice.

```mermaid
classDiagram
    class etcdClient {
        <<interface>>
        +Grant(ctx, ttlSeconds) (leaseID, error)
        +Put(ctx, key, val, lease) error
        +KeepAlive(ctx, lease) (~chan struct~{~}~, error)
        +Revoke(ctx, lease) error
    }

    class clientv3Adapter {
        -client *clientv3.Client
        +NewClientv3Adapter(client) etcdClient
    }

    class Registrar {
        -client etcdClient
        -advertiseAddr string
        -capacity int
        -leaseTTL time.Duration
        -log *slog.Logger
        -leaseID leaseID
        -cancel context.CancelFunc
        -done chan struct~{~}~
        +NewRegistrar(client, advertiseAddr, capacity, leaseTTL, log) *Registrar
        +Start(ctx) error
        +Stop(ctx) error
    }

    etcdClient <|.. clientv3Adapter : implements
    Registrar --> etcdClient : Grant / Put / KeepAlive / Revoke
```

`Start` blocks until the first successful `Put` so `main.go` knows
registration succeeded before serving WebSocket traffic; the keepalive and
re-registration loop then continues in a background goroutine
(`keepAliveLoop`) for the life of the process. `Stop` cancels that
goroutine, waits for it to exit, then revokes the lease explicitly so the
registration disappears immediately on a graceful shutdown, instead of
waiting out the TTL.

## internal/roomsvc/authclient — authsvc notification

Thin HTTP client, also with no knowledge of `GameRoom` or `Player`: it
takes a room ID and reports it to authsvc's `POST /rooms/register`
(`internal/authsvc/api`) so authsvc can write the room-id →
server-address entry in Redis (spine AD-3 reserves that write for the
stateless tier — roomsvc cannot do it directly). Calls are fire-and-forget
from the caller's side: `NotifyRoomCreated` starts a goroutine and returns
immediately, and never returns an error, since an authsvc outage must not
affect roomsvc's ability to create rooms (spine AD-6, independent tiers).
Each notification is retried on its own with jittered exponential backoff
until it succeeds.

The LLD this package implements ("roomsvc Registration & Heartbeat LLD")
originally specified batching notifications (flush at 2 pending or a short
linger timeout) against an assumed `POST /internal/v1/rooms` array
endpoint. Once `internal/authsvc/api.handleRegisterRoom` was actually built
(a single-object body, `204 No Content`, path `/rooms/register`), batching
was dropped in favor of matching the real contract — one HTTP request per
`NotifyRoomCreated` call. If a genuine need for batching resurfaces, it
would require a coordinated change to the authsvc endpoint too, not just
this client.

```mermaid
classDiagram
    class Client {
        <<interface>>
        +NotifyRoomCreated(roomID string)
    }

    class httpClient {
        -baseURL string
        -advertiseAddr string
        -httpClient *http.Client
        -log *slog.Logger
        +NewHTTPClient(baseURL, advertiseAddr, log) *httpClient
        +NotifyRoomCreated(roomID string)
    }

    Client <|.. httpClient : implements
```

`Client` is an interface — not because roomsvc has more than one caller,
but so `RoomManager`'s tests can pass a fake instead of making a real HTTP
call (a test seam; see the LLD for the full reasoning). `httpClient` is the
only real implementation. `advertiseAddr` is the same value passed to
`registry.NewRegistrar` — this instance's own externally-reachable
address, sent alongside every room ID.

`RoomManager` takes a `Client` in its constructor and calls
`NotifyRoomCreated` after registering a newly created room (see
`internal/roomsvc/game`, above). Both `RoomManager` and `httpClient` are
now constructed in `cmd/roomsvc/main.go` (`newAuthClient` reads
`ROOMSVC_ADVERTISE_ADDR` and `AUTHSVC_INTERNAL_ADDR`, builds the
`httpClient`, and hands it to `game.NewRoomManager`); `RoomManager` itself
is threaded into `onConnect` as a constructor parameter, ready for when
WebSocket-to-room assignment is decided, but `onConnect` does not call any
of its methods yet — room assignment onto WebSocket connections is still a
deferred spine item.

## internal/roomsvc/handoff — WebSocket create/join handshake

Every new WebSocket connection performs a one-message handshake before it
joins the normal game message flow: the client says whether it wants to
create a new room or join an existing one, and roomsvc replies once with
the outcome. This implements roomsvc's half of spine AD-9's client-driven
handoff. The exact wire format was previously listed in the shared spine's
"Deferred" list ("Exact WebSocket message/protocol contract between
client and backend") — it was decided pragmatically here, since no client
existed yet to have assumed something else, and is recorded below as the
agreed contract.

### Message contract

The client sends exactly one JSON message immediately after the
connection opens:

```jsonc
// Create a new room:
{"action": "create_room"}

// Join an existing room:
{"action": "join_room", "room_id": "K7XQPT9M"}
```

roomsvc replies with exactly one JSON message:

```jsonc
// Success:
{"status": "ok", "room_id": "K7XQPT9M"}

// Failure (unknown action, invalid JSON, unknown room, full room, etc.):
{"status": "error", "message": "roomsvc: room K7XQPT9M is full (max 4 players)"}
```

An explicit `"action"` field was chosen over inferring intent from
`room_id` being present/absent/empty — the latter is ambiguous in JSON
(missing field? empty string? `null`?) and less self-documenting in logs.

On success, the connection stays open and continues into the normal
`ReadLoop`. On failure, roomsvc has already closed the connection (via
`CloseNow` — see the `ws` section above for why not `Close`); there is
nothing further to send or read.

```mermaid
classDiagram
    class request {
        +Action string
        +RoomID string
    }

    class response {
        +Status string
        +RoomID string
        +Message string
    }

    class handoff_pkg {
        <<internal/roomsvc/handoff/handoff.go>>
        +Handle(ctx, conn, roomManager, log) (*Player, *GameRoom, error)
        -reject(ctx, conn, log, message) error
    }

    handoff_pkg ..> request : unmarshals the first message into
    handoff_pkg ..> response : marshals success/error replies from
    handoff_pkg ..> RoomManager : CreateRoom / GetRoom
    handoff_pkg ..> Player : NewPlayer(conn)
```

`Handle` reads exactly one message via `Connection.ReadOne`, decides
`create_room` vs `join_room`, calls `RoomManager` accordingly, and writes
back the outcome. On success it returns the `*Player` and `*GameRoom` so
the caller (`cmd/roomsvc/main.go`'s `onConnect`) can log context and
continue with `conn.ReadLoop`. It depends on both `ws` and `game` (a new
layer above them, not touched by either) and has no dependency back from
either into it.

## How it all fits together

```mermaid
classDiagram
    class roomsvc_main {
        <<cmd/roomsvc/main.go>>
        +main()
        -newRegistrar(log) (*Registrar, error)
        -newAuthClient(log) (Client, error)
        -onConnect(log, roomManager) func(*Connection)
    }

    class Handler
    class Connection
    class handoff_pkg["handoff.Handle"]
    class RoomManager
    class GameRoom
    class Player
    class GameState
    class Registrar
    class Client
    class httpClient

    roomsvc_main ..> Handler : constructs, mounts at /ws
    roomsvc_main ..> Registrar : constructs, Start() before serving, Stop() on shutdown
    roomsvc_main ..> httpClient : constructs via newAuthClient
    roomsvc_main ..> RoomManager : constructs, injects httpClient, passes into onConnect
    roomsvc_main ..> handoff_pkg : onConnect() calls Handle, then Connection.ReadLoop
    Handler --> Connection : creates on upgrade
    handoff_pkg --> Connection : ReadOne / Write / CloseNow
    handoff_pkg --> RoomManager : CreateRoom / GetRoom
    handoff_pkg --> Player : NewPlayer(conn)
    RoomManager --> GameRoom : CreateRoom / GetRoom
    RoomManager --> Client : NotifyRoomCreated after CreateRoom
    Client <|.. httpClient : implements
    GameRoom --> Player : Players
    GameRoom --> GameState : State
    Player --> Connection : Conn
```

A client connects over WebSocket (`Handler` → `Connection`). `onConnect`
immediately calls `handoff.Handle`, which reads the client's create/join
message and calls `RoomManager.CreateRoom` (seating a new `Player` as the
first occupant of a new `GameRoom`) or `RoomManager.GetRoom` +
`GameRoom.JoinRoom` (seating a new `Player` into an existing one) —
spine AD-6/AD-7's deferred "room assignment" question is now answered by
`handoff`'s message contract (see above). `CreateRoom` also calls
`Client.NotifyRoomCreated` so authsvc learns the new room's address; this
never blocks or fails room creation itself. On success, `onConnect`
continues into `conn.ReadLoop` for whatever comes next (actual gameplay
message routing — not built yet, a separate task). Independently of all
of this, `Registrar` keeps the process's own liveness lease renewed in
etcd for as long as roomsvc runs. `Registrar`, `RoomManager`, the
`authclient.httpClient` it wraps, and now the handoff handshake are all
constructed and running end-to-end in `main.go`.

## Non-class files

Two files have no struct/class to diagram — they hold package-level
functions and constants instead. Listed here so every file in the package
is accounted for; see their docs in
`cabo-bmad/_bmad-output/implementation-artifacts/architecture/` for details.

```mermaid
classDiagram
    class room_code {
        <<internal/roomsvc/game/room_code.go>>
        -roomCodeAlphabet string
        -roomCodeLength int
        -generateRoomCode() string
    }

    class rules {
        <<internal/roomsvc/game/rules.go>>
        +MaxPlayersPerRoom int
    }

    GameRoom ..> room_code : NewGameRoom calls generateRoomCode
    GameRoom ..> rules : NewGameRoom / JoinRoom validate against MaxPlayersPerRoom
```
