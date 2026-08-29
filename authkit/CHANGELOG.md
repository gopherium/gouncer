# Changelog

All notable changes to the `authkit` module are documented in this file. The
format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
the module follows [Semantic Versioning](https://semver.org/). While at v0.x,
minor releases may contain breaking changes.

Releases of this module are tagged `authkit/vX.Y.Z`.

## [Unreleased]

### Added

- `Invites`, the invite and reset flows over an `InviteStore`: `Invite`
  creates an unconfirmed account with its activation token,
  `ResendInvite` replaces a pending token, `RedeemInvite` sets the
  password, confirms the address and answers the account,
  `RequestReset` issues a reset token for confirmed enabled accounts
  only, and `RedeemReset` replaces the password and ends every session
  the account holds.
- `InviteStore` asks for each redemption whole. `ActivateByToken` spends
  the invite token, stores the password and confirms the address, and
  `ResetByToken` spends the reset token, stores the password and ends
  every session, each landing complete or not at all. A refused
  redemption spends no token, so the same link stays good and a
  transient store failure costs the holder nothing.
- `InviteStore` asks for each issuance whole too. `CreateToken` stores a
  token only for an enabled account, and `ReplaceToken` puts one in
  place of every token the account holds for the same purpose as one
  change. A refused resend therefore leaves the standing invite good
  rather than stranding the account without a link.
- Disabling an account revokes every invite and reset token it holds,
  so a link posted before the disable stays dead through any later
  re-enable. Issuance and both redemptions answer
  `gouncer.ErrUserNotFound` for a disabled account, so a disable cannot
  be straddled by a token being minted or spent, and `ResendInvite`
  refuses one the way `RequestReset` always has.
- `ActivateByToken` answers `gouncer.ErrUserNotFound` for an account
  already confirmed, so a token minted while another redemption
  activates the same account cannot replace the password it settled on.
- Token secrets reach the caller alone. The store is handed the hash
  by itself, so no `InviteStore` is in a position to persist one.
- `InviteStore`, the storage capabilities the flows need beyond
  `gouncer.Store`, and `ErrAlreadyActivated`.
- `DefaultInviteTTL` (seven days) and `DefaultResetTTL` (one hour).
- `TokenReaper`, swept by the reaper alongside sessions when the store
  offers it, taking expired tokens and the unconfirmed accounts an
  expired invite leaves behind.
- The testkit store carries tokens and implements every new capability.

### Changed

- The module builds against gouncer v0.4.0.

## [0.11.0] - 2026-08-25

### Changed

- `EnsureAdmin` stamps its role onto an existing account that holds
  none, so a seed run repairs an installation that adopted roles after
  the account was created. An account already holding any role is left
  exactly as it is.

## [0.10.0] - 2026-08-23

### Changed

- **Breaking.** The error body is named for what it is. `Refusal` becomes
  `ErrorResponse` and carries its JSON tags itself, `RespondRefusal`
  folds into `RespondError`, which now takes the struct instead of a
  bare message, and `RefusalForAuthError` becomes
  `ErrorResponseForAuthError`. No alias keeps the old names.
- Nothing on the wire changes. The envelope stays `{error, code, meta}`
  and every code keeps its value, `self_disable_refused`,
  `self_role_refused` and `last_privileged_refused` included.

## [0.9.0] - 2026-08-22

### Changed

- **Breaking.** The concept is named role, never rank. `Identity.Rank`
  becomes `Identity.Role` and `Account.Rank` becomes `Account.Role`, both
  carrying `role` on the wire, `SetAccountRank` becomes `SetAccountRole`,
  the `SetRank` handler becomes `SetRole`, `AdminStore.SetUserRank`
  becomes `SetUserRole`, and `ErrSelfRank` becomes `ErrSelfRole`.
  `CreateAccount`, `CreateAdmin` and `EnsureAdmin` keep their names and
  take a role. No alias keeps the old names.
- **Breaking.** The refusal codes `rank_insufficient` and
  `self_rank_refused` become `role_insufficient` and `self_role_refused`,
  and the `rank` field in the body that creates an account or writes the
  role of one becomes `role`, which the role write requires.
- `testkit.Store` matches, with `SetRoleErr` in place of `SetRankErr`.
- Requires `gouncer` v0.3.0 for its role vocabulary.

## [0.8.0] - 2026-08-21

### Changed

- **Breaking.** `CreateAdmin` and `EnsureAdmin` take the rank the account
  starts under, refusing an empty one. Neither reads the privileged ranks,
  so the caller names a rank its own gate admits.

## [0.7.0] - 2026-08-20

### Added

- `AdminConfig`, naming the store an administration serves and the ranks
  it admits.
- `Account.Rank`, the rank a listed account holds, and a `rank` field in
  the body that creates an account, naming the rank it starts under.
- `AdminHandlers.SetRank` and `AdminHandlers.SetAccountRank`, writing the
  rank an account holds, refusing an actor changing its own with the code
  `self_rank_refused`.
- `ErrSelfRank`, reported when an account changes its own rank.
- The refusal code `last_privileged_refused`, answering a write that
  would leave no enabled account under a privileged rank.

### Changed

- **Breaking.** `NewAdmin` takes an `AdminConfig` rather than a store.
- **Breaking.** `CreateAccount` takes the rank the account starts under.
- **Breaking.** `AdminStore` requires `SetUserDisabledUnderCover` and
  `SetUserRank` in place of `SetUserDisabled`, so every disable an
  administration performs passes the privileged guard.
- Every administration route refuses an actor holding no privileged rank
  with the code `rank_insufficient`. Configuring no ranks admits every
  actor, so a consumer is unaffected until it names them.

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

[0.11.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fv0.11.0
[0.10.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fv0.10.0
[0.9.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fv0.9.0
[0.8.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fv0.8.0
[0.7.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fv0.7.0
[0.6.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fv0.6.0
[0.5.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fv0.5.0
[0.4.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fv0.4.0
[0.3.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fv0.3.0
[0.2.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fv0.2.0
[0.1.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fv0.1.0
