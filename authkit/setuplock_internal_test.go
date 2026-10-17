// SPDX-License-Identifier: Apache-2.0

//go:build (unix && !aix && !solaris) || illumos

package authkit

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// renewChild names the environment variable that runs the test binary as a second copy renewing an expired code.
const renewChild = "AUTHKIT_SETUP_RENEW_CHILD"

// lockKept reports whether a renewal leaves a lock file beside the code file.
const lockKept = true

// takeLock opens the lock file at path as a renewal does and holds its exclusive lock until the file closes.
func takeLock(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		t.Fatalf("opening the lock file %s: %v", path, err)
	}
	t.Cleanup(func() { _ = file.Close() })
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatalf("locking %s: %v", path, err)
	}
	return file
}

// wantLockFree fails t unless another open file takes the exclusive lock on path at once.
func wantLockFree(t *testing.T, path string) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening the lock file %s: %v", path, err)
	}
	defer func() { _ = file.Close() }()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Errorf("taking the lock at %s error = %v, want it free once the renewal ended", path, err)
	}
}

// wantEmptyPrivate fails t unless the lock file at path is an empty regular file at exactly mode 0600.
func wantEmptyPrivate(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("os.Lstat() error = %v, want the lock file %s kept", err, path)
	}
	if info.Mode() != 0o600 || info.Size() != 0 {
		t.Errorf("the lock file is %v holding %d bytes, want an empty regular file at 0600", info.Mode(), info.Size())
	}
}

// readOnlyFolder makes the folder holding path readable but not writable until the test ends.
func readOnlyFolder(t *testing.T, path string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
	folder := filepath.Dir(path)
	if err := os.Chmod(folder, 0o500); err != nil {
		t.Fatalf("os.Chmod() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(folder, 0o700) })
}

// unopenableLock plants at lock an empty file that only its owner can read, skipping as root.
func unopenableLock(t *testing.T, lock string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
	if err := os.WriteFile(lock, nil, 0o400); err != nil {
		t.Fatalf("writing %s: %v", lock, err)
	}
}

// linkLock plants at lock a symbolic link to target.
func linkLock(t *testing.T, target, lock string) {
	t.Helper()
	if err := os.Symlink(target, lock); err != nil {
		t.Fatalf("os.Symlink() error = %v", err)
	}
}

// danglingLock plants at lock a link to a missing file in another folder and checks at the end that none was made.
func danglingLock(t *testing.T, lock string) {
	t.Helper()
	target := filepath.Join(t.TempDir(), "elsewhere")
	linkLock(t, target, lock)
	t.Cleanup(func() {
		if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("os.Lstat() of the link's target error = %v, want no file made there", err)
		}
	})
}

// linkedLock plants at lock a link to a file in another folder and checks at the end that it is unlocked and unchanged.
func linkedLock(t *testing.T, lock string) {
	t.Helper()
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(target, []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("writing %s: %v", target, err)
	}
	before := lstatOf(t, target)
	linkLock(t, target, lock)
	t.Cleanup(func() {
		wantLockFree(t, target)
		wantUnchanged(t, target, before)
	})
}

// wantUnchanged fails t unless path holds the file seen before at the same mode, size and modification time.
func wantUnchanged(t *testing.T, path string, before fs.FileInfo) {
	t.Helper()
	after, err := os.Lstat(path)
	if err != nil || !sameFile(before, after) {
		t.Errorf("os.Lstat() = %v, %v, want the file seen before at %s, unmodified", after, err, path)
		return
	}
	if after.Mode() != before.Mode() || after.Size() != before.Size() {
		t.Errorf("the file at %s is %v holding %d bytes, want %v holding %d", path, after.Mode(), after.Size(),
			before.Mode(), before.Size())
	}
}

// allStacks returns the stack of every goroutine.
func allStacks() string {
	buf := make([]byte, 1<<16)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return string(buf[:n])
		}
		buf = make([]byte, 2*len(buf))
	}
}

// waiters counts the goroutines whose stack names marker that wait in flock and those that wait on a mutex.
func waiters(marker string) (flocked, mutexed int) {
	for stack := range strings.SplitSeq(allStacks(), "\n\n") {
		switch {
		case !strings.Contains(stack, marker):
		case strings.Contains(stack, "syscall.Flock("):
			flocked++
		case strings.Contains(stack, "sync.(*Mutex).Lock("):
			mutexed++
		}
	}
	return flocked, mutexed
}

// inFlock reports whether a goroutine whose stack names marker waits in flock.
func inFlock(marker string) bool {
	flocked, _ := waiters(marker)
	return flocked > 0
}

