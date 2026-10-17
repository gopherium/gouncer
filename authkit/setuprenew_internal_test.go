// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ageCode moves the modification time of the file at path two hours back, past the default lifetime.
func ageCode(t *testing.T, path string) {
	t.Helper()
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("ageing %s: %v", path, err)
	}
}

// expiredSetup returns an internal Setup whose code file holds an expired private code, and that code.
func expiredSetup(t *testing.T) (*Setup, string) {
	t.Helper()
	s := internalSetup(t)
	code := writeCode(t, s.cfg.CodeFile)
	ageCode(t, s.cfg.CodeFile)
	return s, code
}

// burnedSetup returns an internal Setup whose code file holds a live code at mode 0640, and that code.
func burnedSetup(t *testing.T) (*Setup, string) {
	t.Helper()
	s := internalSetup(t)
	code := writeCode(t, s.cfg.CodeFile)
	if err := os.Chmod(s.cfg.CodeFile, 0o640); err != nil {
		t.Fatalf("os.Chmod() error = %v", err)
	}
	return s, code
}

// beside returns another Setup over the code file of s, as a second copy of the binary holds.
func beside(s *Setup) *Setup {
	return NewSetup(SetupConfig{CodeFile: s.cfg.CodeFile, Role: "owner", Logger: slog.New(slog.DiscardHandler)})
}

// codeLog keeps every code an OnCode hook receives.
type codeLog struct {
	// mu guards codes.
	mu sync.Mutex
	// codes are the codes received, in order.
	codes []string
}

// hand keeps one code.
func (l *codeLog) hand(code string, _ time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.codes = append(l.codes, code)
}

// logCodes hands every code s announces to a fresh log and returns the log.
func logCodes(s *Setup) *codeLog {
	l := &codeLog{}
	s.cfg.OnCode = l.hand
	return l
}

// onTempFile runs act once, when s first creates a temporary file and before it does.
func onTempFile(s *Setup, act func()) {
	var once sync.Once
	s.createTemp = func(dir, pattern string) (*os.File, error) {
		once.Do(act)
		return os.CreateTemp(dir, pattern)
	}
}

// atReadBack runs act on the code file of s when s first reads back a fresh code it renamed into place.
func atReadBack(s *Setup, act func(path string)) {
	onTempFile(s, func() {
		looks := 0
		s.lstat = func(name string) (fs.FileInfo, error) {
			looks++
			if looks == 2 {
				act(name)
			}
			return os.Lstat(name)
		}
	})
}

// countTemps makes s count the temporary files it creates and returns the count.
func countTemps(s *Setup) *atomic.Int32 {
	var temps atomic.Int32
	s.createTemp = func(dir, pattern string) (*os.File, error) {
		temps.Add(1)
		return os.CreateTemp(dir, pattern)
	}
	return &temps
}

// failTemp makes every temporary file s creates fail.
func failTemp(s *Setup) {
	s.createTemp = func(string, string) (*os.File, error) { return nil, errors.New("no temporary file") }
}

// failNextLook makes the next look s takes at a path fail and every later one succeed.
func failNextLook(s *Setup) {
	s.lstat = func(string) (fs.FileInfo, error) {
		s.lstat = os.Lstat
		return nil, errors.New("no second look")
	}
}

// dropTempAtNextLook makes s remove the temporary file it creates when it next looks at a path.
func dropTempAtNextLook(s *Setup) {
	s.createTemp = func(dir, pattern string) (*os.File, error) {
		temp, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		s.lstat = func(name string) (fs.FileInfo, error) {
			_ = os.Remove(temp.Name())
			return os.Lstat(name)
		}
		return temp, nil
	}
}

// countGaps makes s count its looks and its temporary files that find the path without the file held there.
func countGaps(s *Setup, held fs.FileInfo) *int {
	var gaps int
	s.lstat = func(name string) (fs.FileInfo, error) {
		info, err := os.Lstat(name)
		if err != nil {
			gaps++
		}
		return info, err
	}
	onTempFile(s, func() {
		if now, err := os.Lstat(s.cfg.CodeFile); err != nil || !os.SameFile(held, now) {
			gaps++
		}
	})
	return &gaps
}

