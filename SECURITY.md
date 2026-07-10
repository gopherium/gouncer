# Security Policy

## Reporting a vulnerability

**Do not open a public issue for security problems.**

Report vulnerabilities privately, either through
[GitHub private vulnerability reporting](https://github.com/gopherium/gouncer/security/advisories/new)
or by email to <info@gopherium.com>.

You will receive an acknowledgement within 7 days. Please include a
description of the issue, a proof of concept if you have one, and the
version or commit you tested against. Coordinated disclosure is
appreciated: give us a chance to ship a fix before publishing details.

## Supported versions

Only the latest release receives security fixes. While the project is
at v0.x there are no backport guarantees.

## Scope

In scope: flaws in the code this module ships. Out of scope: how a
consuming application wires gouncer into a transport. Cookie attributes,
CSRF, rate limiting, timing-equalized login flows, and similar policy
choices are the application's responsibility, and the README documents
the recommended patterns.
