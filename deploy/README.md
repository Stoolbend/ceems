# Installing ceems on a Raspberry Pi

This sets up ceems on a Raspberry Pi 4 running Raspberry Pi OS Bookworm Lite
64-bit, beside an unmodified vbit2. ceems writes the transmit pages as `.tti`
files into `/run/ceems-pages`; vbit2 reads them from there.

Files in this directory:

- `ceems.service`: the systemd unit.
- `ceems.json`: the example config.

Anything about vbit2 that this repository cannot confirm is marked
**check on the Pi**. Check it against vbit2's own documentation.

## 1. Flash the image

Flash Raspberry Pi OS Bookworm Lite 64-bit to the SD card with Raspberry Pi
Imager. Enable SSH and set a user in the imager's settings. Boot the Pi and
update it with `sudo apt update && sudo apt full-upgrade`.

The Pi 4 is the supported board. The Pi 5 is not supported by vbit2 or
raspi-teletext upstream.

A Pi 1 or Pi Zero (ARMv6) needs 32-bit Raspberry Pi OS Lite and the
`ceems-vX.Y.Z-linux-armv6.tar.gz` archive instead of the arm64 one. ceems on
these boards is known to run on a Pi 1. Whether vbit2 and raspi-teletext run
there must be checked against their own documentation.

## 2. Install vbit2 and raspi-teletext

Follow each project's own instructions. Do not copy steps from here.

- vbit2: <https://github.com/peterkvt80/vbit2>
- raspi-teletext: <https://github.com/ali1234/raspi-teletext>

## 3. Get the binary

Pick one route. Both end with the files under `/tmp/ceems` on the Pi.

### On the Pi

The archive has no top-level directory, so extract it into its own directory.
Replace `vX.Y.Z` with the release version. A Pi 1 or Pi Zero uses the
`linux-armv6` archive instead of `linux-arm64`.

```sh
mkdir /tmp/ceems && cd /tmp/ceems
curl -LO https://github.com/Stoolbend/ceems/releases/download/vX.Y.Z/ceems-vX.Y.Z-linux-arm64.tar.gz
tar xzf ceems-vX.Y.Z-linux-arm64.tar.gz
```

It holds `ceems`, `ceems.example.json`, `LICENSE`, `THIRD_PARTY_NOTICES.txt` and the files from this directory in
`deploy/`.

### From a Windows PC

Download `ceems-vX.Y.Z-linux-arm64.tar.gz` from the GitHub releases page
and extract it on the PC:

```sh
tar xzf ceems-vX.Y.Z-linux-arm64.tar.gz
```

It holds `ceems`, `ceems.example.json`, `LICENSE`, `THIRD_PARTY_NOTICES.txt` and the files from this directory in
`deploy/`.

Alternatively build it yourself. From the repository root, in PowerShell:

```powershell
scripts/build.ps1
```

This produces `dist/ceems-linux-arm64`.

## 4. Install the binary

From a Windows PC, first copy the files to the Pi. Run this in the directory
you extracted the archive into, or in the repository root for a local build:

```powershell
ssh <user>@<pi-address> mkdir -p /tmp/ceems/deploy
scp ceems <user>@<pi-address>:/tmp/ceems/
scp deploy/ceems.service deploy/ceems.json <user>@<pi-address>:/tmp/ceems/deploy/
```

(For a local build, copy `dist/ceems-linux-arm64` instead of `ceems`, and name
it `ceems` on the Pi: `scp dist/ceems-linux-arm64 <user>@<pi-address>:/tmp/ceems/ceems`.)

Then, on the Pi, for either route:

```sh
sudo install -m 0755 /tmp/ceems/ceems /usr/local/bin/ceems
sudo install -m 0755 /tmp/ceems/deploy/update.sh /usr/local/bin/ceems-update
```

## 5. Create the service user

```sh
sudo useradd --system --no-create-home --shell /usr/sbin/nologin ceems
```

## 6. Install the unit, the config and the key file

On the Pi, with the files from steps 3 and 4 in `/tmp/ceems`:

