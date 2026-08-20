# Changelog

All notable changes to the `authkit/postgres` module are documented in
this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the module
follows [Semantic Versioning](https://semver.org/). While at v0.x, minor
releases may contain breaking changes.

Releases of this module are tagged `authkit/postgres/vX.Y.Z`.

## [0.4.0] - 2026-08-20

### Added

- Migration `00003`, adding the `rank` column to `auth.users`, unranked by
  default, with an index over the accounts that hold one.
- `UserStore.SetUserRank`, which refuses to leave no enabled account under
  a privileged rank.
- `UserStore.SetUserDisabledUnderCover`, disabling under the same refusal.
- `UserStore.GrantRankToRankless`, the idempotent grant a bootstrap runs,
  refusing an empty rank.
- Every read of a user carries the rank the account holds.

### Changed

- Requires `gouncer` v0.2.0 and `authkit` v0.6.0.
- The toolchain moves to go1.26.7, which carries the standard library
  fixes for GO-2026-5972, GO-2026-6088 and GO-2026-6090.

## [0.3.0] - 2026-07-30

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

[0.4.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fpostgres%2Fv0.4.0
[0.3.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fpostgres%2Fv0.3.0
[0.2.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fpostgres%2Fv0.2.0
[0.1.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fpostgres%2Fv0.1.0