// settledWaiters polls until n goroutines whose stack names marker wait in flock or on a mutex and answers both counts.
func settledWaiters(marker string, n int) (flocked, mutexed int) {
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		<-tick.C
		if flocked, mutexed = waiters(marker); flocked+mutexed == n {
			return flocked, mutexed
		}
	}
}

// waitsInFlock polls until a goroutine whose stack names marker waits in flock, answering false once done closes.
func waitsInFlock(marker string, done <-chan struct{}) bool {
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return false
		case <-tick.C:
		}
		if inFlock(marker) {
			return true
		}
	}
}

// asRenewChild runs Check of the expired code the environment names as a second copy, reporting whether it did.
func asRenewChild(t *testing.T) bool {
	t.Helper()
	old, path, found := strings.Cut(os.Getenv(renewChild), ":")
	if !found {
		return false
	}
	s := NewSetup(SetupConfig{CodeFile: path, Role: "owner", Logger: slog.New(slog.DiscardHandler)})
	handed := logCodes(s)
	temps := countTemps(s)
	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)
		err = s.Check(t.Context(), nil, old)
	}()
	if waitsInFlock("authkit.asRenewChild", done) {
		fmt.Println("waiting")
	}
	<-done
	fmt.Printf("expired %t\ncodes %s\ntemps %d\n", errors.Is(err, ErrSetupCodeExpired), strings.Join(handed.codes, " "),
		temps.Load())
	return true
}

// secondCopy is the test binary run again as a second copy renewing the code.
type secondCopy struct {
	// cmd is the running child.
	cmd *exec.Cmd
	// out reads what the child prints.
	out *bufio.Reader
	// errs keeps what the child writes to its standard error.
	errs *bytes.Buffer
}

// startSecondCopy runs the test of t again as a child that checks the expired code old at the code file of s.
func startSecondCopy(t *testing.T, s *Setup, old string) *secondCopy {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^"+t.Name()+"$", "-test.count=1")
	cmd.Env = append(os.Environ(), renewChild+"="+old+":"+s.cfg.CodeFile)
	errs := &bytes.Buffer{}
	cmd.Stderr = errs
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("cmd.StdoutPipe() error = %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the second copy: %v", err)
	}
	return &secondCopy{cmd: cmd, out: bufio.NewReader(out), errs: errs}
}

// line returns the next line the child prints, without its line break.
func (c *secondCopy) line() string {
	text, _ := c.out.ReadString('\n')
	return strings.TrimSuffix(text, "\n")
}

// rest waits for the child to end and returns what else it printed, ending the test when the child failed.
func (c *secondCopy) rest(t *testing.T) string {
	t.Helper()
	text, _ := io.ReadAll(c.out)
	if err := c.cmd.Wait(); err != nil {
		t.Fatalf("the second copy failed: %v\n%s%s", err, text, c.errs)
	}
	return string(text)
}

// TestARenewalWaitsForALockAnotherFileHolds checks that a renewal writes nothing while another file holds the lock.
func TestARenewalWaitsForALockAnotherFileHolds(t *testing.T) {
	t.Parallel()

	s, old := expiredSetup(t)
	held := takeLock(t, lockOf(s.cfg.CodeFile))
	temps := countTemps(s)
	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)
		err = s.Check(t.Context(), nil, old)
	}()

	waited := waitsInFlock(t.Name(), done)
	written := temps.Load()
	if closed := held.Close(); closed != nil {
		t.Fatalf("closing the held lock file: %v", closed)
	}
	<-done

	if !waited || written != 0 {
		t.Errorf("while another file held the lock the renewal waited %t and wrote %d temporary files, want it "+
			"waiting with none", waited, written)
	}
	if !errors.Is(err, ErrSetupCodeExpired) {
		t.Errorf("Check() once the lock was free error = %v, want ErrSetupCodeExpired", err)
	}
	wantOnlyFreshCode(t, s.cfg.CodeFile, old)
	if written := temps.Load(); written != 1 {
		t.Errorf("the renewal wrote %d temporary files once the lock was free, want one", written)
	}
}

