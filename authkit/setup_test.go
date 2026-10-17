// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

var _ authkit.SetupStore = (*testkit.Store)(nil)

// codePattern matches one setup code, 26 characters from A to Z and 2 to 7.
var codePattern = regexp.MustCompile(`^[A-Z2-7]{26}$`)

// setupRoles returns a role table where owner manages the other accounts and author does not.
func setupRoles() roleTable {
	return roleTable{"owner": {"manage_users"}, "author": {"posts.write"}}
}

// setupConfig returns the config of a site whose code file is at path and whose first account starts as owner.
func setupConfig(path string) authkit.SetupConfig {
	return authkit.SetupConfig{
		CodeFile:     path,
		Role:         "owner",
		Roles:        setupRoles(),
		Capabilities: setupRoles(),
		Manages:      "manage_users",
		Logger:       slog.New(slog.DiscardHandler),
	}
}

// newSetup returns a Setup over the code file at path whose first account starts as owner.
func newSetup(path string) *authkit.Setup {
	return authkit.NewSetup(setupConfig(path))
}

// handedCodes keeps every code and expiry an OnCode hook receives.
type handedCodes struct {
	// mu guards the lists.
	mu sync.Mutex
	// codes are the codes received, in order.
	codes []string
	// at are the expiries received, in order.
	at []time.Time
}

// hand keeps one code and its expiry.
func (h *handedCodes) hand(code string, expiresAt time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.codes = append(h.codes, code)
	h.at = append(h.at, expiresAt)
}

// observedSetup returns a Setup over path whose log lines land in the buffer and whose codes land in the list.
func observedSetup(path string) (*authkit.Setup, *bytes.Buffer, *handedCodes) {
	logged := &bytes.Buffer{}
	handed := &handedCodes{}
	cfg := setupConfig(path)
	cfg.Logger = slog.New(slog.NewTextHandler(logged, nil))
	cfg.OnCode = handed.hand
	return authkit.NewSetup(cfg), logged, handed
}

// codePath returns the path of a code file in a fresh folder of the test.
func codePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "setup-code.txt")
}

// readFile returns what the file at path holds, ending the test when it cannot be read.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

// fileCode returns the code the file at path holds, ending the test when it cannot be read.
func fileCode(t *testing.T, path string) string {
	t.Helper()
	return strings.TrimSpace(readFile(t, path))
}

// plantFile writes content at path under exactly mode.
func plantFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("planting %s: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("setting the mode of %s: %v", path, err)
	}
}

// plantCode writes a fresh code and a newline at path under exactly mode and returns the code.
func plantCode(t *testing.T, path string, mode os.FileMode) string {
	t.Helper()
	code := rand.Text()
	plantFile(t, path, code+"\n", mode)
	return code
}

// plantLink puts at path a symbolic link to a private code file beside it.
func plantLink(t *testing.T, path string) {
	t.Helper()
	target := filepath.Join(filepath.Dir(path), "other-code.txt")
	plantCode(t, target, 0o600)
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("os.Symlink() error = %v", err)
	}
}

// plantFolder puts a folder at path.
func plantFolder(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}
}

// expire moves the modification time of the file at path a minute past the default lifetime.
func expire(t *testing.T, path string) {
	t.Helper()
	old := time.Now().Add(-authkit.DefaultSetupCodeLifetime - time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("ageing %s: %v", path, err)
	}
}

// needsFileModes skips t when the test runs as root.
func needsFileModes(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
}

// lockFolder makes the folder holding path readable but not writable until the test ends.
func lockFolder(t *testing.T, path string) {
	t.Helper()
	folder := filepath.Dir(path)
	if err := os.Chmod(folder, 0o500); err != nil {
		t.Fatalf("os.Chmod() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(folder, 0o700) })
}

// modeOf returns the mode of what sits at path, read without following a link.
func modeOf(t *testing.T, path string) fs.FileMode {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("reading the mode of %s: %v", path, err)
	}
	return info.Mode()
}

// absent fails t unless nothing sits at path.
func absent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("os.Lstat(%s) error = %v, want nothing there", path, err)
	}
}

// wantPrivateCode fails t unless path is a regular file at exactly 0600 holding one code and a newline, returning it.
func wantPrivateCode(t *testing.T, path string) string {
	t.Helper()
	held := readFile(t, path)
	code := strings.TrimSuffix(held, "\n")
	if held != code+"\n" {
		t.Errorf("the code file holds %q, want the code followed by a newline", held)
	}
	if !codePattern.MatchString(code) {
		t.Errorf("the code file holds %q, want one code of 26 characters from A to Z and 2 to 7", code)
	}
	if mode := modeOf(t, path); mode != 0o600 {
		t.Errorf("the code file mode = %v, want a regular file at 0600", mode)
	}
	return code
}

// wantFreshCode fails t unless path holds a private code other than old, returning it.
func wantFreshCode(t *testing.T, path, old string) string {
	t.Helper()
	code := wantPrivateCode(t, path)
	if code == old {
		t.Errorf("the code file still holds %q, want a fresh code", old)
	}
	return code
}

