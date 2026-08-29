# Changelog

All notable changes to this project are documented in this file. The
format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project follows [Semantic Versioning](https://semver.org/). While
at v0.x, minor releases may contain breaking changes.

## [0.4.0] - 2026-08-29

### Added

- `Token`, a single-use hashed secret with a purpose, built by
  `NewToken` under the purposes `PurposeInvite`, `PurposeReset` and
  `PurposeConfirm`.
- `NewInvitedUser`, building an unconfirmed account holding no usable
  password until activation.
- `User.SetPassword`, replacing the password under the same bounds
  `NewUser` applies.
- `User.Confirmed`, true from `NewUser` and false from
  `NewInvitedUser`.
- `ErrEmptyPurpose`, `ErrTokenLifetime`, `ErrTokenNotFound` and
  `ErrTokenExists`.

## [0.3.0] - 2026-08-22

### Changed

- **Breaking.** `Ranks` becomes `Roles`, `User.Rank` becomes
  `User.Role`, and `ErrEmptyRank` becomes `ErrEmptyRole`, with no
  alias for the old names.

## [0.2.0] - 2026-08-20

### Added

- `Ranks`, a set of rank names, and `Holds`, which never admits an
  unranked account.
- `User.Rank`, the rank an account holds, as open text an application
  names for itself.
- `ErrLastPrivileged` for a write leaving no enabled account under a
  privileged rank, and `ErrEmptyRank` for a rank name that is empty.

### Changed

- The toolchain moves to go1.26.7, which carries the standard library
  fixes for GO-2026-5972, GO-2026-6088 and GO-2026-6090.

## [0.1.0] - 2026-07-10

### Added

- `User` and `NewUser` for validated account creation.
- argon2id password hashing with constant-time `VerifyPassword`.
- `Session`, `NewSession`, and `HashToken` for opaque login tokens
  stored only as digests.
- Storage-agnostic `Store` interface with sentinel errors.

[0.4.0]: https://github.com/gopherium/gouncer/releases/tag/v0.4.0
[0.3.0]: https://github.com/gopherium/gouncer/releases/tag/v0.3.0
[0.2.0]: https://github.com/gopherium/gouncer/releases/tag/v0.2.0
[0.1.0]: https://github.com/gopherium/gouncer/releases/tag/v0.1.0
