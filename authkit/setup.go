// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gopherium/gouncer"
)

// DefaultSetupCodeLifetime is how long a setup code works when the config names no lifetime.
const DefaultSetupCodeLifetime = time.Hour

// ErrSetupDone reports a setup that is over, the store holding an account.
var ErrSetupDone = errors.New("authkit: setup already done")

// ErrSetupCodeInvalid reports a setup code that matches no live code in the code file.
var ErrSetupCodeInvalid = errors.New("authkit: setup code invalid")

// ErrSetupCodeExpired reports the right setup code after its lifetime ran out, a fresh code now in the file.
var ErrSetupCodeExpired = errors.New("authkit: setup code expired")

// ErrSetupCodeFile reports a code file path that holds something other than a setup code or cannot be kept private.
var ErrSetupCodeFile = errors.New("authkit: setup code file unusable")

// ErrSetupMisconfigured reports a setup config with a negative code lifetime or a first account role it cannot give.
var ErrSetupMisconfigured = errors.New("authkit: setup misconfigured")

// SetupStore persists accounts and creates the first one only while it holds none.
type SetupStore interface {
	gouncer.Store

	// HasUsers reports whether the store holds any account, disabled and unconfirmed ones included.
	HasUsers(ctx context.Context) (bool, error)

	// CreateFirstUser stores u while the store holds no account, or answers ErrSetupDone and stores nothing.
	CreateFirstUser(ctx context.Context, u gouncer.User) error
}

// SetupConfig parameterizes the one-time setup of a site's first account.
type SetupConfig struct {
	// CodeFile is the path of the file holding the setup code, in a folder only the owner can open.
	CodeFile string
	// CodeLifetime is how long a written code works. Zero applies DefaultSetupCodeLifetime. Negative is refused.
	CodeLifetime time.Duration
	// Role is the role the first account starts under. Empty is refused.
	Role string
	// Roles answers which roles exist. Nil checks none.
	Roles RoleSource
	// Capabilities answers what Role carries. Nil checks nothing.
	Capabilities CapabilitySource
	// Manages is the capability Role must carry, the one that manages the other accounts. Empty checks nothing.
	Manages string
	// Logger receives the lines naming the code file. Nil applies slog.Default.
	Logger *slog.Logger
	// OnCode receives each live code, at Prepare and after every rewrite. Nil calls nothing.
	OnCode func(code string, expiresAt time.Time)
}

// SetupState is what Prepare found or wrote, holding no code.
type SetupState struct {
	// Open reports whether the site still waits for its first account.
	Open bool
	// ExpiresAt is when the live code stops working, in UTC, while Open.
	ExpiresAt time.Time
}

// FirstAccount is a setup code with the details of the first account it creates, which name no role.
type FirstAccount struct {
	// Code is the setup code as typed.
	Code string `json:"code"`
	// Email is the first account's email.
	Email string `json:"email"`
	// Name is the first account's name.
	Name string `json:"name"`
	// Password is the first account's password as typed.
	Password string `json:"password"`
}

// Setup creates a site's first account for whoever shows the one-time code from the owner's file.
type Setup struct {
	// cfg is the config with its fallbacks applied.
	cfg SetupConfig
	// closed reports whether this process saw the store hold an account.
	closed atomic.Bool
	// renewing serializes the renewals of a stale code.
	renewing sync.Mutex
	// now reads the clock.
	now func() time.Time
	// lstat reads a path without following a symbolic link.
	lstat func(name string) (fs.FileInfo, error)
	// openFile opens a file for reading.
	openFile func(name string) (*os.File, error)
	// createTemp creates a fresh temporary file at mode 0600.
	createTemp func(dir, pattern string) (*os.File, error)
}