// codeAt returns the code the file at path holds, ending the test when it cannot be read.
func codeAt(t *testing.T, path string) string {
	t.Helper()
	return strings.TrimSpace(heldText(t, path))
}

// lstatOf returns what Lstat sees at path, ending the test when it sees nothing there.
func lstatOf(t *testing.T, path string) fs.FileInfo {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("os.Lstat() error = %v", err)
	}
	return info
}

// staleRead returns the code s reads at its code file, ending the test unless that code is stale.
func staleRead(t *testing.T, s *Setup) heldCode {
	t.Helper()
	held, err := s.readCode()
	if err != nil || !s.stale(held) {
		t.Fatalf("readCode() stale = %t, error = %v, want a stale code", s.stale(held), err)
	}
	return held
}

// swapIn renames a fresh private file holding content over path and returns what Lstat sees there.
func swapIn(t *testing.T, path, content string) fs.FileInfo {
	t.Helper()
	side := path + ".side"
	if err := os.WriteFile(side, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", side, err)
	}
	if err := os.Rename(side, path); err != nil {
		t.Fatalf("os.Rename() error = %v", err)
	}
	return lstatOf(t, path)
}

// swapInExpired renames a fresh expired code over path and returns that code.
func swapInExpired(t *testing.T, path string) string {
	t.Helper()
	code := rand.Text()
	swapIn(t, path, code+"\n")
	ageCode(t, path)
	return code
}

// swapInBurned renames a fresh live code over path and widens it to mode 0640.
func swapInBurned(t *testing.T, path string) {
	t.Helper()
	swapIn(t, path, rand.Text()+"\n")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatalf("os.Chmod() error = %v", err)
	}
}

// swapInAtTheSameTime renames a file holding no code over path, giving it the modification time of the file there.
func swapInAtTheSameTime(t *testing.T, path string) {
	t.Helper()
	at := lstatOf(t, path).ModTime()
	swapIn(t, path, "hello\n")
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatalf("os.Chtimes() error = %v", err)
	}
}

// makePrivate narrows the file at path to mode 0600 and returns no code.
func makePrivate(t *testing.T, path string) string {
	t.Helper()
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("os.Chmod() error = %v", err)
	}
	return ""
}

// removeFile removes the file at path and returns no code.
func removeFile(t *testing.T, path string) string {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatalf("os.Remove() error = %v", err)
	}
	return ""
}

// plantDir makes a folder at path.
func plantDir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}
}

// wantUnmoved fails t unless the file seen before still sits at path.
func wantUnmoved(t *testing.T, path string, before fs.FileInfo) {
	t.Helper()
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) {
		t.Errorf("the path holds another file or none (%v), want the file seen before kept", err)
	}
}

// lockOf returns the path of the lock file beside the code file at path.
func lockOf(path string) string {
	return path + ".lock"
}

// wantFolderHolds fails t unless the folder holding path holds exactly names, in order.
func wantFolderHolds(t *testing.T, path string, names ...string) {
	t.Helper()
	if got := folderNames(t, path); !slices.Equal(got, names) {
		t.Errorf("the folder holds %q, want %q", got, names)
	}
}

// renewedFolder returns names with the lock file beside path added where a renewal keeps one.
func renewedFolder(path string, names ...string) []string {
	if lockKept {
		names = append(names, filepath.Base(lockOf(path)))
	}
	return names
}

// wantOnlyFreshCode fails t unless the folder holds only the code file and any lock, a private code other than old.
func wantOnlyFreshCode(t *testing.T, path, old string) string {
	t.Helper()
	wantFolderHolds(t, path, renewedFolder(path, filepath.Base(path))...)
	if mode := lstatOf(t, path).Mode(); mode != 0o600 {
		t.Errorf("the code file mode = %v, want a regular file at 0600", mode)
	}
	code := codeAt(t, path)
	if code == old || !isCode(code) {
		t.Errorf("the code file holds %q, want a fresh code in place of %q", code, old)
	}
	return code
}

