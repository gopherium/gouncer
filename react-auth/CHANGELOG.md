# Changelog

All notable changes to `@gopherium/react-auth` are documented in this
file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the
package follows [Semantic Versioning](https://semver.org/). While at
0.x, minor releases may contain breaking changes.

Releases are tagged `react-auth@X.Y.Z` and publish to npm from CI. The
npm-style tag stays invisible to the Go toolchain, unlike a
`react-auth/vX.Y.Z` tag naming the directory's stub module.

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
