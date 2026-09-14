package game

// Game rules and constants for roomsvc. These mirror decisions recorded in
// the shared BMAD project docs (../cabo-bmad/docs/cabo.md) — do not change
// a value here without updating that source first.

// MaxPlayersPerRoom is the maximum number of players allowed in one
// GameRoom. See cabo-bmad/docs/cabo.md, "Decisions".
const MaxPlayersPerRoom = 4

// CardsPerPlayerAtStart is the size of each player's opening hand, dealt
// by GameRoom.StartGame. See cabo-bmad/docs/cabo.md, "Decisions".
const CardsPerPlayerAtStart = 4

// InitialPeekCount is how many of their own cards a player sees once, via
// GameRoom.ViewInitialCards, at the start of a round. Cabo players do not
// know their hand otherwise — see cabo-bmad/docs/cabo.md, "Core loop".
const InitialPeekCount = 2
