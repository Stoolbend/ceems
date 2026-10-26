# CeeMS

A web based content management system for vbit2/.tti file Teletext systems. Includes features like: 
- Page editor (with copy & paste, mouse draw and importing from existing TTI files or <https://zxnet.co.uk/teletext/editor/> URLs)
- Templates with data slots, filled from Google Sheets on an hourly schedule or on demand (for example high-score pages), with a run log and failed runs keeping the previous page
- Fastext groups (static or paged) with a central index link, kept consistent across every member page
- vbit2 settings management & output to `vbit.conf`
- Multiple users with per-magazine, per-page or page-range read and write grants
- A scoped REST API with revocable API keys, rate limiting and an audit log of writes
- Single-binary releases for Windows, Linux and Raspberry Pi, with a systemd deployment kit

## Deployment instructions

### Download

Take the latest release from the GitHub releases page of this repository and
pick the archive for your machine:

- `ceems-<version>-windows-amd64.zip` for Windows.
- `ceems-<version>-linux-arm64.tar.gz` for a Raspberry Pi.
- `ceems-<version>-linux-amd64.tar.gz` for a Linux PC or server.
- `ceems-<version>-linux-armv6.tar.gz` for a Raspberry Pi 1 or Pi Zero, 32-bit OS.

Each archive also holds `LICENSE` and `THIRD_PARTY_NOTICES.txt`.

Extract it, copy `ceems.example.json` to `ceems.json` and edit it (see
Configure), then create the first admin and start the server:

```sh
./ceems admin create <username>
./ceems serve
```

On Windows run `.\ceems.exe` instead of `./ceems`. The footer of the web app
shows the release version.

The archive never contains `ceems.json`, so extracting an upgrade over an
existing install keeps your settings.

### Build

Developers can build from source. From PowerShell in the repository root:

```powershell
powershell -NoProfile -File scripts/build.ps1
```

This builds the web app and embeds it in four binaries:
`dist/ceems-windows-amd64.exe`, `dist/ceems-linux-arm64` (the Pi),
`dist/ceems-linux-amd64` and `dist/ceems-linux-armv6` (Pi 1 and Pi Zero).
The footer shows the output of
`git describe --tags --always --dirty`; pass `-Version <text>` to override it.

### Releasing

Push a `vMAJOR.MINOR.PATCH` tag (a suffix such as `-rc.1` makes it a
pre-release):

```sh
git tag v0.1.0
git push origin v0.1.0
```

The Release workflow tests, builds the four archives and publishes the
GitHub release. Running the workflow by hand builds the archives without
releasing; their names and the footer use the short commit hash.

### Configure

The program reads `ceems.json` from the current directory, or the file named
by `-config`. Copy `ceems.example.json` and edit it:

```json
{
  "listen": "127.0.0.1:8080",
  "dataDir": "data",
  "pagesDir": "data/pages-out",
  "logLevel": "info",
  "trustedProxies": [],
  "googleKeyFile": "path/to/google-service-account-key.json"
}
```

- Relative paths resolve against the directory the program runs from.
  `/ceems.json` and `/data/` are git-ignored.
- `pagesDir` receives the transmit pages. It must be new, empty, or one
  CeeMS created before. A directory holding other files is refused.
- `googleKeyFile` is optional. Without it the Google Sheets source is off.
  Never commit a key file; keep it outside the repository or under the
  ignored `testdata/secrets/`.
- Set `secureCookies` to `true` only when the site is served over HTTPS.
- Read `docs/internet-exposure.md` before exposing the server beyond your
  own machine.

### First run

```powershell
.\dist\ceems-windows-amd64.exe admin create <username>
.\dist\ceems-windows-amd64.exe import testdata\pages
.\dist\ceems-windows-amd64.exe serve
```

- `admin create` prints a random password once. Copy it.
  `admin reset-password <username>` issues a new one.
- `import` is optional. It loads a directory of `.tti` files; `testdata/sample/pages`
  holds 30 sample pages.
- `serve` starts the server. Open http://127.0.0.1:8080 and sign in. Stop it
  with Ctrl+C.

### High-score pages

Follow `docs/highscores.md`:

1. Set `googleKeyFile`.
2. Upload `testdata/sample/highscores/template.tti` as a template.
3. Create one binding per game from the table in that guide.
4. Press **Run now**.

The generated pages appear in `pagesDir`. Changing a binding's page number
moves the page and renames its file.

### Raspberry Pi

`deploy/README.md` is the full guide. It covers:

- copying the Linux binary to the Pi;
- the `ceems` service user;
- the systemd unit (`deploy/ceems.service`) and config
  (`deploy/ceems.json`);
- creating the first admin;
- pointing vbit2 at the pages directory;
- upgrading and logs.

## Development

You need Go (see `go.mod`) and Node.js with npm.

### Run with live reload

Terminal 1, the API server (uses `ceems.json` as above):

```powershell
go run ./cmd/ceems serve
```

Terminal 2, the web app:

```powershell
cd web
npm ci
npm run dev
```

Open the URL Vite prints, usually http://localhost:5173. Vite forwards `/api`
and `/healthz` to the server on port 8080. A `go run` build without the `ui`
tag serves a "UI not built" placeholder on port 8080, so use the Vite URL for
the UI.

### Tests

```powershell
powershell -NoProfile -File scripts/test.ps1
```

This runs `go vet`, `go test ./...`, the web tests (vitest) and the type
check.

- For one area, run `go test ./internal/<package>/` or `npm test` in `web/`.
- Some tests are timing-sensitive. Run one suite at a time; parallel suites
  on a loaded machine can fail them (see `docs/known-issues.md`).

### Agent and developer UI checks

```powershell
powershell -NoProfile -File scripts/agent-server.ps1 start [-Seed <dir of .tti files>]
powershell -NoProfile -File scripts/agent-server.ps1 stop -RunDir <run dir>
```

`start` builds the app with `-tags "ui agentdev"` and runs a throwaway server
on `127.0.0.1` with a disposable admin named `agent`. Its config, data and
pages live in a fresh directory under the system temp path. To log in, open
the printed `login url:`; it needs no password and answers only loopback
callers. `stop` ends the server by its recorded PID and deletes the
directory. This is for local agent and developer UI checks; see D-041 and
D-042 in `docs/decisions.md`. Release builds never use the `agentdev` tag.

### Project documents

- `docs/plan.md`: milestones and acceptance criteria.
- `docs/decisions.md`: decisions and open questions.
- `docs/api.md`: the REST API.
- `docs/adr/`: architecture decision records.
- `AGENTS.md`: the working agreement for agent runs.

## License

CeeMS is free software under the GNU Affero General Public License, version 3
or later (`AGPL-3.0-or-later`). Copyright © 2026 Conor Morgan and CeeMS
contributors. The full text is in `LICENSE`. The licences of the third-party
components CeeMS includes are in `THIRD_PARTY_NOTICES.txt`, which the
releases ship and the running server serves at `/THIRD_PARTY_NOTICES.txt`; the
credits dialog summarises them. `scripts/notices.ps1` regenerates the file
after a dependency changes, and `scripts/test.ps1` fails if it is stale.
