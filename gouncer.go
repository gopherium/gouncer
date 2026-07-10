// SPDX-License-Identifier: Apache-2.0

// Package gouncer provides composable authentication primitives for Go.
package gouncer

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidEmail reports that an email address is not a plain valid address.
var ErrInvalidEmail = errors.New("gouncer: invalid email")

// ErrEmptyName reports that a user name is empty or only whitespace.
var ErrEmptyName = errors.New("gouncer: empty name")

// ErrWeakPassword reports that a password is shorter than the minimum length.
var ErrWeakPassword = errors.New("gouncer: password shorter than 12 characters")

// ErrNameTooLong reports that a name exceeds the maximum length.
var ErrNameTooLong = errors.New("gouncer: name longer than 256 characters")

// ErrPasswordTooLong reports that a password exceeds the maximum length.
var ErrPasswordTooLong = errors.New("gouncer: password longer than 1024 characters")

const (
	minPasswordLength = 12
	maxPasswordLength = 1024
	maxNameLength     = 256
)

// randRead fills a buffer with cryptographic randomness. It is a
// variable so failure paths stay testable.
var randRead = rand.Read

// User is an account holder with password credentials. Build one with [NewUser].
type User struct {
	ID           uuid.UUID
	Email        string
	Name         string
	PasswordHash string
	Disabled     bool
	CreatedAt    time.Time
}

// NewUser returns a validated [User] with a normalized email, a trimmed
// name, and the password stored as an argon2id hash. Invalid input
// returns one of the package's Err* sentinels.
func NewUser(email, name, password string) (User, error) {
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	if !isValidEmail(normalizedEmail) {
		return User{}, ErrInvalidEmail
	}
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return User{}, ErrEmptyName
	}
	if len([]rune(trimmedName)) > maxNameLength {
		return User{}, ErrNameTooLong
	}
	if passwordLength := len([]rune(password)); passwordLength < minPasswordLength {
		return User{}, ErrWeakPassword
	} else if passwordLength > maxPasswordLength {
		return User{}, ErrPasswordTooLong
	}
	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return User{}, fmt.Errorf("gouncer: generate id: %w", err)
	}
	return User{
		ID:           id,
		Email:        normalizedEmail,
		Name:         trimmedName,
		PasswordHash: hash,
		CreatedAt:    time.Now().UTC(),
	}, nil
}

// maxEmailLength is the RFC 5321 address-length limit.
const maxEmailLength = 254

// isValidEmail reports whether email is a single plain address without a
// display name.
func isValidEmail(email string) bool {
	if len(email) > maxEmailLength {
		return false
	}
	parsed, err := mail.ParseAddress(email)
	return err == nil && parsed.Address == email
}