// wantOpenUntil fails t unless state is open until want, in UTC.
func wantOpenUntil(t *testing.T, state authkit.SetupState, want time.Time) {
	t.Helper()
	if !state.Open {
		t.Errorf("Prepare() = %+v, want open", state)
	}
	if !state.ExpiresAt.Equal(want) {
		t.Errorf("Prepare() expires at %v, want %v", state.ExpiresAt, want)
	}
	if state.ExpiresAt.Location() != time.UTC {
		t.Errorf("Prepare() expires at %v, want the time in UTC", state.ExpiresAt)
	}
}

// wantLogged fails t unless the log holds every part.
func wantLogged(t *testing.T, logged *bytes.Buffer, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(logged.String(), part) {
			t.Errorf("the log reads %q, want it to hold %s", logged.String(), part)
		}
	}
}

// wantNoCodeLogged fails t unless the log holds none of codes.
func wantNoCodeLogged(t *testing.T, logged *bytes.Buffer, codes ...string) {
	t.Helper()
	for _, code := range codes {
		if strings.Contains(logged.String(), code) {
			t.Errorf("the log reads %q, want it never to hold the code %s", logged.String(), code)
		}
	}
}

// wantMisconfigured fails t unless err is ErrSetupMisconfigured carrying cause, when not nil, and saying words.
func wantMisconfigured(t *testing.T, err, cause error, words string) {
	t.Helper()
	if !errors.Is(err, authkit.ErrSetupMisconfigured) {
		t.Errorf("error = %v, want ErrSetupMisconfigured", err)
	}
	if cause != nil && !errors.Is(err, cause) {
		t.Errorf("error = %v, want it to carry %v", err, cause)
	}
	if !strings.Contains(fmt.Sprint(err), words) {
		t.Errorf("error = %v, want it to say %s", err, words)
	}
}

// wantFailure fails t unless err says words and carries no error a client is answered with.
func wantFailure(t *testing.T, err error, words string) {
	t.Helper()
	if !strings.Contains(fmt.Sprint(err), words) {
		t.Errorf("error = %v, want a failure saying %s", err, words)
	}
	if _, _, mapped := authkit.ErrorResponseForAuthError(err); mapped {
		t.Errorf("error = %v, want none a client is answered with", err)
	}
}

// wantInPlace fails t unless the same file, link or folder seen before still sits at path.
func wantInPlace(t *testing.T, path string, before fs.FileInfo) {
	t.Helper()
	after, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("os.Lstat() error = %v, want %v kept in place", err, before.Mode())
	}
	if !os.SameFile(before, after) {
		t.Errorf("the path holds another file, want the one seen before kept in place")
	}
	if after.Mode() != before.Mode() {
		t.Errorf("the path mode = %v, want %v", after.Mode(), before.Mode())
	}
}

// addMaria stores Maria Perez as an account and returns it.
func addMaria(t *testing.T, store *testkit.Store) gouncer.User {
	t.Helper()
	return store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
}

// prepared runs Prepare with no store over a fresh code file and returns the Setup, the path and the code.
func prepared(t *testing.T) (*authkit.Setup, string, string) {
	t.Helper()
	path := codePath(t)
	setup := newSetup(path)
	if _, err := setup.Prepare(t.Context(), nil); err != nil {
		t.Fatalf("Prepare() error = %v, want nil", err)
	}
	return setup, path, fileCode(t, path)
}

// createdByShell stores Maria Perez through plain CreateUser, as the command line does.
func createdByShell(t *testing.T, store *testkit.Store) gouncer.User {
	t.Helper()
	u, err := gouncer.NewUser("maria@example.com", "Maria Perez", testPassword)
	if err != nil {
		t.Fatalf("gouncer.NewUser() error = %v", err)
	}
	if err := store.CreateUser(t.Context(), u); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	return u
}

// grouped returns code in groups of four joined by sep.
func grouped(code, sep string) string {
	var groups []string
	for start := 0; start < len(code); start += 4 {
		groups = append(groups, code[start:min(start+4, len(code))])
	}
	return strings.Join(groups, sep)
}

// TestPrepareWritesACodeOnlyTheOwnerReads checks the first code a site with an empty store receives.
func TestPrepareWritesACodeOnlyTheOwnerReads(t *testing.T) {
	t.Parallel()

	path := codePath(t)

	state, err := newSetup(path).Prepare(t.Context(), testkit.NewStore())

	if err != nil {
		t.Fatalf("Prepare() error = %v, want nil", err)
	}
	wantPrivateCode(t, path)
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("os.Lstat() error = %v, want nil", err)
	}
	wantOpenUntil(t, state, info.ModTime().Add(time.Hour))
}

