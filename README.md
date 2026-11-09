# Gouncer

**The Go bouncer.** Composable authentication primitives for Go,
framework-free and storage-agnostic. You assemble them in your own
handlers and database. Gouncer owns none of your HTTP layer.

> **Stability: v0.** The API may change between minor releases while
> the project matures. Pin a version and read the
> [CHANGELOG](CHANGELOG.md) before upgrading. Production use is at your
> own risk until v1.

## API

- `NewUser` validates registration input and hashes the password with argon2id.
- `NewInvitedUser` builds an unconfirmed account with no usable
  password until it is activated.
- `VerifyPassword` checks a password against a stored hash. It never
  panics and a malformed hash never matches.
- `NewSession` issues a login session with a random token. The client
  sees the token once and only its digest is stored.
- `NewToken` issues a single-use token that invites, resets or
  confirms an account. Only its digest is stored too.
- `HashToken` digests a session or single-use token for storage and lookup.
- `Roles` is a set of roles, and `Holds` reports whether a role is one of them.
- `Store` is the persistence contract. Bring your own database and
  return the package sentinel errors.
- [`avatar`](avatar/) builds an account's avatar address, a drawn
  initial or a Gravatar image, with the standard library alone.

## Design

Gouncer is a set of authentication primitives. It stays out of your
transport, routing, and storage decisions, and grows by adding small,
independent building blocks. You adopt only what you need.

## Batteries

Ready-made batteries live in this repository as separately versioned modules:

- [`authkit`](authkit/) serves gouncer sessions and invitations over HTTP and refuses cross-origin browser writes.
- [`authkit/postgres`](authkit/postgres/) persists users and sessions in a PostgreSQL schema of its own.
- [`authkit/sqlite`](authkit/sqlite/) persists users and sessions in SQLite tables on an application's one handle.
- [`authkit/ratelimit`](authkit/ratelimit/) limits failed login attempts per client IP.
- [`react-auth`](react-auth/) is the npm client, `@gopherium/react-auth`, for React frontends.

Adopt them or write your own transport against the same primitives.
Guides and operational recipes live at
[docs.gopherium.org](https://docs.gopherium.org).

## Usage

```go
// Registration.
u, err := gouncer.NewUser("ada@example.com", "Ada Lovelace", "correct horse battery")
if err != nil {
    // errors.Is against the package Err* sentinels.
}
err = store.CreateUser(ctx, u) // gouncer.ErrEmailTaken on duplicates

// Login.
u, err = store.UserByEmail(ctx, email)
if err != nil || !gouncer.VerifyPassword(u.PasswordHash, password) || u.Disabled {
    // Reject with one generic "invalid credentials" answer.
}
s, err := gouncer.NewSession(u.ID)
err = store.CreateSession(ctx, s)
// Hand s.Token to the client, persist only s.TokenHash.

// Authenticating a request.
u, err = store.UserBySession(ctx, gouncer.HashToken(token), time.Now().UTC())
// gouncer.ErrSessionNotFound when unknown, expired, or the user is disabled.

// Logout.
err = store.DeleteSession(ctx, gouncer.HashToken(token))
```

## Security notes for integrators

- Equalize login timing. When `UserByEmail` misses, verify against a
  fixed dummy hash so unknown and known emails cost the same.
- Serve session tokens in `HttpOnly`, `Secure`, `SameSite` cookies with
  the `__Host-` prefix. Never log the plain token.
- Rate limit your login endpoint. Password verification is expensive by
  design.
- Refuse cross-origin browser writes at the root of your router, before
  any route, for example with `authkit.CrossOriginGuard`. Never change
  data on `GET`, `HEAD` or `OPTIONS`, which it lets through.
- Send `Strict-Transport-Security` from your HTTPS site. An older browser
  without `Sec-Fetch-Site` is judged by host alone, never by scheme.
- Behind a reverse proxy, pass the visitor's `Host` header through
  unchanged. The guard reads no forwarded headers, so a browser that
  sends `Origin` without `Sec-Fetch-Site` has its writes refused behind a
  proxy that rewrites `Host`.

The [batteries](#batteries) implement these notes as maintained modules.
Adopt them or keep the notes as your checklist.

## Reporting security issues

Privately, please. See [SECURITY.md](SECURITY.md).

## License

Apache-2.0. Copyright © 2026 Manuel 'SirLouen' Camargo.
