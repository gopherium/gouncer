# Changelog

All notable changes to `@gopherium/react-auth` are documented in this
file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the
package follows [Semantic Versioning](https://semver.org/). While at
0.x, minor releases may contain breaking changes.

Releases are tagged `react-auth@X.Y.Z` and publish to npm from CI. The
npm-style tag stays invisible to the Go toolchain, unlike a
`react-auth/vX.Y.Z` tag naming the directory's stub module.

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
