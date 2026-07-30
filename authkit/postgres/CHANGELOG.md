# Changelog

All notable changes to the `authkit/postgres` module are documented in
this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the module
follows [Semantic Versioning](https://semver.org/). While at v0.x, minor
releases may contain breaking changes.

Releases of this module are tagged `authkit/postgres/vX.Y.Z`.

## [Unreleased]

### Added

- `UserStore.UserByID`, looking a user up by identifier and reporting the
  disabled flag to the caller, matching how `UserByEmail` behaves.

## [0.2.0] - 2026-07-26

### Added

- `RunCreateAdmin`, the complete createadmin subcommand: flag parsing,
  auth-schema migration, and delegation to `authkit.CreateAdmin`, so a
  consuming application's dispatch is a single call.

### Changed

- `authkit` is now a production dependency of this module (previously
  test-only).

## [0.1.0] - 2026-07-16

### Added

- `UserStore` implementing `gouncer.Store` plus the `authkit.AdminStore`
  and `authkit.SessionReaper` contracts, with revoke-on-disable session
  semantics in one transaction.
- `Migrate` owning the `auth` schema and migrating it against the
  module's own `auth.goose_db_version` table.
- `Migrations`, the embedded schema migration files.
- `testdb` package with pgtestdb wiring that migrates test databases
  through `Migrate` itself.

[0.2.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fpostgres%2Fv0.2.0
[0.1.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fpostgres%2Fv0.1.0