// logText sends what s logs to a fresh buffer as text and returns the buffer.
func logText(s *Setup) *bytes.Buffer {
	logged := &bytes.Buffer{}
	s.cfg.Logger = slog.New(slog.NewTextHandler(logged, nil))
	return logged
}

// wantWarning fails t unless the log holds a warning naming path.
func wantWarning(t *testing.T, logged *bytes.Buffer, path string) {
	t.Helper()
	if !strings.Contains(logged.String(), "level=WARN") || !strings.Contains(logged.String(), "path="+path) {
		t.Errorf("the log reads %q, want a warning naming %s", logged.String(), path)
	}
}

// TestChecksAtOnceRenewAnExpiredCodeOnce checks that racing checks of one expired code write and announce one code.
func TestChecksAtOnceRenewAnExpiredCodeOnce(t *testing.T) {
	t.Parallel()

	const racers = 8
	s, old := expiredSetup(t)
	handed := logCodes(s)
	temps := countTemps(s)
	var opened sync.WaitGroup
	opened.Add(racers)
	var opens atomic.Int32
	s.openFile = func(name string) (*os.File, error) {
		file, err := os.Open(name)
		if opens.Add(1) <= racers {
			opened.Done()
			opened.Wait()
		}
		return file, err
	}
	answers := make([]error, racers)
	var wg sync.WaitGroup
	for i := range racers {
		wg.Go(func() { answers[i] = s.Check(t.Context(), nil, old) })
	}
	wg.Wait()

	for i, err := range answers {
		if !errors.Is(err, ErrSetupCodeExpired) {
			t.Errorf("Check() %d error = %v, want ErrSetupCodeExpired", i, err)
		}
	}
	fresh := wantOnlyFreshCode(t, s.cfg.CodeFile, old)
	if want := slices.Repeat([]string{fresh}, racers); !slices.Equal(handed.codes, want) {
		t.Errorf("OnCode received %q, want the code the file holds, %q, each of the %d times", handed.codes, fresh,
			racers)
	}
	if written := temps.Load(); written != 1 {
		t.Errorf("the racing checks wrote %d fresh codes, want one", written)
	}
}

// TestRenewCodeAnswersTheCodeAnotherSetupRenewed checks a stale read whose code another Setup renewed first.
func TestRenewCodeAnswersTheCodeAnotherSetupRenewed(t *testing.T) {
	t.Parallel()

	first, old := expiredSetup(t)
	second := beside(first)
	stale := staleRead(t, second)
	if err := first.Check(t.Context(), nil, old); !errors.Is(err, ErrSetupCodeExpired) {
		t.Fatalf("Check() of the first Setup error = %v, want ErrSetupCodeExpired", err)
	}
	renewed := codeAt(t, first.cfg.CodeFile)
	temps := countTemps(second)

	held, err := second.renewCode(t.Context(), stale)

	if err != nil || held.code != renewed {
		t.Errorf("renewCode() = %q, %v, want the first renewal's %q and nil", held.code, err, renewed)
	}
	if now := codeAt(t, first.cfg.CodeFile); now != renewed {
		t.Errorf("the code file holds %q, want the first renewal's %q kept", now, renewed)
	}
	if written := temps.Load(); written != 0 {
		t.Errorf("renewCode() wrote %d temporary files, want none", written)
	}
}

