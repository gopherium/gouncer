# Changelog

All notable changes to `@gopherium/react-auth` are documented in this
file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the
package follows [Semantic Versioning](https://semver.org/). While at
0.x, minor releases may contain breaking changes.

Releases are tagged `react-auth@X.Y.Z` and publish to npm from CI. The
npm-style tag stays invisible to the Go toolchain, unlike a
`react-auth/vX.Y.Z` tag naming the directory's stub module.

## [0.1.2]

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
