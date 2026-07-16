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
- `VerifyPassword` checks a password against a stored hash. It never
  panics and a malformed hash never matches.
- `NewSession` issues a login session with a random token. The client
  sees the token once and only its digest is stored.
- `HashToken` digests a token for storage and lookup.
- `Store` is the persistence contract. Bring your own database and
  return the package sentinel errors.

## Design

Gouncer is a set of authentication primitives. It stays out of your
transport, routing, and storage decisions, and grows by adding small,
independent building blocks. You adopt only what you need.

## Batteries

Ready-made batteries live in this repository as separately versioned modules:

- [`authkit`](authkit/) serves gouncer sessions over HTTP.
- [`authkit/ratelimit`](authkit/ratelimit/) limits failed login attempts per client IP.

Adopt them or write your own transport against the same primitives.

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

The [batteries](#batteries) implement these notes as maintained modules.
Adopt them or keep the notes as your checklist.

## Reporting security issues

Privately, please. See [SECURITY.md](SECURITY.md).

## License

Apache-2.0. Copyright © 2026 Manuel 'SirLouen' Camargo.