// TestPrepareKeepsALiveCodeAcrossARestart checks that a second Setup over the same file keeps its code.
func TestPrepareKeepsALiveCodeAcrossARestart(t *testing.T) {
	t.Parallel()

	path := codePath(t)
	first, err := newSetup(path).Prepare(t.Context(), nil)
	if err != nil {
		t.Fatalf("the first Prepare() error = %v, want nil", err)
	}
	code := fileCode(t, path)

	again, err := newSetup(path).Prepare(t.Context(), nil)

	if err != nil {
		t.Fatalf("Prepare() after the restart error = %v, want nil", err)
	}
	if held := fileCode(t, path); held != code || !again.ExpiresAt.Equal(first.ExpiresAt) {
		t.Errorf("after the restart the file holds %q until %v, want %q until %v", held, again.ExpiresAt, code,
			first.ExpiresAt)
	}
}

// TestPrepareWritesAFreshCodeOnceTheOldOneExpired checks that an expired code makes way for a new private one.
func TestPrepareWritesAFreshCodeOnceTheOldOneExpired(t *testing.T) {
	t.Parallel()

	path := codePath(t)
	old := plantCode(t, path, 0o600)
	expire(t, path)

	state, err := newSetup(path).Prepare(t.Context(), nil)

	if err != nil {
		t.Fatalf("Prepare() error = %v, want nil", err)
	}
	wantFreshCode(t, path, old)
	if !state.Open {
		t.Errorf("Prepare() = %+v, want open", state)
	}
}

// TestPrepareLeavesAFileThatHoldsNoCode checks that a file at the path that holds no code stays byte for byte.
func TestPrepareLeavesAFileThatHoldsNoCode(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		content string
	}{
		{"a private file holding hello", "hello\n"},
		{"a private settings file", "MYAPP_DATABASE_URL=postgres://maria@example.com/myapp\n"},
		{"a code with more text after it", rand.Text() + "\nMYAPP_SITE_TITLE=myapp\n"},
		{"a code with more text past 64 bytes", rand.Text() + "\n" + strings.Repeat(" ", 64) + "MYAPP_SITE_TITLE=myapp\n"},
		{"a code padded with spaces past 64 bytes", rand.Text() + strings.Repeat(" ", 64) + "\n"},
		{"26 characters outside the code alphabet", strings.Repeat("0", 26) + "\n"},
		{"27 characters from the code alphabet", rand.Text() + "A\n"},
		{"a code in small letters", strings.ToLower(rand.Text()) + "\n"},
		{"a short word in capitals", "MYAPP\n"},
		{"a file of one mebibyte", strings.Repeat("a", 1<<20)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := codePath(t)
			plantFile(t, path, tt.content, 0o600)

			_, err := newSetup(path).Prepare(t.Context(), nil)

			if !errors.Is(err, authkit.ErrSetupCodeFile) {
				t.Errorf("Prepare() error = %v, want ErrSetupCodeFile", err)
			}
			if held := readFile(t, path); held != tt.content {
				t.Errorf("the file at the path changed to %d bytes, want its %d bytes kept", len(held), len(tt.content))
			}
		})
	}
}

// TestPrepareWithoutAStoreWritesACode checks that a site with no database yet receives a code.
func TestPrepareWithoutAStoreWritesACode(t *testing.T) {
	t.Parallel()

	path := codePath(t)

	state, err := newSetup(path).Prepare(t.Context(), nil)

	if err != nil || !state.Open {
		t.Fatalf("Prepare() = %+v, %v, want open and nil", state, err)
	}
	if code := fileCode(t, path); !codePattern.MatchString(code) {
		t.Errorf("the code file holds %q, want a setup code", code)
	}
}

// TestPrepareRemovesTheCodeOnceAnAccountExists checks that a store holding any account closes setup.
func TestPrepareRemovesTheCodeOnceAnAccountExists(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		plant func(t *testing.T, path string)
		left  func(t *testing.T, path string)
	}{
		{
			"a code file goes",
			func(t *testing.T, path string) { plantCode(t, path, 0o600) },
			absent,
		},
		{
			"a file that holds no code stays",
			func(t *testing.T, path string) { plantFile(t, path, "hello\n", 0o600) },
			func(t *testing.T, path string) {
				t.Helper()
				if held := readFile(t, path); held != "hello\n" {
					t.Errorf("the file at the path holds %q, want hello kept", held)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := testkit.NewStore()
			maria := addMaria(t, store)
			maria.Disabled = true
			store.Users[maria.ID] = maria
			path := codePath(t)
			tt.plant(t, path)

			state, err := newSetup(path).Prepare(t.Context(), store)

			if err != nil || state != (authkit.SetupState{}) {
				t.Errorf("Prepare() = %+v, %v, want closed and nil", state, err)
			}
			tt.left(t, path)
		})
	}
}