// NewSetup returns a Setup over cfg's code file and role.
func NewSetup(cfg SetupConfig) *Setup {
	if cfg.CodeLifetime == 0 {
		cfg.CodeLifetime = DefaultSetupCodeLifetime
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Setup{cfg: cfg, now: time.Now, lstat: os.Lstat, openFile: os.Open, createTemp: os.CreateTemp}
}

// Prepare keeps, writes or removes the code by whether store holds an account. Call it after plugins register roles.
func (s *Setup) Prepare(ctx context.Context, store SetupStore) (SetupState, error) {
	if err := s.refuseConfig(); err != nil {
		return SetupState{}, err
	}
	err := s.refuseClosed(ctx, store)
	if errors.Is(err, ErrSetupDone) {
		return SetupState{}, nil
	}
	if err != nil {
		return SetupState{}, err
	}
	held, err := s.ensureCode(ctx)
	if err != nil {
		return SetupState{}, err
	}
	s.announce(ctx, held)
	return SetupState{Open: true, ExpiresAt: held.expiresAt}, nil
}

// refuseConfig answers ErrSetupMisconfigured with its cause for a negative lifetime or a role the config cannot give.
func (s *Setup) refuseConfig() error {
	if s.cfg.CodeLifetime < 0 {
		return fmt.Errorf("%w: the code lifetime %v is negative", ErrSetupMisconfigured, s.cfg.CodeLifetime)
	}
	return s.refuseRole()
}

// refuseRole answers ErrSetupMisconfigured with its cause for a first account role the config cannot give.
func (s *Setup) refuseRole() error {
	role := s.cfg.Role
	if role == "" {
		return fmt.Errorf("%w: %w", ErrSetupMisconfigured, gouncer.ErrEmptyRole)
	}
	if !declared(s.cfg.Roles, role) {
		return fmt.Errorf("%w: the first account's role %q: %w", ErrSetupMisconfigured, role, ErrUnknownRole)
	}
	if !s.carriesManages(role) {
		return fmt.Errorf("%w: the first account's role %q does not carry %q", ErrSetupMisconfigured, role,
			s.cfg.Manages)
	}
	return nil
}

// carriesManages reports whether role carries the managing capability, every role passing when the config names none.
func (s *Setup) carriesManages(role string) bool {
	if s.cfg.Capabilities == nil || s.cfg.Manages == "" {
		return true
	}
	return slices.Contains(s.cfg.Capabilities.CapabilitiesOf(role), s.cfg.Manages)
}

// refuseClosed answers ErrSetupDone once this process or store saw an account, closing setup at the store's word.
func (s *Setup) refuseClosed(ctx context.Context, store SetupStore) error {
	if s.closed.Load() {
		return ErrSetupDone
	}
	if store == nil {
		return nil
	}
	held, err := store.HasUsers(ctx)
	if err != nil {
		return err
	}
	if !held {
		return nil
	}
	s.finish(ctx)
	return ErrSetupDone
}

// finish closes setup in this process and removes the code file, warning when it cannot.
func (s *Setup) finish(ctx context.Context) {
	s.closed.Store(true)
	if err := s.removeCode(); err != nil {
		s.cfg.Logger.WarnContext(ctx, "setup code file could not be removed", "path", s.cfg.CodeFile, "error", err)
	}
}

// announce logs where the live code waits and until when, and hands the code to OnCode when set.
func (s *Setup) announce(ctx context.Context, held heldCode) {
	s.cfg.Logger.InfoContext(ctx, "setup code waiting", "path", s.cfg.CodeFile, "expires_at", held.expiresAt)
	if s.cfg.OnCode != nil {
		s.cfg.OnCode(held.code, held.expiresAt)
	}
}

// Check answers nil while code matches the live code and store, when not nil, holds no account.
func (s *Setup) Check(ctx context.Context, store SetupStore, code string) error {
	if err := s.refuseClosed(ctx, store); err != nil {
		return err
	}
	held, err := s.readCode()
	if errors.Is(err, fs.ErrNotExist) {
		if s.closed.Load() {
			return ErrSetupDone
		}
		return ErrSetupCodeInvalid
	}
	if err != nil {
		return err
	}
	return s.judge(ctx, held, sameCode(normalizeCode(code), held.code))
}

// judge answers what a code that matched the held one or not opens, renewing a stale held code first.
func (s *Setup) judge(ctx context.Context, held heldCode, matched bool) error {
	stale := s.stale(held)
	if stale {
		fresh, err := s.renewCode(ctx, held)
		if err != nil {
			return err
		}
		s.announce(ctx, fresh)
	}
	switch {
	case !matched:
		return ErrSetupCodeInvalid
	case stale:
		return ErrSetupCodeExpired
	}
	return nil
}

// CreateFirstAccount creates the first account from first under the configured role and spends the code.
func (s *Setup) CreateFirstAccount(ctx context.Context, store SetupStore, first FirstAccount) (Account, error) {
	if store == nil {
		return Account{}, errors.New("authkit: create the first account: no store")
	}
	if err := s.refuseConfig(); err != nil {
		return Account{}, err
	}
	if err := s.Check(ctx, store, first.Code); err != nil {
		return Account{}, err
	}
	u, err := gouncer.NewUser(first.Email, first.Name, first.Password)
	if err != nil {
		return Account{}, err
	}
	u.Role = s.cfg.Role
	if err := store.CreateFirstUser(ctx, u); err != nil {
		return Account{}, err
	}
	s.finish(ctx)
	return newAccount(u), nil
}
