// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// codeLength is how many characters a setup code holds.
const codeLength = 26

// codeAlphabet is every character a setup code may hold.
const codeAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

// maxCodeFileBytes is the most a file holding a setup code may hold.
const maxCodeFileBytes = 64

// sharedBits are the mode bits that let the group or others into a file.
const sharedBits fs.FileMode = 0o077

// heldCode is a setup code read from the code file, with when it expires and whether others could read it.
type heldCode struct {
	// code is the setup code the file holds.
	code string
	// expiresAt is when the code stops working, in UTC.
	expiresAt time.Time
	// burned reports whether the file let the group or others in.
	burned bool
	// seen is what lstat answered for the file the code was read from.
	seen fs.FileInfo
}

// ensureCode answers the live private code at the code file, writing a fresh one when none is there or it went stale.
func (s *Setup) ensureCode(ctx context.Context) (heldCode, error) {
	held, err := s.readCode()
	if errors.Is(err, fs.ErrNotExist) {
		return s.placeCode()
	}
	if err != nil {
		return heldCode{}, err
	}
	if s.stale(held) {
		return s.renewCode(ctx, held)
	}
	return held, nil
}

// stale reports whether a held code expired or others could read it.
func (s *Setup) stale(held heldCode) bool {
	return held.burned || s.expired(held.expiresAt)
}

// expired reports whether a code expiring at expiresAt has stopped working.
func (s *Setup) expired(expiresAt time.Time) bool {
	return !s.now().Before(expiresAt)
}

// renewCode renews the stale code while it holds the renewal mutex and the lock file beside the code file.
func (s *Setup) renewCode(ctx context.Context, stale heldCode) (heldCode, error) {
	s.renewing.Lock()
	defer s.renewing.Unlock()
	unlock, err := s.lockRenewal()
	if err != nil {
		return heldCode{}, err
	}
	defer unlock()
	return s.renewLocked(ctx, stale)
}

// renewLocked replaces the stale code with a fresh one, answering instead a live private code another renewal left.
func (s *Setup) renewLocked(ctx context.Context, stale heldCode) (heldCode, error) {
	current, err := s.readCode()
	if errors.Is(err, fs.ErrNotExist) {
		return s.placeCode()
	}
	if err != nil {
		return heldCode{}, err
	}
	if !sameFile(stale.seen, current.seen) && !s.stale(current) {
		return current, nil
	}
	if stale.burned || current.burned {
		s.cfg.Logger.WarnContext(ctx, "setup code was readable by others, so a new one replaced it",
			"path", s.cfg.CodeFile)
	}
	return s.replaceCode(current)
}

// replaceCode renames a fresh code over the stale one while the code file still holds it, keeping what took its place.
func (s *Setup) replaceCode(stale heldCode) (heldCode, error) {
	temp, ours, err := s.writeTemp()
	if err != nil {
		return heldCode{}, s.writeFailure(err)
	}
	defer func() { _ = os.Remove(temp) }()
	now, err := s.lstat(s.cfg.CodeFile)
	if errors.Is(err, fs.ErrNotExist) {
		return s.placeCode()
	}
	if err != nil {
		return heldCode{}, s.readFailure(err)
	}
	if !sameFile(stale.seen, now) {
		return s.keptCode()
	}
	if err := os.Rename(temp, s.cfg.CodeFile); err != nil {
		return heldCode{}, s.writeFailure(err)
	}
	return s.readBack(ours)
}

// sameFile reports whether a and b describe the same file at the same modification time.
func sameFile(a, b fs.FileInfo) bool {
	return os.SameFile(a, b) && a.ModTime().Equal(b.ModTime())
}

// keptCode answers the live private code another writer put at the code file, refusing anything else it holds.
func (s *Setup) keptCode() (heldCode, error) {
	held, err := s.readCode()
	if err != nil {
		return heldCode{}, err
	}
	if s.stale(held) {
		return heldCode{}, fmt.Errorf("%w: %s changed while it was renewed, so it stays in place", ErrSetupCodeFile,
			s.cfg.CodeFile)
	}
	return held, nil
}

// readCode reads the code file without following a link, refusing what is there unless it is one setup code.
func (s *Setup) readCode() (heldCode, error) {
	seen, err := s.lstat(s.cfg.CodeFile)
	if err != nil {
		return heldCode{}, s.readFailure(err)
	}
	if !seen.Mode().IsRegular() {
		return heldCode{}, fmt.Errorf("%w: %s is not a regular file, so it stays in place", ErrSetupCodeFile,
			s.cfg.CodeFile)
	}
	text, err := s.readSame(seen)
	if err != nil {
		return heldCode{}, err
	}
	if !isCode(text) {
		return heldCode{}, s.notACode()
	}
	return heldCode{
		code:      text,
		expiresAt: seen.ModTime().Add(s.cfg.CodeLifetime).UTC(),
		burned:    seen.Mode().Perm()&sharedBits != 0,
		seen:      seen,
	}, nil
}