// TestPrepareReplacesACodeOthersCouldRead checks that a code the group or others could read is burned and replaced.
func TestPrepareReplacesACodeOthersCouldRead(t *testing.T) {
	t.Parallel()

	for _, mode := range []os.FileMode{0o640, 0o604} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()

			path := codePath(t)
			old := plantCode(t, path, mode)
			setup, logged, _ := observedSetup(path)

			state, err := setup.Prepare(t.Context(), nil)

			if err != nil || !state.Open {
				t.Fatalf("Prepare() = %+v, %v, want open and nil", state, err)
			}
			fresh := wantFreshCode(t, path, old)
			wantLogged(t, logged, "level=WARN", "path="+path)
			wantNoCodeLogged(t, logged, old, fresh)
		})
	}
}

// TestPrepareLeavesWhatIsNotARegularFile checks that a link or a folder at the path stays where it is.
func TestPrepareLeavesWhatIsNotARegularFile(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		plant func(t *testing.T, path string)
	}{
		{"a symbolic link to a private code file", plantLink},
		{"a folder", plantFolder},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := codePath(t)
			tt.plant(t, path)
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatalf("os.Lstat() error = %v", err)
			}

			_, err = newSetup(path).Prepare(t.Context(), nil)

			if !errors.Is(err, authkit.ErrSetupCodeFile) {
				t.Errorf("Prepare() error = %v, want ErrSetupCodeFile", err)
			}
			if !strings.Contains(fmt.Sprint(err), "is not a regular file, so it stays in place") {
				t.Errorf("Prepare() error = %v, want it to say the path is not a regular file", err)
			}
			wantInPlace(t, path, before)
		})
	}
}

// TestPrepareStopsWhenTheFolderIsMissing checks that a missing folder fails with an error naming the path.
func TestPrepareStopsWhenTheFolderIsMissing(t *testing.T) {
	t.Parallel()

	folder := filepath.Join(t.TempDir(), "state")
	path := filepath.Join(folder, "setup-code.txt")

	_, err := newSetup(path).Prepare(t.Context(), nil)

	wantFailure(t, err, "write the setup code at "+path)
	absent(t, folder)
}

// unusableRoles are first account roles the config of setupConfig cannot give, each with its cause and its words.
var unusableRoles = []struct {
	name  string
	role  string
	cause error
	words string
}{
	{"an empty role", "", gouncer.ErrEmptyRole, "empty role"},
	{"an undeclared role", "editor", authkit.ErrUnknownRole, `"editor"`},
	{"a role that does not manage the others", "author", nil,
		`the first account's role "author" does not carry "manage_users"`},
}

// TestPrepareRefusesARoleItCannotGive checks that a first role the config cannot give stops the start.
func TestPrepareRefusesARoleItCannotGive(t *testing.T) {
	t.Parallel()

	for _, tt := range unusableRoles {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := codePath(t)
			cfg := setupConfig(path)
			cfg.Role = tt.role

			_, err := authkit.NewSetup(cfg).Prepare(t.Context(), nil)

			wantMisconfigured(t, err, tt.cause, tt.words)
			absent(t, path)
		})
	}
}

// TestPrepareChecksOnlyWhatTheConfigNames checks that nil sources and an empty Manages refuse no role.
func TestPrepareChecksOnlyWhatTheConfigNames(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		shape func(cfg *authkit.SetupConfig)
	}{
		{"no role source", func(cfg *authkit.SetupConfig) {
			cfg.Roles, cfg.Capabilities, cfg.Role = nil, roleTable{"editor": {"manage_users"}}, "editor"
		}},
		{"no capability source", func(cfg *authkit.SetupConfig) { cfg.Capabilities, cfg.Role = nil, "author" }},
		{"no managing capability", func(cfg *authkit.SetupConfig) { cfg.Manages, cfg.Role = "", "author" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := codePath(t)
			cfg := setupConfig(path)
			tt.shape(&cfg)

			state, err := authkit.NewSetup(cfg).Prepare(t.Context(), nil)

			if err != nil || !state.Open {
				t.Fatalf("Prepare() = %+v, %v, want open and nil", state, err)
			}
			if code := fileCode(t, path); !codePattern.MatchString(code) {
				t.Errorf("the code file holds %q, want a setup code", code)
			}
		})
	}
}

// TestPrepareLogsThePathAndNeverTheCode checks the Info line and the OnCode hook.
func TestPrepareLogsThePathAndNeverTheCode(t *testing.T) {
	t.Parallel()

	path := codePath(t)
	setup, logged, handed := observedSetup(path)

	state, err := setup.Prepare(t.Context(), nil)

	if err != nil {
		t.Fatalf("Prepare() error = %v, want nil", err)
	}
	code := fileCode(t, path)
	wantLogged(t, logged, `msg="setup code waiting"`, "path="+path, "expires_at=")
	wantNoCodeLogged(t, logged, code)
	if !slices.Equal(handed.codes, []string{code}) {
		t.Errorf("OnCode received %q, want once %q", handed.codes, code)
	}
	if !slices.EqualFunc(handed.at, []time.Time{state.ExpiresAt}, time.Time.Equal) {
		t.Errorf("OnCode received the expiries %v, want once %v", handed.at, state.ExpiresAt)
	}
}

