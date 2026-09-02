# Changelog

All notable changes to the `authkit/postgres` module are documented in
this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the module
follows [Semantic Versioning](https://semver.org/). While at v0.x, minor
releases may contain breaking changes.

Releases of this module are tagged `authkit/postgres/vX.Y.Z`.

## [Unreleased]

### Added

- `UserStore.Within` answers a store running every call inside a
  caller's `pgx.Tx`, so account and token creation can join a larger
  atomic write. The store's own transactions become savepoints there,
  and a refused savepoint leaves the caller's transaction usable.

## [0.10.0] - 2026-08-31

### Changed

- **Breaking.** `CreateToken` takes the number of live tokens it may
  admit, refusing at that cap inside the same account lock that guards
  every token operation. Requires authkit v0.15.0, whose invite flow
  passes 1.
- `ResetByToken` also deletes every other reset token the account holds
  inside its transaction, so spending any link of a stack kills its
  siblings.

## [0.9.0] - 2026-08-29

### Added

- `UserStore` implements `authkit.InviteStore` and `authkit.TokenReaper`.
  `CreateToken` and `ReplaceToken` issue a token against an enabled
  account, `ActivateByToken` and `ResetByToken` spend one and write the
  account it names, and `DeleteExpiredTokens` sweeps expired tokens with
  the unconfirmed accounts an expired invite strands. Each is one
  transaction, so a failure leaves the account and the token exactly as
  they were.
- Migration `00004` adds `auth.users.confirmed` and the `auth.tokens`
  table with `tokens_user_id_purpose_idx` and `tokens_expires_at_idx`.
  The column defaults to true, so rows an earlier release wrote and rows
  it goes on writing during a rolling upgrade both count as activated.

### Fixed

- Disabling an account revokes the invite and reset tokens it holds, on
  the guarded path an administration uses as well as the plain one. A
  link posted before the disable no longer activates the account or
  replaces its password after a later re-enable.
- Every token operation holds the account row while it runs, so a token
  cannot be issued against an account being disabled, two issuances
  cannot both mint a live token for one purpose, and a redemption cannot
  deadlock against a disable.
- The sweep holds each account an expired invite stranded before it
  looks again, so a resend landing beside the sweep keeps both the
  account and its fresh link.
- A replacement invite requires an unactivated account, so a resend
  racing an activation refuses rather than answering a dead link.

### Changed

- The module requires `gouncer` 0.4.0 and `authkit` 0.12.0.

## [0.8.0] - 2026-08-25

### Changed

- The module requires `authkit` 0.11.0, the first release whose
  `EnsureAdmin` stamps its role onto an account holding none.

## [0.7.0] - 2026-08-22

### Changed

- **Breaking.** The concept is named role, never rank. Migration `00003`
  adds `auth.users.role` and `users_role_idx`, whereas every release
  through 0.6.0 added `rank` and `users_rank_idx`. Goose records no
  checksum, so a database migrated by 0.5.0 or 0.6.0 keeps the old names
  and no migration will correct it. Recreate that database, or rename by hand
  with `ALTER TABLE auth.users RENAME COLUMN rank TO role;` and
  `ALTER INDEX auth.users_rank_idx RENAME TO users_role_idx;`.
- **Breaking.** `UserStore.SetUserRank` becomes `SetUserRole` and
  `UserStore.GrantRankToRankless` becomes `GrantRoleToRoleless`. No alias
  keeps the old names.
- **Breaking.** The `grantrank` subcommand becomes `grantrole`, run by
  `RunGrantRole`, and both it and `RunCreateAdmin` take `-role` in place
  of `-rank`.
- Requires `gouncer` v0.3.0 and `authkit` v0.9.0 for their role
  vocabulary.

## [0.6.0] - 2026-08-21

### Added

- `RunGrantRank`, giving a rank to every account holding none and
  reporting how many took it, which an installation adopting ranks runs
  once from the command line.

### Changed

- **Breaking.** `RunCreateAdmin` takes a `-rank` flag naming the rank the
  new account starts under, and refuses without it.
- Requires `authkit` v0.8.0.

## [0.5.0] - 2026-08-20

### Changed

- Requires `authkit` v0.7.0, whose administration store asks for the
  guarded writes this module already carries.

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

[0.9.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fpostgres%2Fv0.9.0
[0.8.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fpostgres%2Fv0.8.0
[0.7.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fpostgres%2Fv0.7.0
[0.6.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fpostgres%2Fv0.6.0
[0.5.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fpostgres%2Fv0.5.0
[0.4.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fpostgres%2Fv0.4.0
[0.3.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fpostgres%2Fv0.3.0
[0.2.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fpostgres%2Fv0.2.0
[0.1.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fpostgres%2Fv0.1.0