// TestASecondSetupWaitsForTheRenewalAndKeepsItsCode checks a renewal by another Setup that starts while one writes.
func TestASecondSetupWaitsForTheRenewalAndKeepsItsCode(t *testing.T) {
	t.Parallel()

	first, old := expiredSetup(t)
	second := beside(first)
	firstCodes, secondCodes := logCodes(first), logCodes(second)
	secondTemps := countTemps(second)
	done := make(chan struct{})
	var secondErr error
	var waited bool
	onTempFile(first, func() {
		go func() {
			defer close(done)
			secondErr = second.Check(t.Context(), nil, old)
		}()
		waited = waitsInFlock(t.Name(), done)
	})

	err := first.Check(t.Context(), nil, old)
	<-done

	if !errors.Is(err, ErrSetupCodeExpired) || !errors.Is(secondErr, ErrSetupCodeExpired) {
		t.Errorf("the two Check() errors = %v and %v, want ErrSetupCodeExpired twice", err, secondErr)
	}
	if !waited {
		t.Errorf("the second Setup renewed while the first wrote, want it waiting for the lock")
	}
	renewed := wantOnlyFreshCode(t, first.cfg.CodeFile, old)
	for name, got := range map[string][]string{"first": firstCodes.codes, "second": secondCodes.codes} {
		if !slices.Equal(got, []string{renewed}) {
			t.Errorf("OnCode of the %s Setup received %q, want once the code the file holds, %q", name, got, renewed)
		}
	}
	if written := secondTemps.Load(); written != 0 {
		t.Errorf("the second Setup wrote %d temporary files, want none", written)
	}
}

// TestASecondCopyKeepsTheCodeTheFirstRenewed checks a second process renewing the same expired code meanwhile.
func TestASecondCopyKeepsTheCodeTheFirstRenewed(t *testing.T) {
	if asRenewChild(t) {
		return
	}
	t.Parallel()

	s, old := expiredSetup(t)
	handed := logCodes(s)
	var second *secondCopy
	var first string
	onTempFile(s, func() {
		second = startSecondCopy(t, s, old)
		first = second.line()
	})

	err := s.Check(t.Context(), nil, old)

	if !errors.Is(err, ErrSetupCodeExpired) {
		t.Errorf("Check() error = %v, want ErrSetupCodeExpired", err)
	}
	if second == nil {
		t.Fatalf("the renewal wrote no temporary file, want it to start the second copy while it held the lock")
	}
	reported := second.rest(t)
	fresh := wantOnlyFreshCode(t, s.cfg.CodeFile, old)
	if first != "waiting" {
		t.Errorf("the second copy printed %q first, want it waiting for the lock while the first renewal wrote", first)
	}
	if want := fmt.Sprintf("expired true\ncodes %s\ntemps 0\n", fresh); !strings.HasPrefix(reported, want) {
		t.Errorf("the second copy reported %q, want %q, the first copy's code and no code of its own", reported, want)
	}
	if !slices.Equal(handed.codes, []string{fresh}) {
		t.Errorf("OnCode of the first copy received %q, want once the code the file holds, %q", handed.codes, fresh)
	}
}

// TestCheckKeepsTheExpiredCodeWhenItCannotOpenTheLock checks a lock path the renewal cannot open.
func TestCheckKeepsTheExpiredCodeWhenItCannotOpenTheLock(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		block func(t *testing.T, lock string)
	}{
		{"a folder at the lock path", plantDir},
		{"a folder the process cannot write", readOnlyFolder},
		{"a file the process cannot open", unopenableLock},
		{"a link to a missing file elsewhere", danglingLock},
		{"a link to a file elsewhere", linkedLock},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, old := expiredSetup(t)
			path, lock := s.cfg.CodeFile, lockOf(s.cfg.CodeFile)
			before := lstatOf(t, path)
			tt.block(t, lock)
			names := folderNames(t, path)
			handed := logCodes(s)
			temps := countTemps(s)

			err := s.Check(t.Context(), nil, old)

			if !strings.Contains(fmt.Sprint(err), "take the setup code lock at "+lock) {
				t.Errorf("Check() error = %v, want the failure naming the lock file %s", err, lock)
			}
			wantUnmoved(t, path, before)
			if held := codeAt(t, path); held != old {
				t.Errorf("the code file holds %q, want the expired %q kept", held, old)
			}
			wantFolderHolds(t, path, names...)
			if written := temps.Load(); written != 0 || len(handed.codes) != 0 {
				t.Errorf("the failed renewal wrote %d temporary files and announced %q, want nothing", written,
					handed.codes)
			}
		})
	}
}

// openFiles returns how many files the process holds open.
func openFiles(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Fatalf("reading /dev/fd: %v", err)
	}
	return len(entries)
}