// TestPreparesAtOnceLeaveOneCode checks that copies starting together hand out the one code the file holds.
func TestPreparesAtOnceLeaveOneCode(t *testing.T) {
	t.Parallel()

	const copies = 8
	path := codePath(t)
	handed := &handedCodes{}
	answers := make([]error, copies)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range copies {
		cfg := setupConfig(path)
		cfg.OnCode = handed.hand
		setup := authkit.NewSetup(cfg)
		wg.Go(func() {
			<-start
			_, answers[i] = setup.Prepare(t.Context(), nil)
		})
	}
	close(start)
	wg.Wait()

	if err := errors.Join(answers...); err != nil {
		t.Fatalf("a Prepare() error = %v, want nil", err)
	}
	code := fileCode(t, path)
	if want := slices.Repeat([]string{code}, copies); !slices.Equal(handed.codes, want) {
		t.Errorf("OnCode received %q, want %q each of the %d times", handed.codes, code, copies)
	}
}

// TestPrepareReportsAStoreItCannotRead checks that a failed store read passes through and writes nothing.
func TestPrepareReportsAStoreItCannotRead(t *testing.T) {
	t.Parallel()

	path := codePath(t)
	store := testkit.NewStore()
	store.LookupErr = errLookupFailed

	_, err := newSetup(path).Prepare(t.Context(), store)

	if !errors.Is(err, errLookupFailed) {
		t.Errorf("Prepare() error = %v, want the lookup failure", err)
	}
	absent(t, path)
}

// TestPrepareStopsAtACodeFileItCannotOpen checks a code file whose mode keeps its owner from reading it.
func TestPrepareStopsAtACodeFileItCannotOpen(t *testing.T) {
	needsFileModes(t)
	t.Parallel()

	path := codePath(t)
	code := plantCode(t, path, 0o200)

	_, err := newSetup(path).Prepare(t.Context(), nil)

	wantFailure(t, err, "read the setup code at "+path)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("os.Chmod() error = %v", err)
	}
	if held := fileCode(t, path); held != code {
		t.Errorf("the code file holds %q, want %q kept", held, code)
	}
}

// TestPrepareWarnsWhenItCannotRemoveTheCode checks a set-up site whose code file the owner cannot read.
func TestPrepareWarnsWhenItCannotRemoveTheCode(t *testing.T) {
	needsFileModes(t)
	t.Parallel()

	path := codePath(t)
	plantCode(t, path, 0o200)
	store := testkit.NewStore()
	addMaria(t, store)
	setup, logged, _ := observedSetup(path)

	state, err := setup.Prepare(t.Context(), store)

	if err != nil || state.Open {
		t.Errorf("Prepare() = %+v, %v, want closed and nil", state, err)
	}
	wantLogged(t, logged, "level=WARN", "path="+path)
	if mode := modeOf(t, path); mode != 0o200 {
		t.Errorf("the code file mode = %v, want the file left in place", mode)
	}
}

// TestCheckAcceptsOnlyTheCodeInTheFile checks which inputs Check takes for the live code.
func TestCheckAcceptsOnlyTheCodeInTheFile(t *testing.T) {
	t.Parallel()

	setup, path, code := prepared(t)
	for _, tt := range []struct {
		name  string
		input string
		want  error
	}{
		{"the code", code, nil},
		{"the code in small letters", strings.ToLower(code), nil},
		{"the code in groups of four with spaces", grouped(code, " "), nil},
		{"the code with hyphens", grouped(code, "-"), nil},
		{"the code in groups of four with tabs", grouped(code, "\t"), nil},
		{"the code with a trailing line break", code + "\r\n", nil},
		{"another code", rand.Text(), authkit.ErrSetupCodeInvalid},
		{"the empty string", "", authkit.ErrSetupCodeInvalid},
		{"the first 25 characters", code[:25], authkit.ErrSetupCodeInvalid},
		{"the code plus one character", code + "A", authkit.ErrSetupCodeInvalid},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := setup.Check(t.Context(), nil, tt.input); !errors.Is(err, tt.want) {
				t.Errorf("Check(%q) error = %v, want %v", tt.input, err, tt.want)
			}
		})
	}
	if held := fileCode(t, path); held != code {
		t.Errorf("the code file holds %q after the checks, want %q kept", held, code)
	}
}