// TestCheckKeepsTheExpiredCodeWhenTheRenewalFails checks that a renewal failing before it publishes keeps the old file.
func TestCheckKeepsTheExpiredCodeWhenTheRenewalFails(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		words string
		fail  func(s *Setup)
	}{
		{"no temporary file", "write the setup code at ", failTemp},
		{"the second look fails once", "read the setup code at ", func(s *Setup) {
			onTempFile(s, func() { failNextLook(s) })
		}},
		{"the fresh file vanishes before it takes the place", "write the setup code at ", dropTempAtNextLook},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, old := expiredSetup(t)
			path := s.cfg.CodeFile
			before := lstatOf(t, path)
			tt.fail(s)

			err := s.Check(t.Context(), nil, old)

			if !strings.Contains(fmt.Sprint(err), tt.words+path) {
				t.Errorf("Check() error = %v, want the failure saying %s", err, tt.words+path)
			}
			wantUnmoved(t, path, before)
			s.createTemp, s.lstat = os.CreateTemp, os.Lstat
			if err := s.Check(t.Context(), nil, old); !errors.Is(err, ErrSetupCodeExpired) {
				t.Errorf("Check() after the failure error = %v, want ErrSetupCodeExpired", err)
			}
			fresh := wantOnlyFreshCode(t, path, old)
			if err := s.Check(t.Context(), nil, fresh); err != nil {
				t.Errorf("Check() of the fresh code error = %v, want nil", err)
			}
		})
	}
}

// TestCheckReplacesABurnedCodeInOneStep checks that the burned file holds the path until the fresh code takes it.
func TestCheckReplacesABurnedCodeInOneStep(t *testing.T) {
	t.Parallel()

	s, old := burnedSetup(t)
	logged := logText(s)
	gaps := countGaps(s, lstatOf(t, s.cfg.CodeFile))

	err := s.Check(t.Context(), nil, old)

	if !errors.Is(err, ErrSetupCodeExpired) {
		t.Errorf("Check() error = %v, want ErrSetupCodeExpired", err)
	}
	if *gaps != 0 {
		t.Errorf("the renewal found the burned file gone %d times, want it at the path until the fresh code", *gaps)
	}
	wantOnlyFreshCode(t, s.cfg.CodeFile, old)
	wantWarning(t, logged, s.cfg.CodeFile)
}

// TestRenewCodeRenewsWhatTheFileHoldsByThen checks a burned read whose file changed before the renewal began.
func TestRenewCodeRenewsWhatTheFileHoldsByThen(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		change func(t *testing.T, path string) string
		warns  bool
	}{
		{"the burned file made private again", makePrivate, true},
		{"another expired code in its place", swapInExpired, true},
		{"the file removed", removeFile, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, old := burnedSetup(t)
			logged := logText(s)
			stale := staleRead(t, s)
			other := tt.change(t, s.cfg.CodeFile)

			held, err := s.renewCode(t.Context(), stale)

			if err != nil {
				t.Fatalf("renewCode() error = %v, want nil", err)
			}
			fresh := wantOnlyFreshCode(t, s.cfg.CodeFile, old)
			if held.code != fresh || fresh == other {
				t.Errorf("renewCode() = %q with the file holding %q, want a fresh code in place of %q", held.code,
					fresh, other)
			}
			if tt.warns {
				wantWarning(t, logged, s.cfg.CodeFile)
			}
		})
	}
}

// TestRenewCodeLeavesAFileThatTookTheCodesPlace checks a stale read whose file a file holding no code replaced.
func TestRenewCodeLeavesAFileThatTookTheCodesPlace(t *testing.T) {
	t.Parallel()

	s, _ := expiredSetup(t)
	stale := staleRead(t, s)
	put := swapIn(t, s.cfg.CodeFile, "hello\n")
	temps := countTemps(s)

	_, err := s.renewCode(t.Context(), stale)

	if !errors.Is(err, ErrSetupCodeFile) {
		t.Errorf("renewCode() error = %v, want ErrSetupCodeFile", err)
	}
	wantUnmoved(t, s.cfg.CodeFile, put)
	if held := heldText(t, s.cfg.CodeFile); held != "hello\n" {
		t.Errorf("the file at the path holds %q, want hello kept", held)
	}
	if written := temps.Load(); written != 0 {
		t.Errorf("renewCode() wrote %d temporary files, want none", written)
	}
}

