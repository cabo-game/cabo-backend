package game

import (
	"crypto/rand"
	"fmt"
)

// roomCodeAlphabet excludes visually ambiguous characters (0/O, 1/I/L) so
// codes are easy to read aloud and type when a player shares one to invite
// others into their game.
const roomCodeAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

const roomCodeLength = 8

// generateRoomCode returns a random room code drawn from roomCodeAlphabet.
// It uses crypto/rand because a room code doubles as a join credential:
// anyone holding it can attempt to join the room, so codes must be
// unpredictable, not just random-looking.
//
// This does not check the code against other currently-live rooms — that
// requires a room registry, which does not exist yet. Until one does, treat
// generated codes as very unlikely, but not guaranteed, to be unique.
func generateRoomCode() string {
	alphabetLen := byte(len(roomCodeAlphabet))
	code := make([]byte, roomCodeLength)

	randomBytes := make([]byte, roomCodeLength)
	if _, err := rand.Read(randomBytes); err != nil {
		panic(fmt.Sprintf("roomsvc: failed to read random bytes for room code: %v", err))
	}

	for i, b := range randomBytes {
		code[i] = roomCodeAlphabet[b%alphabetLen]
	}

	return string(code)
}
