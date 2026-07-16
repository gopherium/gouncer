# @gopherium/react-auth

Session authentication for React apps backed by gouncer's authkit.

## Entry points

- `@gopherium/react-auth` is the headless core: the auth API client,
  `useSession`, `useLogout`, `sessionQueryKey`, the slot-based
  `AuthGate`, `createAuthQueryClient`, and `isSessionRevoked`.
- `@gopherium/react-auth/admin` is the user administration API client.
- `@gopherium/react-auth/wpds` renders the WordPress Design System UI:
  `LoginScreen` (brand prop), `AccountPanel`, `UsersScreen`,
  `NewUserScreen`, and `usersNavItem`. Import
  `@gopherium/react-auth/wpds/style.css` alongside it.
- `@gopherium/react-auth/testing` is the msw harness: `server`,
  `installTestEnvironment`, `seedSession`, `defaultUser`, and canned
  handlers for the auth endpoints.

## Mount

```tsx
const client = createAuthQueryClient()

<QueryClientProvider client={client}>
  <AuthGate loginScreen={(onLogin) => <LoginScreen brand="MyApp" onLogin={onLogin} />}>
    <App />
  </AuthGate>
</QueryClientProvider>
```

## Consumer requirements

- `react` and `@tanstack/react-query` are peer dependencies; the app
  must own exactly one copy of each. With workspace or linked installs,
  add both to your bundler's dedupe list.
- The client calls same-origin `/api/auth/*` and `/api/users` paths. In
  development, proxy `/api` to your backend so the `__Host-` session
  cookie is first-party.
- `/wpds` needs the `@wordpress/ui` peer and, on React 19, a
  `@wordpress/element` patch stubbing the react-dom exports React 19
  removed. See `patches/` in this package's repository directory.

## License

Apache-2.0. Copyright © 2026 Manuel 'SirLouen' Camargo.