// TestCheckLeavesWhatTookTheCodesPlaceDuringTheRenewal checks a path that changed while the fresh code was written.
func TestCheckLeavesWhatTookTheCodesPlaceDuringTheRenewal(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		words string
		put   func(t *testing.T, path string)
	}{
		{"a file holding no code", "does not hold a setup code", func(t *testing.T, path string) {
			swapIn(t, path, "hello\n")
		}},
		{"another expired code", "changed while it was renewed", func(t *testing.T, path string) {
			swapInExpired(t, path)
		}},
		{"a live code others can read", "changed while it was renewed", swapInBurned},
		{"a file holding no code at the same time", "does not hold a setup code", swapInAtTheSameTime},
		{"a folder", "is not a regular file", func(t *testing.T, path string) {
			removeFile(t, path)
			plantDir(t, path)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, old := expiredSetup(t)
			path := s.cfg.CodeFile
			var put fs.FileInfo
			onTempFile(s, func() {
				tt.put(t, path)
				put = lstatOf(t, path)
			})

			err := s.Check(t.Context(), nil, old)

			if !errors.Is(err, ErrSetupCodeFile) || !strings.Contains(err.Error(), tt.words) {
				t.Errorf("Check() error = %v, want ErrSetupCodeFile saying %s", err, tt.words)
			}
			wantUnmoved(t, path, put)
			wantFolderHolds(t, path, renewedFolder(path, filepath.Base(path))...)
		})
	}
}

// TestCheckKeepsACodeRewrittenInPlaceDuringTheRenewal checks a code file another writer rewrote during the renewal.
func TestCheckKeepsACodeRewrittenInPlaceDuringTheRenewal(t *testing.T) {
	t.Parallel()

	s, old := expiredSetup(t)
	handed := logCodes(s)
	other := rand.Text()
	onTempFile(s, func() {
		if err := os.WriteFile(s.cfg.CodeFile, []byte(other+"\n"), 0o600); err != nil {
			t.Fatalf("rewriting %s: %v", s.cfg.CodeFile, err)
		}
	})

	err := s.Check(t.Context(), nil, old)

	if !errors.Is(err, ErrSetupCodeExpired) {
		t.Errorf("Check() error = %v, want ErrSetupCodeExpired", err)
	}
	if held := wantOnlyFreshCode(t, s.cfg.CodeFile, old); held != other {
		t.Errorf("the code file holds %q, want the code written in place, %q, kept", held, other)
	}
	if !slices.Equal(handed.codes, []string{other}) {
		t.Errorf("OnCode received %q, want once the code written in place, %q", handed.codes, other)
	}
}

// TestCheckWritesACodeWhenTheFileGoesDuringTheRenewal checks a code file removed while the fresh code was written.
func TestCheckWritesACodeWhenTheFileGoesDuringTheRenewal(t *testing.T) {
	t.Parallel()

	s, old := expiredSetup(t)
	onTempFile(s, func() { removeFile(t, s.cfg.CodeFile) })

	if err := s.Check(t.Context(), nil, old); !errors.Is(err, ErrSetupCodeExpired) {
		t.Errorf("Check() error = %v, want ErrSetupCodeExpired", err)
	}
	wantOnlyFreshCode(t, s.cfg.CodeFile, old)
}

// TestCheckRemovesARenewedCodeItCannotReadBack checks a renewal whose published code fails its read back.
func TestCheckRemovesARenewedCodeItCannotReadBack(t *testing.T) {
	t.Parallel()

	s, old := expiredSetup(t)
	onTempFile(s, func() { s.openFile = openWriteOnly })

	err := s.Check(t.Context(), nil, old)

	wantReadFailure(t, err, s.cfg.CodeFile)
	wantFolderHolds(t, s.cfg.CodeFile, renewedFolder(s.cfg.CodeFile)...)
}

// TestCheckKeepsACodeAnotherCopyPublishedBeforeTheReadBack checks a failed read back after another copy renamed over.
func TestCheckKeepsACodeAnotherCopyPublishedBeforeTheReadBack(t *testing.T) {
	t.Parallel()

	s, old := expiredSetup(t)
	var other string
	atReadBack(s, func(path string) {
		other = rand.Text()
		swapIn(t, path, other+"\n")
		s.openFile = openWriteOnly
	})

	err := s.Check(t.Context(), nil, old)

	wantReadFailure(t, err, s.cfg.CodeFile)
	if held := codeAt(t, s.cfg.CodeFile); held != other {
		t.Errorf("the code file holds %q, want the other copy's live %q kept", held, other)
	}
}

