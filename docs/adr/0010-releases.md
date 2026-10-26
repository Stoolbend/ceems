# 0010 — Releases are cut by pushing a semver tag; one build script stamps the footer; per-platform archives

Status: `Accepted`
Date: `2026-10-25`
Raised by: owner request, 2026-10-25
Decided by: `owner` (D-069), on the architect's design record in `T-073`

## Context

- CI only tests (`.github/workflows/ci.yml`). `scripts/build.ps1` is the
  only release build (ADR 0007) and never set the footer version, so every
  build showed `dev` (`web/src/App.vue:10`).
- The owner wants releases only when wanted, numbered in semantic
  versioning, as archives a user extracts and uses, with the version in the
  footer. Only the owner pushes.

## Options

1. **Push a `vX.Y.Z` tag.** The owner already pushes; no bot writes refs.
2. **A manual run with a version input that creates the tag.** Needs
   `contents: write` to push a tag and a bot-authored tag; adds nothing for
   a single owner.
3. **Release branches or `develop`.** Two branches to keep in step for one
   person.

## Decision

Option 1. `release.yml` runs on `v*` tags and on manual dispatch. It fails
unless the tag matches
`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$`,
runs the tests on the tagged commit, builds with
`scripts/build.ps1 -Version <tag>` on Ubuntu (so tarballs keep the
executable bit), and publishes with `gh release create`. A manual run is
stamped with the 7-character commit hash and uploads the archives without a
release. Local builds stamp `git describe --tags --always --dirty`.

## Consequences

- One string (the tag) names the release, the archives and the footer.
- `build.ps1` must stay portable PowerShell: it runs under `pwsh` on Linux.
- Archive names and contents (D-069) are a convention the README and
  `deploy/README.md` document; change them only with this ADR.
- The archive ships `ceems.example.json`, never `ceems.json`, so an upgrade
  extracted over an install keeps the user's settings.
- Not proved until the owner pushes a tag; linux test failures are not seen
  by CI, which runs on Windows.
- `agentdev` stays out of `build.ps1`, `release.yml` and `deploy/`
  (ADR 0007).
