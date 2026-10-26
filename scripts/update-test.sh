#!/bin/bash
# Tests deploy/update.sh against stub curl/systemctl/etc. No network, no root.
set -u
here=$(cd "$(dirname "$0")/.." && pwd)
w=$(mktemp -d)
trap 'rm -rf "$w"' EXIT
mkdir "$w/stubs"
export PATH="$w/stubs:$PATH"
export CEEMS_BIN="$w/bin/ceems" CEEMS_DATA="$w/data" CEEMS_UNIT=ceems-test CEEMS_REPO=https://example.invalid/r
export STUB_LOG="$w/log" STUB_ARCHIVE="$w/good.tar.gz"

cat > "$w/stubs/curl" <<'S'
#!/bin/sh
echo "curl $*" >> "$STUB_LOG"
case " $* " in *" -w "*) echo "https://example.invalid/r/releases/tag/${STUB_LATEST:-v1.2.3}"; exit 0 ;; esac
while [ $# -gt 0 ]; do [ "$1" = "-o" ] && out=$2; shift; done
cp "$STUB_ARCHIVE" "$out"
S
cat > "$w/stubs/systemctl" <<'S'
#!/bin/sh
echo "systemctl $*" >> "$STUB_LOG"
[ "$1" = "is-active" ] && [ "${STUB_ACTIVE:-1}" = "0" ] && exit 3
exit 0
S
printf '#!/bin/sh\necho "journal line"\n' > "$w/stubs/journalctl"
printf '#!/bin/sh\necho 0\n' > "$w/stubs/id"
printf '#!/bin/sh\necho "${STUB_ARCH:-aarch64}"\n' > "$w/stubs/uname"
printf '#!/bin/sh\nexit 0\n' > "$w/stubs/sleep"
chmod +x "$w/stubs"/*

mkdir -p "$w/src/deploy"
echo "new-binary" > "$w/src/ceems"
cp "$here/deploy/update.sh" "$w/src/deploy/update.sh"
tar -czf "$w/good.tar.gz" -C "$w/src" ceems deploy
tar -czf "$w/bad.tar.gz" -C "$w/src" deploy

fails=0
check() { if [ "$2" = ok ]; then echo "ok   $1"; else echo "FAIL $1"; fails=$((fails + 1)); fi; }
reset() {
    rm -rf "$w/bin" "$w/data"; mkdir -p "$w/bin" "$w/data"; : > "$STUB_LOG"
    echo old-binary > "$CEEMS_BIN"
    export STUB_ARCHIVE="$w/good.tar.gz" STUB_ACTIVE=1 STUB_ARCH=aarch64 STUB_LATEST=v1.2.3
}
run() { sh "$here/deploy/update.sh" "$@" > "$w/out" 2>&1; rc=$?; }
has() { grep -q -- "$1" "$2"; }
lacks() { ! grep -q -- "$1" "$2"; }
yes() { if "$@"; then echo ok; else echo no; fi; }

reset; run
check "update exits 0" "$([ $rc = 0 ] && echo ok)"
check "binary replaced" "$(yes has new-binary "$CEEMS_BIN")"
check "previous binary kept" "$(yes has old-binary "$CEEMS_BIN.prev")"
check "version written" "$([ "$(cat "$CEEMS_DATA/installed-version")" = v1.2.3 ] && echo ok)"
check "self-update installed" "$(yes test -x "$w/bin/ceems-update")"
check "self-update is the archive copy" "$(yes cmp -s "$w/src/deploy/update.sh" "$w/bin/ceems-update")"
check "no .new file left" "$(yes test ! -e "$w/bin/ceems-update.new")"
check "arm64 archive requested" "$(yes has 'ceems-v1.2.3-linux-arm64.tar.gz' "$STUB_LOG")"
check "service stopped then started" "$([ "$(grep -c '^systemctl \(stop\|start\) ' "$STUB_LOG")" = 2 ] && echo ok)"

run
check "same version exits 0" "$([ $rc = 0 ] && echo ok)"
check "same version says already installed" "$(yes has 'already installed' "$w/out")"
reset; echo v1.2.3 > "$CEEMS_DATA/installed-version"; run
check "same version does not stop service" "$(yes lacks 'systemctl stop' "$STUB_LOG")"
run -f
check "-f forces update" "$(yes has new-binary "$CEEMS_BIN")"

reset; STUB_ARCHIVE="$w/bad.tar.gz" run v1.2.3
check "archive without ceems fails" "$([ $rc != 0 ] && echo ok)"
check "bad archive: service untouched" "$(yes lacks 'systemctl stop' "$STUB_LOG")"
check "bad archive: binary unchanged" "$(yes has old-binary "$CEEMS_BIN")"

reset; export STUB_ACTIVE=0
run v1.2.3
check "dead service exits 1" "$([ $rc = 1 ] && echo ok)"
check "dead service prints journal" "$(yes has 'journal line' "$w/out")"
check "dead service prints rollback" "$(yes has 'roll back' "$w/out")"
check "dead service leaves version unwritten" "$(yes test ! -e "$CEEMS_DATA/installed-version")"

reset; printf "#!/bin/sh
exit 1
" > "$w/stubs/install"; chmod +x "$w/stubs/install"; run v1.2.3; rm "$w/stubs/install"
check "failed install exits 1" "$([ $rc = 1 ] && echo ok)"
check "failed install says stopped and prints rollback" "$(grep -q stopped "$w/out" && grep -q 'roll back' "$w/out" && echo ok)"
check "failed install does not start service" "$(yes lacks 'systemctl start' "$STUB_LOG")"

reset; export STUB_ARCH=riscv64; run
check "unknown arch fails naming it" "$([ $rc != 0 ] && grep -q riscv64 "$w/out" && echo ok)"
reset; run 1.2
check "bad tag fails" "$([ $rc != 0 ] && echo ok)"
run v1.2.x
check "non-numeric tag fails" "$([ $rc != 0 ] && echo ok)"
run v1.2.3-rc1
check "prerelease tag accepted" "$([ $rc = 0 ] && echo ok)"

[ $fails = 0 ] && echo "all update tests passed" || { echo "$fails failed"; exit 1; }