// TestCheckRefusesARenewedCodeOthersCanStillRead checks a renewal whose published file the system leaves readable.
func TestCheckRefusesARenewedCodeOthersCanStillRead(t *testing.T) {
	t.Parallel()

	s, old := expiredSetup(t)
	atReadBack(s, func(path string) {
		if err := os.Chmod(path, 0o666); err != nil {
			t.Fatalf("os.Chmod() error = %v", err)
		}
	})

	err := s.Check(t.Context(), nil, old)

	if !errors.Is(err, ErrSetupCodeFile) || !strings.Contains(err.Error(), "cannot keep") {
		t.Errorf("Check() error = %v, want ErrSetupCodeFile saying the system cannot keep the file private", err)
	}
	wantFolderHolds(t, s.cfg.CodeFile, renewedFolder(s.cfg.CodeFile)...)
}

// TestRenewCodeKeepsAStaleCodeItCannotReadAgain checks a renewal whose second read of the code file fails.
func TestRenewCodeKeepsAStaleCodeItCannotReadAgain(t *testing.T) {
	t.Parallel()

	s, old := expiredSetup(t)
	stale := staleRead(t, s)
	s.openFile = openWriteOnly

	_, err := s.renewCode(t.Context(), stale)

	wantReadFailure(t, err, s.cfg.CodeFile)
	if held := codeAt(t, s.cfg.CodeFile); held != old {
		t.Errorf("the code file holds %q, want the expired %q kept", held, old)
	}
}

// TestCheckKeepsAnotherCodeItCannotRead checks a renewal that cannot read the code another writer put at the path.
func TestCheckKeepsAnotherCodeItCannotRead(t *testing.T) {
	t.Parallel()

	s, old := expiredSetup(t)
	var other string
	onTempFile(s, func() {
		other = rand.Text()
		swapIn(t, s.cfg.CodeFile, other+"\n")
		s.openFile = openWriteOnly
	})

	err := s.Check(t.Context(), nil, old)

	wantReadFailure(t, err, s.cfg.CodeFile)
	if held := codeAt(t, s.cfg.CodeFile); held != other {
		t.Errorf("the code file holds %q, want the other writer's %q kept", held, other)
	}
}

// TestRenewCodeReplacesALiveCodeOthersCanRead checks a stale read whose file a live code others can read replaced.
func TestRenewCodeReplacesALiveCodeOthersCanRead(t *testing.T) {
	t.Parallel()

	s, old := expiredSetup(t)
	logged := logText(s)
	stale := staleRead(t, s)
	swapInBurned(t, s.cfg.CodeFile)
	other := codeAt(t, s.cfg.CodeFile)

	held, err := s.renewCode(t.Context(), stale)

	if err != nil {
		t.Fatalf("renewCode() error = %v, want nil", err)
	}
	fresh := wantOnlyFreshCode(t, s.cfg.CodeFile, old)
	if held.code != fresh || fresh == other {
		t.Errorf("renewCode() = %q with the file holding %q, want a fresh code in place of %q", held.code, fresh, other)
	}
	wantWarning(t, logged, s.cfg.CodeFile)
}

// TestARenewalHoldsTheLockWhileItWrites checks that no other renewal of the Setup can start while one writes.
func TestARenewalHoldsTheLockWhileItWrites(t *testing.T) {
	t.Parallel()

	s, old := expiredSetup(t)
	var free bool
	onTempFile(s, func() {
		if s.renewing.TryLock() {
			free = true
			s.renewing.Unlock()
		}
	})

	if err := s.Check(t.Context(), nil, old); !errors.Is(err, ErrSetupCodeExpired) {
		t.Errorf("Check() error = %v, want ErrSetupCodeExpired", err)
	}
	if free {
		t.Errorf("the renewal lock was free while the fresh code was written, want it held")
	}
}
