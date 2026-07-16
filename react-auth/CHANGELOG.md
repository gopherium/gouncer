# Changelog

All notable changes to `@gopherium/react-auth` are documented in this
file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the
package follows [Semantic Versioning](https://semver.org/). While at
0.x, minor releases may contain breaking changes.

The package version lives in `package.json`; releases publish to npm
and carry no git tags.

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
