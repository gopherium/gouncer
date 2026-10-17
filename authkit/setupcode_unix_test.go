// SPDX-License-Identifier: Apache-2.0

//go:build unix

package authkit_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// umaskChild names the environment variable that runs the test binary as a child writing a code under a umask.
const umaskChild = "AUTHKIT_SETUP_UMASK_CHILD"

// asUmaskChild runs Prepare under the umask and over the path the environment names, reporting whether it did.
func asUmaskChild(t *testing.T) bool {
	t.Helper()
	spec := os.Getenv(umaskChild)
	if spec == "" {
		return false
	}
	mask, path, _ := strings.Cut(spec, ":")
	bits, err := strconv.ParseUint(mask, 8, 32)
	if err != nil {
		t.Fatalf("parsing the umask %q: %v", mask, err)
	}
	defer syscall.Umask(syscall.Umask(int(bits)))
	if _, err := newSetup(path).Prepare(t.Context(), nil); err != nil {
		t.Fatalf("Prepare() under umask %s error = %v", mask, err)
	}
	return true
}

// underUmask runs the named test as a child that calls Prepare over path under mask, answering its output.
func underUmask(t *testing.T, test, mask, path string) (string, error) {
	t.Helper()
	child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^"+test+"$", "-test.count=1")
	child.Env = append(os.Environ(), umaskChild+"="+mask+":"+path)
	out, err := child.CombinedOutput()
	return string(out), err
}

// TestPrepareWritesAPrivateCodeUnderAnOpenUmask checks the code file mode in a child process under umask 0.
func TestPrepareWritesAPrivateCodeUnderAnOpenUmask(t *testing.T) {
	if asUmaskChild(t) {
		return
	}
	t.Parallel()

	path := codePath(t)

	if out, err := underUmask(t, "TestPrepareWritesAPrivateCodeUnderAnOpenUmask", "0", path); err != nil {
		t.Fatalf("the child under umask 0 failed: %v\n%s", err, out)
	}

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("os.Lstat() error = %v, want the code file the child wrote", err)
	}
	if info.Mode() != 0o600 {
		t.Errorf("the code file mode under umask 0 = %v, want a regular file at exactly 0600", info.Mode())
	}
}

// TestPrepareWritesBesideTheCodeFileWithoutATempFolder checks a start whose system temporary folder is missing.
func TestPrepareWritesBesideTheCodeFileWithoutATempFolder(t *testing.T) {
	path := codePath(t)
	t.Setenv("TMPDIR", filepath.Join(filepath.Dir(path), "missing"))

	if _, err := newSetup(path).Prepare(t.Context(), nil); err != nil {
		t.Fatalf("Prepare() with no system temporary folder error = %v, want nil", err)
	}

	wantPrivateCode(t, path)
}

// TestPrepareStopsWhenTheUmaskHidesTheCodeFromItsOwner checks a start whose umask clears the owner's read bit.
func TestPrepareStopsWhenTheUmaskHidesTheCodeFromItsOwner(t *testing.T) {
	if asUmaskChild(t) {
		return
	}
	needsFileModes(t)
	t.Parallel()

	path := codePath(t)

	out, err := underUmask(t, "TestPrepareStopsWhenTheUmaskHidesTheCodeFromItsOwner", "477", path)

	if err == nil || !strings.Contains(out, "read the setup code at "+path) {
		t.Errorf("the child under umask 0477 answered %v with %s, want Prepare to fail with the read error", err, out)
	}
	absent(t, path)
	if _, err := newSetup(path).Prepare(t.Context(), nil); err != nil {
		t.Errorf("Prepare() under the usual umask after the failed start error = %v, want nil", err)
	}
}
