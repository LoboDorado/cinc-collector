// Package auth implements Cinc Collector's bearer-token authentication: a
// single shared-secret token model used identically across the ingest,
// query, and profile-store API surfaces. Tokens are random values; only
// their SHA-256 hash is ever stored or compared, so a database leak alone
// does not expose usable credentials.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// tokenBytes is the size of a generated token's random payload. 32 bytes
// (256 bits) of entropy is comfortably enough that brute-forcing a hash is
// infeasible.
const tokenBytes = 32

// GenerateToken returns a new random bearer token in printable form
// (prefixed so leaked tokens are greppable in logs/config) and its SHA-256
// hash, which is what gets stored.
func GenerateToken() (token, hash string, err error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("auth: generate token: %w", err)
	}
	token = "cinc_" + hex.EncodeToString(buf)
	return token, HashToken(token), nil
}

// HashToken returns the hex-encoded SHA-256 hash of token, the form stored
// in and compared against the tokens table.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
