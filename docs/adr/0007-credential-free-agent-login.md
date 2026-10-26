# 0007 — Credential-free agent login is a build-tagged, loopback-only route

Status: `Accepted`
Date: `2026-10-15`
Raised by: `T-035` (Q-032)
Decided by: `owner` (D-042), on the architect's recommendation

## Context

- **Why agents need a login.** D-041 lets agents check the UI in a browser
  against a disposable local server (`scripts/agent-server.ps1`). Every
  session needs a password: `POST /api/v1/login` is the only anonymous API
  path (`internal/httpapi/httpapi.go:113`).
- **Why a password does not work.** The Claude Code permission classifier
  refused an agent reading the generated password, as "Credential
  Materialization". Agents must not work around such a refusal.
- **What already exists.** The repository uses build tags in one place,
  with two files: `web/embed.go` (`ui`) and `web/embed_noui.go` (`!ui`).
  `scripts/build.ps1` is the only release build, and passes `-tags ui`.
- **Why a loopback check is not enough.** `trustedProxies` exists, so a
  same-host reverse proxy is an anticipated deployment. Behind one, every
  request arrives from loopback.

## Options

1. **A permission rule for the password file** (an owner action). This is
   uncertain: the classifier may also refuse typing the password. And a
   password sitting in the agent's context is the risk D-041 meant to
   avoid.
2. **A build-tagged, credential-free route.** It is compiled only with
   `-tags agentdev`, and issues a session for the `agent` user to loopback
   callers.
3. **A one-time login URL, or a cookie the script mints.** Either still
   puts a credential in the agent's hands, and the route would ship unless
   it too were tagged.
4. **Do nothing.** Browser checks behind the login stay owner checks.

## Decision

Option 2. It removes the credential instead of moving it, and it follows
the existing build-tag pattern.

- **The route.** `GET /api/v1/agent-login` exists only in an `agentdev`
  build.
  - It refuses any non-loopback raw `RemoteAddr`, and ignores
    `X-Forwarded-For`.
  - It issues a session only for an existing, enabled user named `agent`.
  - It answers with a redirect to `/`.
- **The listen guard.** A tagged `serve` refuses to start on a non-loopback
  listen address.

## Consequences

- **Agents can check authenticated screens** in a browser without ever
  handling a secret.
- **The barrier that matters** is that `agentdev` never appears in
  `scripts/build.ps1` or in any deployment. The loopback checks reduce
  exposure but do not help behind a same-host reverse proxy. A future
  change to the build or deploy scripts must keep the tag out.
- **CI tests both tag states.** `scripts/test.ps1` runs `go test ./...` and
  `go test -tags agentdev ./...`.
- **Future auth work must know the exception.** This is a standing
  exception to "every session comes from a password". Any change to
  session issuance must keep the tagged route's guards, or retire it.
