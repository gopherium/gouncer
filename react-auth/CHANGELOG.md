# Changelog

All notable changes to `@gopherium/react-auth` are documented in this
file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the
package follows [Semantic Versioning](https://semver.org/). While at
0.x, minor releases may contain breaking changes.

Releases are tagged `react-auth@X.Y.Z` and publish to npm from CI. The
npm-style tag stays invisible to the Go toolchain, unlike a
`react-auth/vX.Y.Z` tag naming the directory's stub module.

## [Unreleased]

### Added

- `required` and `customValidity` on `PasswordField`, as `ValidatedInputControl` takes them: the message under the field and its meter in red with the warning icon, `aria-invalid`, and the form held back while it is invalid.

### Changed

- `usersListDefaults` leaves Created out of the table and the phone row, as WordPress's users list has no such column. The list still opens newest first, and View options shows it.

### Fixed

- With `labelPosition="top"` `PasswordField` draws no strength meter for an empty password, where its blank box left a band between the field and the message under it.
- With `labelPosition="top"` the password message and help line sit under the meter at the WPDS field gap the Email message uses, where the message sat 5px lower.
- `RoleSelect` draws its text at 16px on a phone, as the WPDS inputs beside it do, where it stayed at 13px. It needs `/wpds/style.css`.
- With `labelPosition="top"` the strength meter and the Caps Lock line take the form's body text size, where they read larger than the field.
- The `/admin` REST writes keep a server `code` they have no error class for on the plain error, so the app's `errorMessages` for it show.
- The users list draws the email under the name on one line, clipped with an ellipsis at the column edge and whole in a tooltip, where a long one ran under the Status column. It needs `/wpds/style.css`.

## [0.16.1] - 2026-10-29

### Changed

- `/wpds/users` takes `@gopherium/godmin` 0.29 as well as 0.28 as an optional peer.

## [0.16.0] - 2026-10-28

### Added

- `PasswordField` and `usePasswordField` in `/wpds`, the setup step's password row for any form that sets a password: the meter, the show or hide button, the Caps Lock line, the weak password row and an optional `help` line.
- `generateFirst` draws WordPress's "Generate password" button before the field, as its Add User screen does, each press putting in a new password as long as `usePasswordField`'s `length`.
- `labelPosition` on `PasswordField`, DataForm's name: `side`, the default, keeps the installer's rows, and `top` stacks each label above its field as a godmin form does, for an Add User form.
- `avatar` on an account, the address of its picture when the server sends one.
- `resendInvite(email)` in `/admin`, sending a pending account a new invitation that ends the link sent before. Its REST default posts to `POST /api/users/invite/resend`, which no released server serves yet.
- `UserNotEditableError` and `RoleNotGrantableError` for the codes `user_not_editable` and `role_not_grantable`, thrown by `createUser`, `invite`, `resendInvite`, `setUserRole` and `setUserDisabled`.
- `SelfDisableError` and `UserNotFoundError`. `setUserDisabled` throws them, `LastPrivilegedError`, `RoleRefusedError` and the role errors for their codes, where it threw a plain error before.
- `setUserRole` throws `UserNotFoundError` for an account that is gone.
- `code` on every typed error of `/admin`, the code the server answers it with, set by the class so a transport that throws it passes nothing.
- A `ValidationError` from `createUser` or `invite` carries the server's `code` and `meta`.
- `fieldError(error)` in `/admin`, naming the field of an Add User form (`email`, `name` or `password`) an error from `createUser` is about, with the message to show under it.
- `accountWithRole`, `usersOk`, `usersFailure`, `disabledOk`, `disabledRefused`, `roleRefused`, `resendDelivered`, `resendUndelivered` and `resendRefused` in `testing`.
- `@gopherium/react-auth/wpds/users`, the Users page on WordPress DataViews, its own entry so an app that only signs people in never resolves its peers.
- `UsersPage` lists the accounts with search, Role and Status filters, sort by Name, Email or Created, paging and the avatar, all in the view the app holds.
- `UsersPage` offers Change role…, Resend invitation…, Enable and Disable from the row menu, and all but Resend from the bulk bar, never on the reader's own row nor on one the server marks not `editable`.
- Change role opens on one account or several, and a click on the name or a phone tap opens it on an account the reader may change.
- Every run ends in one toast naming the one account whole, as WordPress does, with WordPress's words for each error code authkit answers.
- `UsersPage` takes `roleLabels`, `errorMessages`, `columns`, `showAvatars`, `addLink`, `tabs`, `headingLevel`, `pageSizes` and `sizesFailed`, all data.
- `usersListDefaults`, the view the page opens on, as data for godmin's `useListView`.
- `RoleSelect`, the role menu with the fewest capabilities first, and `roleLabelOf`.
- `errorMessageOf(error, errorMessages, fallback)`, the words the list toasts a failed account write in, for an app's own Add User form.
- `@gopherium/godmin` 0.28 only, `@gopherium/gottext` from 0.6, `@wordpress/dataviews` 20, `@wordpress/components` 42 and `@wordpress/date` 5 join the optional peers, for `/wpds/users` only.
- An `fr-FR` catalogue covering all 118 messages, which `catalogFor('fr-FR')` answers.

### Changed

- **Breaking.** An account requires `confirmed` and `editable`, as authkit v0.24.0 sends them. A server or transport sending neither fails to read.
- **Breaking.** `RoleBeyondReachError` is gone with the code `role_beyond_reach`, split as authkit v0.24.0 splits it into `UserNotEditableError` and `RoleNotGrantableError`.
- **Breaking.** `AuthTransport` requires `resendInvite`. A partial transport given to `configureAuthTransport` keeps working.
- The `es-ES` catalogue covers the Users page, Change role and the Add User field messages.

### Removed

- **Breaking.** `UsersScreen` leaves `/wpds` with its `.gopherium-users` stylesheet rules. `UsersPage` in `/wpds/users` takes its place.
- **Breaking.** `NewUserScreen` leaves `/wpds`, since each app keeps its own Add User page with `RoleSelect` and `PasswordField`.

## [0.15.0] - 2026-10-23

### Changed

- **Breaking.** `usersNavItem` is renamed `usersMenuItem`, the name WordPress's admin gives a menu row. The old name is gone.
- The Users row draws `people` from `@wordpress/icons`, WordPress's people icon, in place of its own glyph.
- `@wordpress/icons` from 18.0.0 joins `@wordpress/ui` as an optional peer of `/wpds`.

## [0.14.0] - 2026-10-21

### Added

- `SetupCodeStep` in `/wpds`, the screen that asks for the setup code, modelled on Jenkins' "Unlock Jenkins", with `fileHint` for where the application keeps the code file.
- `SetupAccountStep` in `/wpds`, the account rows of WordPress's installer (Name, Password, Your Email), with WordPress's password strength meter and its "Confirm use of weak password" row.
- The meter loads zxcvbn 4.4.1, WordPress's version, on demand as its own chunk.
- `isSetupOpen`, `checkSetupCode` and `createFirstAccount`, with `SetupCodeError`, `SetupClosedError` and `SiteStartingError`. A `ValidationError` from `createFirstAccount` carries the server's `code` and `meta`.
- The `FirstAccount` and `Account` types on the package entry point.
- The testing handlers `setupStatus`, `setupCodeOk`, `setupCodeRefused`, `setupCodeClosed`, `setupCodeRateLimited`, `setupCodeStarting`, `firstAccountOk`, `firstAccountCodeRefused`, `firstAccountClosed`, `firstAccountRateLimited` and `firstAccountStarting`, with `createdAccount`, the account `firstAccountOk` answers by default.

### Changed

- **Breaking.** `AuthTransport` gains `isSetupOpen`, `checkSetupCode` and `createFirstAccount`. A partial transport given to `configureAuthTransport` keeps working.
- The Spanish catalogue declares the three CLDR plural forms.

## [0.13.0] - 2026-10-16

### Changed

- `LoginScreen` says "Lost your password?" under the card, as WordPress does.

## [0.12.0] - 2026-10-11

### Changed

- The design system window moves to the `@wordpress/ui` 0.24 train,
  with `@wordpress/theme` from 2.3.0 and `@wordpress/i18n` from 6.30.0.
  Apps on the `@wordpress/ui` 0.23 train stay on 0.11.

## [0.11.1] - 2026-10-11

### Changed

- The optional `vitest` peer admits 5 alongside 4.

## [0.11.0] - 2026-10-11

### Added

- `User` carries the optional `capabilities` list authkit sends with a
  capability source, and `grantable` when that source answers it.
- `can(user, capability)`, answering whether a user holds a capability.
  A missing user or list holds none.
- `RoleUnknownError` and `RoleBeyondReachError`, thrown by `createUser`,
  `invite`, `setUserRole` and `setUserDisabled` for the codes
  `role_unknown` and `role_beyond_reach`. `RoleUnknownError` is a
  `ValidationError`.
- `RoleChangedError`, thrown by `setUserRole` and `setUserDisabled` for
  the code `role_changed`.
- `userWithCapabilities` in `testing`, a canned account holding
  capabilities and the roles it may hand out.

## [0.10.0] - 2026-10-04

### Changed

- The design system window moves to `@wordpress/ui` 0.23 and
  `@wordpress/theme` 2.2, and `@wordpress/i18n` starts at 6.29.

## [0.9.0] - 2026-09-16

### Changed

- The design system window moves to `@wordpress/ui` 0.22 and
  `@wordpress/theme` 2.1, and `@wordpress/i18n` starts at 6.28.

### Removed

- The React 19 patch for `@wordpress/element`. Delete your copy and its
  `patchedDependencies` entry.

## [0.8.0] - 2026-08-30

### Added

- `invite`, sending an invitation and answering the same shape whatever
  the address's state, so a taken address is indistinguishable from a
  fresh one. An undelivered report without an activation link is
  refused.
- `acceptInvite`, setting the first password and starting the session in
  one step.
- `requestPasswordReset`, asking for a reset link and answering nothing
  about the address.
- `resetPassword`, replacing the password behind a reset link.
- `InvalidTokenError`, answered for a spent, expired or unknown link.
- `adoptSession`, installing a freshly authenticated user as the cached
  session, cancelling any in-flight fetch and dropping what the previous
  account left cached.
- `SetPasswordScreen`, `RequestResetScreen` and `ResetPasswordScreen`,
  the three screens behind those operations. Each tells a throttled
  person to wait rather than offering an immediate retry, and a handoff
  that fails after activation offers a retry that never posts the spent
  token again.
- `LoginScreen` takes an optional `onForgotPassword`, rendering the way
  to the reset request under the form when given.
- The testkit cans the four new routes, and the login fields carry
  `username` and `current-password` autocomplete hints.

### Changed

- **Breaking.** `AuthTransport` requires `invite`, `acceptInvite`,
  `requestPasswordReset` and `resetPassword`. A consumer implementing the
  whole interface adds them, one overriding part of it through
  `configureAuthTransport` is untouched.
- **Breaking.** `NewUserScreen` sends an invitation instead of creating
  an account, so it has no password field and posts to
  `POST /api/users/invite`. No released server serves that route yet, a
  future `authkit` release does. `createUser` and its route are
  unchanged for consumers calling them directly.
- `ValidationError` is declared on the package entry point and
  re-exported from `./admin`, so both import paths keep working.

## [0.7.0] - 2026-08-26

### Changed

- The optional `@testing-library/jest-dom` peer admits 7 alongside 6,
  and the package's own suite runs against 7.

## [0.6.0] - 2026-08-22

### Changed

- **Breaking.** The concept is named role, never rank. `rank` becomes
  `role` on the session user, on every listed account and on the account
  `createUser` creates, `setUserRank` becomes `setUserRole`, and the
  transport method it resolves is `setUserRole`.
- **Breaking.** `RankRefusedError` becomes `RoleRefusedError` and
  `SelfRankError` becomes `SelfRoleError`, thrown for the renamed
  refusal codes `role_insufficient` and `self_role_refused`.
- **Breaking.** The route a role write takes is
  `PUT /api/users/{id}/role`, carrying `role` in its body. A server
  older than `authkit` v0.9.0 answers it with 404.
- **Breaking.** `rankedUser` becomes `userWithRole` and `rankOk` becomes
  `roleOk` in the testing harness. No alias keeps the old names.

## [0.5.0] - 2026-08-21

### Added

- `rank` on the session and on every listed account, empty when the
  server sends none, so a screen can tell one account apart from another.
- `rank` on the account a create names, and `setUserRank` writing the
  rank an account holds.
- `RankRefusedError`, `SelfRankError` and `LastPrivilegedError`, thrown
  for the refusals the server answers a rank write with.
- `rankedUser` and `rankOk` in the testing harness, canning an account
  per rank and answering a rank change, so a consumer can walk both
  sides of a gate.

### Changed

- `AuthTransport` carries `setUserRank`, so a consumer replacing the
  transport implements it.

## [0.4.0] - 2026-08-17

### Added

- Every rendered string is translatable under the text domain
  `gopherium-react-auth`, exported as `DOMAIN`.
- `catalogFor(locale)` and the `Catalog` type, reading the compiled
  catalogue the package ships under `dist/languages`.
- An `es-ES` catalogue covering all 32 messages.

### Changed

- `usersNavItem.label` became a getter, so it follows the catalogue the
  consumer loaded.
- `@wordpress/i18n` is a required peer at `>=6.26.0 <7.0.0`.
- The `Enable` and `Disable` row labels carry the name as a placeholder
  instead of joining it.

## [0.3.0] - 2026-08-08

### Changed

- The log in and log out buttons show the design system busy state while
  their request is in flight, instead of only dimming.
- The `@wordpress/ui` peer range narrowed to `>=0.19.0 <0.20.0`, the one
  window this release is built and tested against. The busy state needs
  the `loading` prop, so a consumer on an older design system would have
  seen a button that neither spins nor blocks a second click.

## [0.2.0] - 2026-08-06

### Added

- `configureAuthTransport`, `resetAuthTransport`, and the `AuthTransport`
  type. Every backend call of the package resolves through a configurable
  transport whose default is the unchanged REST implementation, so a
  consumer can swap transports without touching the screens, hooks, or
  error contract.

## [0.1.2] - 2026-07-30

### Fixed

- Widened the `@wordpress/ui` peer range to `>=0.17.0 <1.0.0`. It was
  `^0.17.0`, which semver reads as `>=0.17.0 <0.18.0`, so every consumer on
  0.18 or 0.19 was outside the declared range. Nothing warned, because the
  peer is optional. Verified green against both 0.17.0 and 0.19.0.

- Declared `@wordpress/theme` as an optional peer at `>=1.0.0 <2.0.0`. The
  stylesheet at `react-auth/wpds/style.css` reads three `--wpds-` design
  tokens, so a consumer without the theme rendered it unstyled with no
  warning. Verified against 1.1.0.

### Changed

- Develops against `@wordpress/ui` 0.19.0, `@wordpress/theme` 1.1.0 and
  `@wordpress/element` 8.4.0, matching the train both consuming products run.

### Added

- A test asserting every design token the stylesheet reads is declared by
  the installed `@wordpress/theme`, so a train bump that renames a token
  fails the build instead of degrading silently.

## [0.1.1] - 2026-07-16

### Fixed

- Relative imports in the published output carry explicit extensions,
  so Node's ESM resolution can load the package outside a bundler.

## [0.1.0] - 2026-07-16

### Added

- Headless core: `fetchSession`, `login`, `logout`, typed auth errors,
  `useSession`, `useLogout`, `sessionQueryKey`, the slot-based
  `AuthGate`, `createAuthQueryClient`, and `isSessionRevoked`.
- `/admin`: `fetchUsers`, `createUser`, `setUserDisabled`,
  `usersQueryKey`, and typed admin errors.
- `/wpds`: `LoginScreen` with a brand prop, `AccountPanel`,
  `UsersScreen`, `NewUserScreen`, `usersNavItem`, and their stylesheet.
- `/testing`: the msw `server`, `installTestEnvironment`,
  `seedSession`, `defaultUser`, and canned auth endpoint handlers.
