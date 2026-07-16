# Changelog

All notable changes to the `authkit/postgres` module are documented in
this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the module
follows [Semantic Versioning](https://semver.org/). While at v0.x, minor
releases may contain breaking changes.

Releases of this module are tagged `authkit/postgres/vX.Y.Z`.

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

[0.1.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fpostgres%2Fv0.1.0
