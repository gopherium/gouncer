// SPDX-License-Identifier: Apache-2.0

package gouncer

import (
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemoryKiB  = 19456
	argonTime       = 2
	argonThreads    = 1
	argonSaltLength = 16
	argonKeyLength  = 32
)

// The resource envelope [VerifyPassword] accepts. Parameters outside
// these bounds are rejected as malformed. The ceilings clear any real
// argon2id deployment while blocking hashes crafted to exhaust the process.
const (
	maxArgonMemoryKiB = 1 << 20 // 1 GiB
	maxArgonTime      = 32
	maxArgonThreads   = 16
)

// hashPassword returns password hashed with argon2id in PHC format.
func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLength)
	if _, err := randRead(salt); err != nil {
		return "", fmt.Errorf("gouncer: generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLength)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKiB, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword reports whether password matches the argon2id PHC hash.
// It never panics, a malformed or out-of-envelope hash never matches.
func VerifyPassword(hash, password string) bool {
	p, ok := parseArgonHash(hash)
	if !ok {
		return false
	}
	got := argon2.IDKey([]byte(password), p.salt, p.timeCost, p.memoryKiB, p.threads, uint32(len(p.key)))
	return subtle.ConstantTimeCompare(got, p.key) == 1
}

// argonParams carries the decoded fields of an argon2id PHC hash.
type argonParams struct {
	memoryKiB uint32
	timeCost  uint32
	threads   uint8
	salt      []byte
	key       []byte
}

// parseArgonHash decodes an argon2id PHC hash, reporting whether it is well
// formed and inside the accepted resource envelope.
func parseArgonHash(hash string) (argonParams, bool) {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return argonParams{}, false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return argonParams{}, false
	}
	var p argonParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memoryKiB, &p.timeCost, &p.threads); err != nil {
		return argonParams{}, false
	}
	if !withinArgonEnvelope(p) {
		return argonParams{}, false
	}
	salt, key, ok := decodeArgonFields(parts[4], parts[5])
	if !ok {
		return argonParams{}, false
	}
	p.salt, p.key = salt, key
	return p, true
}

// withinArgonEnvelope reports whether the cost parameters are inside the
// accepted resource envelope.
func withinArgonEnvelope(p argonParams) bool {
	return p.timeCost >= 1 && p.timeCost <= maxArgonTime &&
		p.threads >= 1 && p.threads <= maxArgonThreads &&
		p.memoryKiB >= 1 && p.memoryKiB <= maxArgonMemoryKiB
}

// decodeArgonFields decodes the salt and key of a PHC hash, rejecting an
// unreadable or empty one.
func decodeArgonFields(rawSalt, rawKey string) (salt, key []byte, ok bool) {
	salt, err := base64.RawStdEncoding.DecodeString(rawSalt)
	if err != nil || len(salt) == 0 {
		return nil, nil, false
	}
	key, err = base64.RawStdEncoding.DecodeString(rawKey)
	if err != nil || len(key) == 0 {
		return nil, nil, false
	}
	return salt, key, true
}