// TestCheckTellsAnExpiredCodeFromAWrongOne checks the answers over an expired code and the fresh code after them.
func TestCheckTellsAnExpiredCodeFromAWrongOne(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		input func(old string) string
		want  error
	}{
		{"the expired code", func(old string) string { return old }, authkit.ErrSetupCodeExpired},
		{"a wrong code", func(string) string { return rand.Text() }, authkit.ErrSetupCodeInvalid},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := codePath(t)
			setup, logged, handed := observedSetup(path)
			if _, err := setup.Prepare(t.Context(), nil); err != nil {
				t.Fatalf("Prepare() error = %v", err)
			}
			old := fileCode(t, path)
			expire(t, path)

			err := setup.Check(t.Context(), nil, tt.input(old))

			if !errors.Is(err, tt.want) {
				t.Errorf("Check() error = %v, want %v", err, tt.want)
			}
			fresh := wantFreshCode(t, path, old)
			if strings.Contains(logged.String(), "level=WARN") {
				t.Errorf("the log reads %q, want no warning for a private code", logged.String())
			}
			wantNoCodeLogged(t, logged, old, fresh)
			if err := setup.Check(t.Context(), nil, fresh); err != nil {
				t.Errorf("Check() of the fresh code error = %v, want nil", err)
			}
			if !slices.Equal(handed.codes, []string{old, fresh}) {
				t.Errorf("OnCode received %q, want %q, then %q", handed.codes, old, fresh)
			}
			if waiting := strings.Count(logged.String(), "setup code waiting"); waiting != 2 {
				t.Errorf("the log holds %d waiting lines, want 2", waiting)
			}
			last := handed.at[len(handed.at)-1]
			wantLogged(t, logged, "expires_at="+last.Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z07:00"))
		})
	}
}

// TestCheckFailsWhenItCannotRenewTheCode checks an expired code and its lock file in a folder it cannot write.
func TestCheckFailsWhenItCannotRenewTheCode(t *testing.T) {
	needsFileModes(t)
	t.Parallel()

	setup, path, old := prepared(t)
	expire(t, path)
	plantFile(t, path+".lock", "", 0o600)
	lockFolder(t, path)

	err := setup.Check(t.Context(), nil, old)

	wantFailure(t, err, "write the setup code at "+path)
	if held := fileCode(t, path); held != old {
		t.Errorf("the code file holds %q, want the expired %q left in place", held, old)
	}
}

// TestCheckWritesNothingWithoutACodeFile checks that a missing code file refuses every code and stays missing.
func TestCheckWritesNothingWithoutACodeFile(t *testing.T) {
	t.Parallel()

	path := codePath(t)

	err := newSetup(path).Check(t.Context(), nil, rand.Text())

	if !errors.Is(err, authkit.ErrSetupCodeInvalid) {
		t.Errorf("Check() error = %v, want ErrSetupCodeInvalid", err)
	}
	absent(t, path)
}

// TestCheckAnswersSetupDoneOnceTheSiteHasAnAccount checks that a Setup that saw an account refuses every code.
func TestCheckAnswersSetupDoneOnceTheSiteHasAnAccount(t *testing.T) {
	t.Parallel()

	setup, path, old := prepared(t)
	store := testkit.NewStore()
	addMaria(t, store)
	if _, err := setup.Prepare(t.Context(), store); err != nil {
		t.Fatalf("Prepare() with an account error = %v", err)
	}
	if _, err := newSetup(path).Prepare(t.Context(), nil); err != nil {
		t.Fatalf("Prepare() of another copy error = %v", err)
	}
	other := fileCode(t, path)

	for what, code := range map[string]string{"the old code": old, "a code another copy wrote since": other} {
		if err := setup.Check(t.Context(), nil, code); !errors.Is(err, authkit.ErrSetupDone) {
			t.Errorf("Check() of %s error = %v, want ErrSetupDone", what, err)
		}
	}
}

// TestCheckAnswersSetupDoneOnceTheStoreHoldsAnAccount checks that an account made from the shell closes setup.
func TestCheckAnswersSetupDoneOnceTheStoreHoldsAnAccount(t *testing.T) {
	t.Parallel()

	setup, path, code := prepared(t)
	store := testkit.NewStore()
	createdByShell(t, store)
	if err := setup.Check(t.Context(), nil, code); err != nil {
		t.Fatalf("Check() with no store error = %v, want nil", err)
	}

	err := setup.Check(t.Context(), store, code)

	if !errors.Is(err, authkit.ErrSetupDone) {
		t.Errorf("Check() with the store error = %v, want ErrSetupDone", err)
	}
	absent(t, path)
}

// TestCheckReplacesACodeOthersCouldRead checks that a code made readable after Prepare is burned.
func TestCheckReplacesACodeOthersCouldRead(t *testing.T) {
	t.Parallel()

	setup, path, old := prepared(t)
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatalf("os.Chmod() error = %v", err)
	}

	err := setup.Check(t.Context(), nil, old)

	if !errors.Is(err, authkit.ErrSetupCodeExpired) {
		t.Errorf("Check() error = %v, want ErrSetupCodeExpired", err)
	}
	wantFreshCode(t, path, old)
}

// TestCheckRefusesAFileThatHoldsNoCode checks that a file written over the code stays and refuses the code.
func TestCheckRefusesAFileThatHoldsNoCode(t *testing.T) {
	t.Parallel()

	setup, path, old := prepared(t)
	plantFile(t, path, "hello\n", 0o600)

	err := setup.Check(t.Context(), nil, old)

	if !errors.Is(err, authkit.ErrSetupCodeFile) {
		t.Errorf("Check() error = %v, want ErrSetupCodeFile", err)
	}
	if held := readFile(t, path); held != "hello\n" {
		t.Errorf("the file at the path holds %q, want hello kept", held)
	}
}

