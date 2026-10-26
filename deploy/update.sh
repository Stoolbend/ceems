#!/bin/sh
# Updates ceems to the latest (or a given) GitHub release. See deploy/README.md.
# Usage: ceems-update [-f] [vX.Y.Z]
set -eu

CEEMS_BIN=${CEEMS_BIN:-/usr/local/bin/ceems}
CEEMS_UNIT=${CEEMS_UNIT:-ceems}
CEEMS_DATA=${CEEMS_DATA:-/var/lib/ceems}
CEEMS_REPO=${CEEMS_REPO:-https://github.com/Stoolbend/ceems}

die() { echo "ceems-update: $*" >&2; exit 1; }

force=0
if [ "${1:-}" = "-f" ]; then force=1; shift; fi
[ $# -le 1 ] || die "usage: ceems-update [-f] [vX.Y.Z]"

[ "$(id -u)" = "0" ] || die "must run as root (use sudo)"
for c in curl tar systemctl uname; do
    command -v "$c" >/dev/null 2>&1 || die "$c is required but not installed"
done

case "$(uname -m)" in
    aarch64|arm64) arch=arm64 ;;
    x86_64) arch=amd64 ;;
    armv6l|armv7l) arch=armv6 ;;
    *) die "unsupported architecture: $(uname -m)" ;;
esac

tag=${1:-}
if [ -z "$tag" ]; then
    url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$CEEMS_REPO/releases/latest") ||
        die "could not look up the latest release"
    tag=${url##*/}
fi
case "$tag" in
    v[0-9]*.[0-9]*.[0-9]*) ;;
    *) die "not a release tag: $tag" ;;
esac
# Strip an optional -suffix, then require exactly digits.digits.digits.
core=${tag#v}; core=${core%%-*}
case "$core" in
    *[!0-9.]*|*..*|.*|*.) die "not a release tag: $tag" ;;
esac
[ "$(echo "$core" | tr -cd . | wc -c)" -eq 2 ] || die "not a release tag: $tag"

versionfile=$CEEMS_DATA/installed-version
old=unknown
[ ! -f "$versionfile" ] || old=$(cat "$versionfile")
if [ "$old" = "$tag" ] && [ "$force" -eq 0 ]; then
    echo "ceems $tag is already installed"
    exit 0
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM
echo "Downloading ceems $tag ($arch)..."
curl -fsSL -o "$tmp/ceems.tar.gz" "$CEEMS_REPO/releases/download/$tag/ceems-$tag-linux-$arch.tar.gz" ||
    die "download failed"
tar -xzf "$tmp/ceems.tar.gz" -C "$tmp" || die "could not extract the archive"
[ -f "$tmp/ceems" ] && [ -s "$tmp/ceems" ] || die "archive has no usable ceems binary"

started=$tmp/started
: > "$started"
bindir=$(dirname "$CEEMS_BIN")
[ ! -f "$CEEMS_BIN" ] || cp "$CEEMS_BIN" "$CEEMS_BIN.prev"
rollback_help() {
    cat >&2 <<MSG

The previous binary is at $CEEMS_BIN.prev. To roll back:
  sudo systemctl stop $CEEMS_UNIT
  sudo install -m 0755 $CEEMS_BIN.prev $CEEMS_BIN
MSG
    for db in $(find "$CEEMS_DATA" -name 'ceems-pre-v*.db' -newer "$started" 2>/dev/null); do
        echo "  sudo cp $db $CEEMS_DATA/ceems.db   # the update migrated the database" >&2
    done
    echo "  sudo systemctl start $CEEMS_UNIT" >&2
}

# Past this point the service is stopped: a failing step must say so and show the way back.
systemctl stop "$CEEMS_UNIT" || die "could not stop $CEEMS_UNIT"
if ! install -m 0755 "$tmp/ceems" "$CEEMS_BIN"; then
    echo "ceems-update: could not install the new binary; $CEEMS_UNIT is stopped." >&2
    rollback_help
    exit 1
fi
# Rename over the script so the running shell keeps its old inode.
if [ -f "$tmp/deploy/update.sh" ]; then
    install -m 0755 "$tmp/deploy/update.sh" "$bindir/ceems-update.new" &&
        mv -f "$bindir/ceems-update.new" "$bindir/ceems-update" ||
        echo "ceems-update: could not update ceems-update itself (continuing)" >&2
fi
systemctl start "$CEEMS_UNIT" || true

up=0
i=0
while [ "$i" -lt 10 ]; do
    if systemctl is-active --quiet "$CEEMS_UNIT"; then up=1; break; fi
    sleep 1
    i=$((i + 1))
done

if [ "$up" -eq 1 ]; then
    echo "$tag" > "$versionfile"
    echo "Updated ceems $old -> $tag; the service is running."
    echo "Logs: journalctl -u $CEEMS_UNIT -n 20 --no-pager"
    exit 0
fi


echo "ceems $tag did not come up. Last log lines:" >&2
journalctl -u "$CEEMS_UNIT" -n 20 --no-pager >&2 || true
rollback_help
exit 1
