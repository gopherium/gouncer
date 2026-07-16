# Changelog

All notable changes to the `authkit/ratelimit` module are documented in
this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the module
follows [Semantic Versioning](https://semver.org/). While at v0.x, minor
releases may contain breaking changes.

Releases of this module are tagged `authkit/ratelimit/vX.Y.Z`.

## [0.1.0] - 2026-07-16

### Added

- `Middleware` limiting failed login attempts per client IP, counting
  only 401 responses and failing closed on counter errors.
- `Config` with a configurable limit, window, and trusted proxies.
- `ParseTrustedProxies` validating comma-separated CIDR lists.

[0.1.0]: https://github.com/gopherium/gouncer/releases/tag/authkit%2Fratelimit%2Fv0.1.0