// firstMaria creates Maria Perez as the first account with code and the test password.
func firstMaria(t *testing.T, setup *authkit.Setup, store authkit.SetupStore, code string) (authkit.Account, error) {
	t.Helper()
	return setup.CreateFirstAccount(t.Context(), store, authkit.FirstAccount{
		Code: code, Email: "maria@example.com", Name: "Maria Perez", Password: testPassword,
	})
}

// codeKept fails t unless code still passes Check with no store.
func codeKept(t *testing.T, setup *authkit.Setup, code string) {
	t.Helper()
	if err := setup.Check(t.Context(), nil, code); err != nil {
		t.Errorf("Check() of the code error = %v, want the code kept", err)
	}
}

// storedOwner returns Maria Perez as store holds her, failing t unless she is a confirmed owner with the password.
func storedOwner(t *testing.T, store *testkit.Store) gouncer.User {
	t.Helper()
	held, err := store.UserByEmail(t.Context(), "maria@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the account stored", err)
	}
	if held.Role != "owner" {
		t.Errorf("the stored role = %q, want owner", held.Role)
	}
	if !held.Confirmed {
		t.Errorf("the stored account is unconfirmed, want it confirmed")
	}
	if !gouncer.VerifyPassword(held.PasswordHash, testPassword) {
		t.Errorf("the stored account does not hold the password it was created with")
	}
	return held
}

// TestCreateFirstAccountStartsTheAccountUnderTheRoleItIsGiven checks the account and the spent code.
func TestCreateFirstAccountStartsTheAccountUnderTheRoleItIsGiven(t *testing.T) {
	t.Parallel()

	setup, path, code := prepared(t)
	store := testkit.NewStore()

	account, err := firstMaria(t, setup, store, code)

	if err != nil {
		t.Fatalf("CreateFirstAccount() error = %v, want nil", err)
	}
	held := storedOwner(t, store)
	want := authkit.Account{
		ID: held.ID, Email: "maria@example.com", Name: "Maria Perez", Confirmed: true, Role: "owner",
		CreatedAt: account.CreatedAt,
	}
	if account != want || !account.CreatedAt.Equal(held.CreatedAt) {
		t.Errorf("CreateFirstAccount() = %+v, want %+v created at %v", account, want, held.CreatedAt)
	}
	absent(t, path)
	if err := setup.Check(t.Context(), nil, code); !errors.Is(err, authkit.ErrSetupDone) {
		t.Errorf("Check() of the spent code error = %v, want ErrSetupDone", err)
	}
}

// TestCreateFirstAccountChecksTheCodeBeforeTheDetails checks that a wrong code is refused before a weak password.
func TestCreateFirstAccountChecksTheCodeBeforeTheDetails(t *testing.T) {
	t.Parallel()

	setup, _, _ := prepared(t)

	_, err := setup.CreateFirstAccount(t.Context(), testkit.NewStore(), authkit.FirstAccount{
		Code: rand.Text(), Email: "maria@example.com", Name: "Maria Perez", Password: "elevenchars",
	})

	if !errors.Is(err, authkit.ErrSetupCodeInvalid) {
		t.Errorf("CreateFirstAccount() error = %v, want ErrSetupCodeInvalid before the password", err)
	}
}

// TestCreateFirstAccountKeepsTheCodeWhenTheDetailsAreRefused checks that a weak password spends nothing.
func TestCreateFirstAccountKeepsTheCodeWhenTheDetailsAreRefused(t *testing.T) {
	t.Parallel()

	setup, _, code := prepared(t)
	store := testkit.NewStore()

	_, err := setup.CreateFirstAccount(t.Context(), store, authkit.FirstAccount{
		Code: code, Email: "maria@example.com", Name: "Maria Perez", Password: "elevenchars",
	})

	if !errors.Is(err, gouncer.ErrWeakPassword) || len(store.Users) != 0 {
		t.Errorf("CreateFirstAccount() error = %v storing %d accounts, want ErrWeakPassword and none", err,
			len(store.Users))
	}
	codeKept(t, setup, code)
}

// TestCreateFirstAccountKeepsTheCodeWhenTheStoreFails checks that a failed account write spends nothing.
func TestCreateFirstAccountKeepsTheCodeWhenTheStoreFails(t *testing.T) {
	t.Parallel()

	setup, _, code := prepared(t)
	store := testkit.NewStore()
	store.CreateUserErr = errLookupFailed

	_, err := firstMaria(t, setup, store, code)

	if !errors.Is(err, errLookupFailed) {
		t.Errorf("CreateFirstAccount() error = %v, want the store failure", err)
	}
	codeKept(t, setup, code)
}