// TestCheckKeepsTheExpiredCodeWhenTheLockFails checks a renewal whose wait for the lock fails.
func TestCheckKeepsTheExpiredCodeWhenTheLockFails(t *testing.T) {
	failed := errors.New("no lock")
	was := flock
	flock = func(int, int) error { return failed }
	t.Cleanup(func() { flock = was })
	s, old := expiredSetup(t)
	lock := lockOf(s.cfg.CodeFile)
	temps := countTemps(s)
	open := openFiles(t)

	err := s.Check(t.Context(), nil, old)

	if !errors.Is(err, failed) || !strings.Contains(fmt.Sprint(err), "take the setup code lock at "+lock) {
		t.Errorf("Check() error = %v, want the failed lock naming %s", err, lock)
	}
	if held := codeAt(t, s.cfg.CodeFile); held != old {
		t.Errorf("the code file holds %q, want the expired %q kept", held, old)
	}
	if written := temps.Load(); written != 0 {
		t.Errorf("the failed renewal wrote %d temporary files, want none", written)
	}
	if now := openFiles(t); now > open {
		t.Errorf("the failed lock left %d more files open, want none", now-open)
	}
	wantEmptyPrivate(t, lock)
}

// TestTheLockFileStaysEmptyAndPrivate checks the lock file after a renewal and after the first account closes setup.
func TestTheLockFileStaysEmptyAndPrivate(t *testing.T) {
	s, old := expiredSetup(t)
	lock := lockOf(s.cfg.CodeFile)
	if err := s.Check(t.Context(), nil, old); !errors.Is(err, ErrSetupCodeExpired) {
		t.Fatalf("Check() error = %v, want ErrSetupCodeExpired", err)
	}
	wantEmptyPrivate(t, lock)
	wantLockFree(t, lock)

	_, err := s.CreateFirstAccount(t.Context(), &takingSetupStore{}, FirstAccount{
		Code: codeAt(t, s.cfg.CodeFile), Email: "maria@example.com", Name: "Maria Perez",
		Password: "correct horse battery",
	})

	if err != nil {
		t.Fatalf("CreateFirstAccount() error = %v, want nil", err)
	}
	wantFolderHolds(t, s.cfg.CodeFile, filepath.Base(lock))
	wantEmptyPrivate(t, lock)
}

// TestAFailedRenewalFreesTheLock checks that the lock is free once a renewal failed after it took the lock.
func TestAFailedRenewalFreesTheLock(t *testing.T) {
	s, old := expiredSetup(t)
	failTemp(s)

	err := s.Check(t.Context(), nil, old)

	if err == nil || errors.Is(err, ErrSetupCodeExpired) {
		t.Fatalf("Check() error = %v, want the failed renewal", err)
	}
	wantLockFree(t, lockOf(s.cfg.CodeFile))
}

// TestTheLockIsHeldThroughTheReadBack checks that the lock is still held while the renewal reads back its code.
func TestTheLockIsHeldThroughTheReadBack(t *testing.T) {
	s, old := expiredSetup(t)
	lock := lockOf(s.cfg.CodeFile)
	var held error
	atReadBack(s, func(string) {
		file, err := os.Open(lock)
		if err != nil {
			held = err
			return
		}
		defer func() { _ = file.Close() }()
		held = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	})

	if err := s.Check(t.Context(), nil, old); !errors.Is(err, ErrSetupCodeExpired) {
		t.Fatalf("Check() error = %v, want ErrSetupCodeExpired", err)
	}
	if !errors.Is(held, syscall.EWOULDBLOCK) {
		t.Errorf("taking the lock during the read back error = %v, want EWOULDBLOCK while the renewal holds it", held)
	}
}

// TestTheLockIsTakenInsideTheMutex checks that of racing renewals only the one holding the mutex waits for the lock.
func TestTheLockIsTakenInsideTheMutex(t *testing.T) {
	const racers = 8
	s, old := expiredSetup(t)
	held := takeLock(t, lockOf(s.cfg.CodeFile))
	answers := make([]error, racers)
	var wg sync.WaitGroup
	for i := range racers {
		wg.Go(func() { answers[i] = s.Check(t.Context(), nil, old) })
	}

	flocked, mutexed := settledWaiters(t.Name(), racers)
	if err := held.Close(); err != nil {
		t.Fatalf("closing the held lock file: %v", err)
	}
	wg.Wait()

	if flocked != 1 || mutexed != racers-1 {
		t.Errorf("%d checks waited in flock and %d on the mutex, want one in flock and %d on the mutex", flocked,
			mutexed, racers-1)
	}
	for i, err := range answers {
		if !errors.Is(err, ErrSetupCodeExpired) {
			t.Errorf("Check() %d error = %v, want ErrSetupCodeExpired", i, err)
		}
	}
}
