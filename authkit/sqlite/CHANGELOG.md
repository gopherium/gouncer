# Changelog

All notable changes to the `authkit/sqlite` module are documented in
this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the module
follows [Semantic Versioning](https://semver.org/). While at v0.x, minor
releases may contain breaking changes.

Releases of this module are tagged `authkit/sqlite/vX.Y.Z`.

## [Unreleased]

### Changed

- The module needs Go 1.27.4.

## [0.3.0] - 2026-10-18

### Added

- `HasUsers`, reporting whether the store holds any account, and
  `CreateFirstUser`, which stores the first account only while the store
  holds none, or returns `authkit.ErrSetupDone`. It checks and inserts in
  one immediate write transaction.

### Changed

- Requires authkit v0.22.0.

## [0.2.0] - 2026-10-13

### Added

- `UsersByIDs`, reading many accounts by id in one query without their
  password hashes, disabled and unconfirmed ones too.

### Changed

- Requires authkit v0.20.0.

## [0.1.0] - 2026-10-11

### Added

- `UserStore` implementing `gouncer.Store` plus the `authkit.AdminStore`,
  `authkit.InviteStore`, `authkit.SessionReaper` and
  `authkit.TokenReaper` contracts on the application's one handle from
  `dbkit/sqlite`. Requires authkit v0.19.0.
- `Migrate` with `MigrateOptions`, which runs the auth migrations under
  dbkit's file lock and records them in `auth_goose_db_version`.
- One baseline migration, `00004`, for `auth_users`, `auth_sessions` and
  `auth_tokens`, with ids as UUID text and times as UTC microseconds.
- A busy, locked or constraint failure carries dbkit's error class, such
  as `dbkit.ErrBusy`. The PostgreSQL store does not class its failures.

[0.3.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fsqlite%2Fv0.3.0
[0.2.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fsqlite%2Fv0.2.0
[0.1.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fsqlite%2Fv0.1.0
