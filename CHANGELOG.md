# Changelog

All notable changes to this project are documented in this file. The
format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project follows [Semantic Versioning](https://semver.org/). While
at v0.x, minor releases may contain breaking changes.

## [0.1.0] - 2026-07-10

### Added

- `User` and `NewUser` for validated account creation.
- argon2id password hashing with constant-time `VerifyPassword`.
- `Session`, `NewSession`, and `HashToken` for opaque login tokens
  stored only as digests.
- Storage-agnostic `Store` interface with sentinel errors.

[0.1.0]: https://github.com/gopherium/gouncer/releases/tag/v0.1.0