// readSame returns the trimmed text of the code file, refusing a file other than the one seen or a long one.
func (s *Setup) readSame(seen fs.FileInfo) (string, error) {
	file, err := s.openFile(s.cfg.CodeFile)
	if err != nil {
		return "", s.readFailure(err)
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(seen, opened) {
		return "", fmt.Errorf("%w: %s changed while it was read, so it stays in place", ErrSetupCodeFile,
			s.cfg.CodeFile)
	}
	if opened.Size() > maxCodeFileBytes {
		return "", s.notACode()
	}
	data, err := io.ReadAll(io.LimitReader(file, maxCodeFileBytes))
	if err != nil {
		return "", s.readFailure(err)
	}
	return strings.TrimSpace(string(data)), nil
}

// isCode reports whether text is one setup code, codeLength characters from codeAlphabet.
func isCode(text string) bool {
	return len(text) == codeLength && strings.Trim(text, codeAlphabet) == ""
}

// codeSeparators drops the spaces, tabs, line breaks and hyphens a person may type inside a code.
var codeSeparators = strings.NewReplacer(" ", "", "\t", "", "\r", "", "\n", "", "-", "")

// normalizeCode returns input without separators and in capitals.
func normalizeCode(input string) string {
	return strings.ToUpper(codeSeparators.Replace(input))
}

// sameCode reports in constant time whether input and held are the same code.
func sameCode(input, held string) bool {
	given := sha256.Sum256([]byte(input))
	want := sha256.Sum256([]byte(held))
	return subtle.ConstantTimeCompare(given[:], want[:]) == 1
}

// readFailure wraps err as a failure to read the code file.
func (s *Setup) readFailure(err error) error {
	return fmt.Errorf("authkit: read the setup code at %s: %w", s.cfg.CodeFile, err)
}

// writeFailure wraps err as a failure to write the code file.
func (s *Setup) writeFailure(err error) error {
	return fmt.Errorf("authkit: write the setup code at %s: %w", s.cfg.CodeFile, err)
}

// notACode reports a code file that holds no setup code and stays in place.
func (s *Setup) notACode() error {
	return fmt.Errorf("%w: %s does not hold a setup code, so it stays in place", ErrSetupCodeFile, s.cfg.CodeFile)
}

// placeCode writes a fresh code beside the code file and links it into place unless another copy did first.
func (s *Setup) placeCode() (heldCode, error) {
	ours, linked := s.linkFresh()
	if linked != nil && !errors.Is(linked, fs.ErrExist) {
		return heldCode{}, s.writeFailure(linked)
	}
	return s.readBack(ours)
}

// readBack answers the code the code file holds after a write, refusing and removing one others can still read.
func (s *Setup) readBack(ours fs.FileInfo) (heldCode, error) {
	held, err := s.readCode()
	if err != nil {
		return heldCode{}, s.failedReadBack(err, ours)
	}
	if held.burned {
		refused := fmt.Errorf("%w: this system cannot keep %s private", ErrSetupCodeFile, s.cfg.CodeFile)
		return heldCode{}, errors.Join(refused, s.unlink())
	}
	return held, nil
}

// failedReadBack answers err, first removing the code file while it is ours and err is not ErrSetupCodeFile.
func (s *Setup) failedReadBack(err error, ours fs.FileInfo) error {
	if errors.Is(err, ErrSetupCodeFile) {
		return err
	}
	if now, looked := s.lstat(s.cfg.CodeFile); looked != nil || !os.SameFile(ours, now) {
		return err
	}
	return errors.Join(err, s.unlink())
}

// linkFresh links a fresh private code to the code file unless one exists there, answering what it linked.
func (s *Setup) linkFresh() (fs.FileInfo, error) {
	temp, ours, err := s.writeTemp()
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(temp) }()
	if err := os.Link(temp, s.cfg.CodeFile); err != nil {
		return nil, err
	}
	return ours, nil
}

// writeTemp writes a fresh code to a private temporary file beside the code file and answers its name and FileInfo.
func (s *Setup) writeTemp() (string, fs.FileInfo, error) {
	temp, err := s.createTemp(filepath.Dir(s.cfg.CodeFile), "."+filepath.Base(s.cfg.CodeFile)+".*")
	if err != nil {
		return "", nil, err
	}
	_, written := temp.WriteString(rand.Text() + "\n")
	info, stated := temp.Stat()
	if err = errors.Join(written, stated, temp.Sync(), temp.Close()); err != nil {
		_ = os.Remove(temp.Name())
		return "", nil, err
	}
	return temp.Name(), info, nil
}

// removeCode deletes the code file when it holds a setup code and leaves anything else at the path in place.
func (s *Setup) removeCode() error {
	_, err := s.readCode()
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, ErrSetupCodeFile) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.unlink()
}

// unlink deletes the code file, a file already gone included.
func (s *Setup) unlink() error {
	if err := os.Remove(s.cfg.CodeFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("authkit: remove the setup code at %s: %w", s.cfg.CodeFile, err)
	}
	return nil
}
