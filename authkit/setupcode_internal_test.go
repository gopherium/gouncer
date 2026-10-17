// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gopherium/gouncer"
)

// internalSetup returns a Setup over a code file in a fresh folder of the test, whose first account starts as owner.
func internalSetup(t *testing.T) *Setup {
	t.Helper()
	return NewSetup(SetupConfig{
		CodeFile: filepath.Join(t.TempDir(), "setup-code.txt"),
		Role:     "owner",
		Logger:   slog.New(slog.DiscardHandler),
	})
}

// writeCode writes a fresh code and a newline at path at mode 0600 and returns the code.
func writeCode(t *testing.T, path string) string {
	t.Helper()
	code := rand.Text()
	if err := os.WriteFile(path, []byte(code+"\n"), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return code
}

// heldText returns what the file at path holds, ending the test when it cannot be read.
func heldText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

// folderNames returns the names in the folder holding path.
func folderNames(t *testing.T, path string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("reading the folder of %s: %v", path, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// wantReadFailure fails t unless err is the failure to read the code file at path, not a refusal of what it holds.
func wantReadFailure(t *testing.T, err error, path string) {
	t.Helper()
	if !strings.Contains(fmt.Sprint(err), "read the setup code at "+path) {
		t.Errorf("error = %v, want the read failure naming %s", err, path)
	}
	if errors.Is(err, ErrSetupCodeFile) {
		t.Errorf("error = %v, want a read failure, not ErrSetupCodeFile", err)
	}
}

// openWriteOnly opens the file at name for writing only, so a read of it fails.
func openWriteOnly(name string) (*os.File, error) {
	return os.OpenFile(name, os.O_WRONLY, 0)
}

// TestPlaceCodeKeepsACodeAnotherCopyWrote checks that a code already at the final name wins over the fresh one.
func TestPlaceCodeKeepsACodeAnotherCopyWrote(t *testing.T) {
	t.Parallel()

	s := internalSetup(t)
	other := writeCode(t, s.cfg.CodeFile)

	held, err := s.placeCode()

	if err != nil || held.code != other {
		t.Errorf("placeCode() = %q, %v, want the other copy's %q and nil", held.code, err, other)
	}
	if names := folderNames(t, s.cfg.CodeFile); len(names) != 1 {
		t.Errorf("the folder holds %q, want only the code file", names)
	}
}

// TestCodeExpiresAtExactlyItsLifetime checks the expiry boundary under a fixed clock.
func TestCodeExpiresAtExactlyItsLifetime(t *testing.T) {
	t.Parallel()

	s := internalSetup(t)
	expiresAt := time.Date(2026, 10, 19, 11, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name    string
		now     time.Time
		expired bool
	}{
		{"one second inside its lifetime", expiresAt.Add(-time.Second), false},
		{"at exactly its lifetime", expiresAt, true},
		{"one second past its lifetime", expiresAt.Add(time.Second), true},
	} {
		s.now = func() time.Time { return tt.now }
		if got := s.expired(expiresAt); got != tt.expired {
			t.Errorf("expired() %s = %t, want %t", tt.name, got, tt.expired)
		}
	}
}

// TestNewSetupAppliesTheDefaultLifetime checks the lifetime a zero config receives.
func TestNewSetupAppliesTheDefaultLifetime(t *testing.T) {
	t.Parallel()

	if got := NewSetup(SetupConfig{}).cfg.CodeLifetime; got != time.Hour {
		t.Errorf("the lifetime of a zero config = %v, want one hour", got)
	}
}

// TestNewSetupAppliesTheDefaultLogger checks the logger a zero config receives.
func TestNewSetupAppliesTheDefaultLogger(t *testing.T) {
	t.Parallel()

	if got := NewSetup(SetupConfig{}).cfg.Logger; got != slog.Default() {
		t.Errorf("the logger of a zero config = %v, want slog.Default", got)
	}
}

// TestReadCodeRefusesAPathSwappedForALink checks that a link swapped in after the look is refused, not followed.
func TestReadCodeRefusesAPathSwappedForALink(t *testing.T) {
	t.Parallel()

	s := internalSetup(t)
	path := s.cfg.CodeFile
	folder := filepath.Dir(path)
	moved := filepath.Join(folder, "moved-code.txt")
	other := filepath.Join(folder, "other-code.txt")
	writeCode(t, path)
	writeCode(t, other)
	before := heldText(t, path)
	otherText := heldText(t, other)
	var swap sync.Once
	s.lstat = func(name string) (fs.FileInfo, error) {
		info, err := os.Lstat(name)
		swap.Do(func() {
			err = errors.Join(err, os.Rename(name, moved), os.Symlink(other, name))
		})
		return info, err
	}

	_, err := s.ensureCode(t.Context())

	if !errors.Is(err, ErrSetupCodeFile) {
		t.Errorf("ensureCode() error = %v, want ErrSetupCodeFile", err)
	}
	if target, err := os.Readlink(path); err != nil || target != other {
		t.Errorf("os.Readlink() = %q, %v, want the swapped link to %s left in place", target, err, other)
	}
	if heldText(t, moved) != before || heldText(t, other) != otherText {
		t.Errorf("a code file changed, want both kept byte for byte")
	}
}

// TestCheckReadsTheFlagAgainWhenTheFileIsGone checks a code file removed by a first account made meanwhile.
func TestCheckReadsTheFlagAgainWhenTheFileIsGone(t *testing.T) {
	t.Parallel()

	s := internalSetup(t)
	s.lstat = func(name string) (fs.FileInfo, error) {
		s.closed.Store(true)
		return os.Lstat(name)
	}

	if err := s.Check(t.Context(), nil, rand.Text()); !errors.Is(err, ErrSetupDone) {
		t.Errorf("Check() error = %v, want ErrSetupDone", err)
	}
}

// takingSetupStore is a store holding no account that takes every first account and keeps that it took one.
type takingSetupStore struct {
	stubUserStore
	// took reports whether a first account was taken.
	took bool
}

// HasUsers reports no account.
func (*takingSetupStore) HasUsers(context.Context) (bool, error) {
	return false, nil
}

// CreateFirstUser takes the account.
func (s *takingSetupStore) CreateFirstUser(context.Context, gouncer.User) error {
	s.took = true
	return nil
}

// TestCreateFirstAccountClosesSetupBeforeTheFileGoes checks that every read after the account write sees setup closed.
func TestCreateFirstAccountClosesSetupBeforeTheFileGoes(t *testing.T) {
	t.Parallel()

	s := internalSetup(t)
	code := writeCode(t, s.cfg.CodeFile)
	store := &takingSetupStore{}
	var readsAfter, openAfter int
	s.lstat = func(name string) (fs.FileInfo, error) {
		if store.took {
			readsAfter++
			if !s.closed.Load() {
				openAfter++
			}
		}
		return os.Lstat(name)
	}

	_, err := s.CreateFirstAccount(t.Context(), store, FirstAccount{
		Code: code, Email: "maria@example.com", Name: "Maria Perez", Password: "correct horse battery",
	})

	if err != nil {
		t.Fatalf("CreateFirstAccount() error = %v, want nil", err)
	}
	if readsAfter == 0 || openAfter != 0 {
		t.Errorf("after the account write the code file was read %d times, %d with setup open, want setup closed "+
			"before the file is touched", readsAfter, openAfter)
	}
	if _, err := os.Lstat(s.cfg.CodeFile); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("os.Lstat() error = %v, want the code file gone", err)
	}
}

// TestPlaceCodeLinksNothingItCouldNotWrite checks that a failed write leaves no code file and no temporary file.
func TestPlaceCodeLinksNothingItCouldNotWrite(t *testing.T) {
	t.Parallel()

	s := internalSetup(t)
	s.createTemp = func(dir, pattern string) (*os.File, error) {
		temp, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		return os.Open(temp.Name())
	}

	_, err := s.placeCode()

	if err == nil || !strings.Contains(err.Error(), "write the setup code at "+s.cfg.CodeFile) {
		t.Errorf("placeCode() error = %v, want the write failure naming the code file", err)
	}
	if names := folderNames(t, s.cfg.CodeFile); len(names) != 0 {
		t.Errorf("the folder holds %q, want nothing", names)
	}
}

// TestPlaceCodeReportsACodeItCannotReadBack checks a fresh code file whose read back fails.
func TestPlaceCodeReportsACodeItCannotReadBack(t *testing.T) {
	t.Parallel()

	s := internalSetup(t)
	s.openFile = openWriteOnly

	_, err := s.placeCode()

	wantReadFailure(t, err, s.cfg.CodeFile)
	if names := folderNames(t, s.cfg.CodeFile); len(names) != 0 {
		t.Errorf("the folder holds %q, want the unread code file gone", names)
	}
}

// TestPlaceCodeLeavesACodeAnotherCopyWroteThatItCannotRead checks a failed read back of a code another copy linked.
func TestPlaceCodeLeavesACodeAnotherCopyWroteThatItCannotRead(t *testing.T) {
	t.Parallel()

	s := internalSetup(t)
	other := writeCode(t, s.cfg.CodeFile)
	s.openFile = openWriteOnly

	_, err := s.placeCode()

	wantReadFailure(t, err, s.cfg.CodeFile)
	if held := heldText(t, s.cfg.CodeFile); held != other+"\n" {
		t.Errorf("the code file holds %q, want the other copy's %q kept", held, other)
	}
}

// TestPlaceCodeLeavesWhatReplacedItsFreshCode checks that a file put over the fresh code before its read back stays.
func TestPlaceCodeLeavesWhatReplacedItsFreshCode(t *testing.T) {
	t.Parallel()

	s := internalSetup(t)
	s.lstat = func(name string) (fs.FileInfo, error) {
		if err := os.WriteFile(name, []byte("hello\n"), 0o600); err != nil {
			return nil, err
		}
		return os.Lstat(name)
	}

	_, err := s.placeCode()

	if !errors.Is(err, ErrSetupCodeFile) {
		t.Errorf("placeCode() error = %v, want ErrSetupCodeFile", err)
	}
	if held := heldText(t, s.cfg.CodeFile); held != "hello\n" {
		t.Errorf("the file at the path holds %q, want hello kept", held)
	}
}

// TestReadCodeReportsAFileItCannotRead checks a code file whose read fails after it opened.
func TestReadCodeReportsAFileItCannotRead(t *testing.T) {
	t.Parallel()

	s := internalSetup(t)
	code := writeCode(t, s.cfg.CodeFile)
	s.openFile = openWriteOnly

	_, err := s.ensureCode(t.Context())

	wantReadFailure(t, err, s.cfg.CodeFile)
	if held := heldText(t, s.cfg.CodeFile); held != code+"\n" {
		t.Errorf("the code file holds %q, want %q kept", held, code)
	}
}

// TestAWrittenFileOthersCanStillReadIsRefused checks a system that cannot keep the fresh file private.
func TestAWrittenFileOthersCanStillReadIsRefused(t *testing.T) {
	t.Parallel()

	s := internalSetup(t)
	s.lstat = func(name string) (fs.FileInfo, error) {
		if _, err := os.Lstat(name); err == nil {
			if err := os.Chmod(name, 0o666); err != nil {
				return nil, err
			}
		}
		return os.Lstat(name)
	}

	_, err := s.Prepare(t.Context(), nil)

	if !errors.Is(err, ErrSetupCodeFile) || !strings.Contains(err.Error(), "cannot keep") {
		t.Errorf("Prepare() error = %v, want ErrSetupCodeFile saying the system cannot keep the file private", err)
	}
	if names := folderNames(t, s.cfg.CodeFile); len(names) != 0 {
		t.Errorf("the folder holds %q, want the code file gone", names)
	}
}
