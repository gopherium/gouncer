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
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var memoryKiB, timeCost uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memoryKiB, &timeCost, &threads); err != nil {
		return false
	}
	if timeCost < 1 || timeCost > maxArgonTime ||
		threads < 1 || threads > maxArgonThreads ||
		memoryKiB < 1 || memoryKiB > maxArgonMemoryKiB {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	if len(salt) == 0 || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, timeCost, memoryKiB, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
