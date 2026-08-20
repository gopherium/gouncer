# Changelog

All notable changes to the `authkit` module are documented in this file. The
format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
the module follows [Semantic Versioning](https://semver.org/). While at v0.x,
minor releases may contain breaking changes.

Releases of this module are tagged `authkit/vX.Y.Z`.

## [0.6.0] - 2026-08-20

### Added

- `Identity.Rank`, the rank the authenticated account holds, resolved by
  both `Authenticate` and `SessionIdentity`.
- `Config.Privileged`, the ranks a gate admits, copied on construction so
  a caller mutating its own slice cannot widen a gate.
- `Handlers.RequirePrivilege`, refusing a request whose identity holds no
  privileged rank with the code `rank_insufficient`. Configuring no ranks
  admits every request.

### Changed

- Requires `gouncer` v0.2.0 for its rank vocabulary.
- The toolchain moves to go1.26.7, which carries the standard library
  fixes for GO-2026-5972, GO-2026-6088 and GO-2026-6090.

## [0.5.0] - 2026-08-17

### Added

- A stable `Code` on every refusal, so a consumer can answer a refusal in
  its own words rather than matching on message text.

## [0.4.0] - 2026-08-06

### Added

- `Handlers.Authenticate`, `Handlers.StartSession`, `Handlers.EndSession`,
  `Handlers.SessionIdentity`, and `Handlers.CookieName`, the transport free
  service seams the HTTP handlers now delegate to.
- `ErrInvalidCredentials`, reported by `Authenticate` for unknown, wrong,
  or disabled logins.
- `Account`, `AdminHandlers.ListAccounts`, `AdminHandlers.CreateAccount`,
  and `AdminHandlers.SetAccountDisabled`, the administration seams behind
  the admin handlers.
- `ErrSelfDisable`, reported when an account disables itself.

## [0.3.0] - 2026-07-30

### Added

- `testkit.Store.UserByID`, keeping the in-memory double aligned with the
  postgres store.

## [0.2.0] - 2026-07-26

### Added

- `EnsureAdmin` creating a user account unless the email is already
  taken, for non-interactive bootstrapping such as development seeders.

## [0.1.0] - 2026-07-16

### Added

- `Config` and `New` building `Handlers` with `Login`, `Logout`,
  `Session`, and the `RequireSession` middleware, with a configurable
  cookie name and session lifetime.
- `Identity`, `WithIdentity`, and `IdentityFromContext` for the
  authenticated request identity.
- `AdminStore` and `NewAdmin` building `AdminHandlers` with `List`,
  `Create`, and `SetDisabled`, guarding against self-disabling.
- `SessionReaper` and `NewReaper` building a `Reaper` that sweeps
  expired sessions with `Start` and `Stop`.
- `CreateAdmin` for command-line account bootstrapping.
- `Respond`, `RespondError`, `Decode`, `MaxRequestBodyBytes`, and
  `StatusForAuthError` JSON helpers shared with application handlers.
- `testkit.Store`, an in-memory store double encoding the gouncer
  contract semantics.

[0.6.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fv0.6.0
[0.5.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fv0.5.0
[0.4.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fv0.4.0
[0.3.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fv0.3.0
[0.2.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fv0.2.0
[0.1.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fv0.1.0