```sh
sudo install -m 0644 /tmp/ceems/deploy/ceems.service /etc/systemd/system/ceems.service
sudo install -d -m 0755 /etc/ceems
sudo install -m 0644 /tmp/ceems/deploy/ceems.json /etc/ceems/ceems.json
sudo install -m 0600 -o ceems -g ceems google-api-key.json /etc/ceems/google-api-key.json
```

`google-api-key.json` is the Google service-account key. Do not commit it. If
you have no key yet, remove the `googleKeyFile` line from
`/etc/ceems/ceems.json`; the Sheets source stays off until you add it.
ceems refuses to start if `googleKeyFile` names a missing file.

Settings in `/etc/ceems/ceems.json`:

- `listen` is `127.0.0.1:8080`, reachable only from the Pi. To reach the
  interface directly, with no proxy, set it to the Pi's ZeroTier or LAN
  address and port.
- When you put a TLS reverse proxy in front, keep `listen` on loopback, set
  `secureCookies` to `true` and list the proxy's address in `trustedProxies`.
  See `docs/internet-exposure.md` for the full list of requirements.
- `dataDir` is `/var/lib/ceems`, created by the unit (`StateDirectory`).
- `pagesDir` is `/run/ceems-pages`, created by the unit
  (`RuntimeDirectory`). `/run` is a tmpfs, so the directory is empty after a
  reboot and ceems regenerates the pages when it starts. A service restart
  leaves the files in place, so vbit2 keeps reading the last pages.

Once the files are installed, remove the staging directory:

```sh
rm -r /tmp/ceems
```

## 7. Start the service

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now ceems
systemctl status ceems
```

## 8. Create the first admin

```sh
sudo -u ceems ceems admin create -config /etc/ceems/ceems.json <username>
```

The password is random and printed once. Note it now. To set a new one later,
use `ceems admin reset-password` with the same arguments.

## 9. Import existing pages (optional)

Copy a folder of `.tti` files to the Pi and run:

```sh
sudo -u ceems ceems import -config /etc/ceems/ceems.json <dir>
```

The `ceems` user must be able to read `<dir>`.

The import does not notify the running service, so the pages reach
`/run/ceems-pages` only at the next page change or restart. Restart it now:

```sh
sudo systemctl restart ceems
```

## 10. Point vbit2 at the pages

Configure vbit2 to read `/run/ceems-pages`. The name of its pages-directory
option, the file it lives in and how vbit2 is started as a service are
**check on the Pi**. Look in vbit2's documentation and in the installed
config. Restart vbit2 after changing it.

ceems writes the pages within about a minute of a change (D-018), so give
vbit2 that long to notice them. Whether vbit2 reloads changed files by itself
or needs a restart is also **check on the Pi**.

## 11. Check

1. List the pages: `ls /run/ceems-pages`. The `.tti` files are there.
2. Check that the pages appear on the teletext output.
3. Reboot: `sudo reboot`.
4. After the reboot, check again that the pages appear. The directory starts
   empty and ceems refills it. Whether vbit2 starts cleanly while the
   directory is still empty is **check on the Pi**.

## Upgrading

On a Pi installed as above, one command downloads the latest release for the
Pi's architecture, stops the service, installs the binary and starts the
service again, then reports whether it came back up:

```sh
sudo ceems-update            # latest release
sudo ceems-update v1.2.3     # a given version (prereleases too)
sudo ceems-update -f         # reinstall even if already up to date
```

It needs internet access to GitHub. The archive is checked only by HTTPS: no
checksums are published. The previous binary is kept as
`/usr/local/bin/ceems.prev`, and the installed tag is recorded in
`/var/lib/ceems/installed-version`. There is no automatic rollback; if the
service does not come up, the script prints the manual rollback steps.

The script also updates itself from the archive.

To upgrade by hand instead, get the new release's files into `/tmp/ceems` by
either route of steps 3 and 4. Only the binary is needed; skip `deploy/`. Then:

```sh
sudo systemctl stop ceems
sudo install -m 0755 /tmp/ceems/ceems /usr/local/bin/ceems
sudo systemctl start ceems
rm -r /tmp/ceems
```

If the new version migrates the database, it first copies it to
`/var/lib/ceems/ceems-pre-vN.db`, where `N` is the new schema version.
Keep that file until you are satisfied with the upgrade.

## Logs

```sh
journalctl -u ceems
journalctl -u ceems -f
```