// TestCreateFirstAccountAnswersSetupDoneOnceAnAccountExists checks a site whose account came from the shell.
func TestCreateFirstAccountAnswersSetupDoneOnceAnAccountExists(t *testing.T) {
	t.Parallel()

	setup, path, code := prepared(t)
	store := testkit.NewStore()
	createdByShell(t, store)

	_, err := setup.CreateFirstAccount(t.Context(), store, authkit.FirstAccount{
		Code: code, Email: "maria+2@example.com", Name: "Maria Perez", Password: testPassword,
	})

	if !errors.Is(err, authkit.ErrSetupDone) || len(store.Users) != 1 {
		t.Errorf("CreateFirstAccount() error = %v leaving %d accounts, want ErrSetupDone and 1", err, len(store.Users))
	}
	absent(t, path)
}

// raceOutcome counts how racing first account creations ended.
type raceOutcome struct {
	// won counts the creations that answered nil.
	won int
	// done counts the creations that answered ErrSetupDone.
	done int
	// stored counts the accounts the store holds after the race.
	stored int
}

// TestCreateFirstAccountsRacingLandOnce checks that eight requests with the right code make one account.
func TestCreateFirstAccountsRacingLandOnce(t *testing.T) {
	t.Parallel()

	const racers = 8
	setup, _, code := prepared(t)
	store := testkit.NewStore()
	answers := make([]error, racers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range racers {
		wg.Go(func() {
			<-start
			email := "maria+" + string(rune('a'+i)) + "@example.com"
			_, answers[i] = setup.CreateFirstAccount(t.Context(), store, authkit.FirstAccount{
				Code: code, Email: email, Name: "Maria Perez", Password: testPassword,
			})
		})
	}
	close(start)
	wg.Wait()

	got := raceOutcome{stored: len(store.Users)}
	for _, err := range answers {
		switch {
		case err == nil:
			got.won++
		case errors.Is(err, authkit.ErrSetupDone):
			got.done++
		}
	}
	if want := (raceOutcome{won: 1, done: racers - 1, stored: 1}); got != want {
		t.Errorf("the racing creations answered %v, ending %+v, want %+v", answers, got, want)
	}
}

// TestCreateFirstAccountRefusesARoleItCannotGive checks that a role the config cannot give spends nothing.
func TestCreateFirstAccountRefusesARoleItCannotGive(t *testing.T) {
	t.Parallel()

	for _, tt := range unusableRoles {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			setup, path, code := prepared(t)
			cfg := setupConfig(path)
			cfg.Role = tt.role
			store := testkit.NewStore()

			_, err := firstMaria(t, authkit.NewSetup(cfg), store, rand.Text())

			wantMisconfigured(t, err, tt.cause, tt.words)
			if len(store.Users) != 0 {
				t.Errorf("the refused creation stored %d accounts, want none", len(store.Users))
			}
			codeKept(t, setup, code)
		})
	}
}

// TestSetupRefusesANegativeLifetime checks that Prepare and CreateFirstAccount refuse a lifetime below zero.
func TestSetupRefusesANegativeLifetime(t *testing.T) {
	t.Parallel()

	setup, path, code := prepared(t)
	cfg := setupConfig(path)
	cfg.CodeLifetime = -time.Hour
	negative := authkit.NewSetup(cfg)
	store := testkit.NewStore()

	_, prepareErr := negative.Prepare(t.Context(), store)
	_, createErr := firstMaria(t, negative, store, code)

	for _, err := range []error{prepareErr, createErr} {
		wantMisconfigured(t, err, nil, "the code lifetime -1h0m0s is negative")
	}
	if len(store.Users) != 0 {
		t.Errorf("the refused creation stored %d accounts, want none", len(store.Users))
	}
	codeKept(t, setup, code)
}

// TestCreateFirstAccountRefusesANilStore checks that a missing store is a plain error that spends nothing.
func TestCreateFirstAccountRefusesANilStore(t *testing.T) {
	t.Parallel()

	setup, _, code := prepared(t)

	_, err := firstMaria(t, setup, nil, code)

	wantFailure(t, err, "create the first account: no store")
	codeKept(t, setup, code)
}

// TestCreateFirstAccountKeepsTheAccountWhenTheCodeFileCannotGo checks a code file the process cannot remove.
func TestCreateFirstAccountKeepsTheAccountWhenTheCodeFileCannotGo(t *testing.T) {
	needsFileModes(t)
	t.Parallel()

	path := codePath(t)
	setup, logged, _ := observedSetup(path)
	if _, err := setup.Prepare(t.Context(), nil); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	code := fileCode(t, path)
	lockFolder(t, path)

	account, err := firstMaria(t, setup, testkit.NewStore(), code)

	if err != nil || account.Email != "maria@example.com" {
		t.Errorf("CreateFirstAccount() = %+v, %v, want the account and nil", account, err)
	}
	wantLogged(t, logged, "level=WARN", "path="+path)
	wantNoCodeLogged(t, logged, code)
	if err := setup.Check(t.Context(), nil, code); !errors.Is(err, authkit.ErrSetupDone) {
		t.Errorf("Check() of the spent code error = %v, want ErrSetupDone", err)
	}
}
