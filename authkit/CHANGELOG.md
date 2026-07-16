# Changelog

All notable changes to the `authkit` module are documented in this file. The
format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
the module follows [Semantic Versioning](https://semver.org/). While at v0.x,
minor releases may contain breaking changes.

Releases of this module are tagged `authkit/vX.Y.Z`.

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

[0.1.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fv0.1.0
