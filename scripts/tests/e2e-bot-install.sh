#!/usr/bin/env bash
# Gamaj Bot — end-to-end install test.
#
# Builds the bot binary from this checkout, runs the real installer, and checks
# that everything a running host needs was actually created: the systemd unit,
# the installed binary and the hardened configuration file.
#
# A fresh install has no credentials yet, so the service is not expected to stay
# healthy — the installer says so itself. What this test asserts is the install
# path: unit written and loadable, binary in place, config written with mode
# 600. A broken installer fails here instead of on a customer's server.
#
# Requirements: Linux, systemd, root.
# On any other platform the test reports SKIP and succeeds.

set -euo pipefail

SERVICE_NAME="gamaj-bot"
INSTALL_DIR="/opt/gamaj-bot"
CONFIG_FILE="$INSTALL_DIR/.gamaj-bot.json"
BINARY_PATH="$INSTALL_DIR/gamajbot"
UNIT_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"

skip() {
    echo "SKIP: $*"
    exit 0
}

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

step() { printf '\n==> %s\n' "$*"; }
ok()   { printf '  ok  %s\n' "$*"; }

# ------------------------------------------------------------- requirements
[ "$(uname -s)" = "Linux" ] || skip "the binary install path is Linux-only"
command -v systemctl >/dev/null 2>&1 || skip "systemctl is not available"
[ "$(id -u)" -eq 0 ] || skip "the installer must run as root"

cleanup() {
    printf '\n==> cleaning up\n'
    systemctl disable --now "${SERVICE_NAME}.service" >/dev/null 2>&1 || true
    rm -f "$UNIT_FILE"
    systemctl daemon-reload >/dev/null 2>&1 || true
    rm -rf "$INSTALL_DIR"
    rm -f "$repo_root/gamajbot"
}
trap cleanup EXIT

# ------------------------------------------------------------------- build
step "building the bot binary from this checkout"
cd "$repo_root"
go build -trimpath -o gamajbot ./cmd/gamajbot
[ -x "$repo_root/gamajbot" ] || fail "go build produced no executable at ./gamajbot"
ok "built ./gamajbot"

# ----------------------------------------------------------------- install
step "running the installer"
bash scripts/install.sh
ok "installer exited without error"

# ------------------------------------------------------------------ verify
step "checking the installed artifacts"
[ -x "$BINARY_PATH" ] || fail "installer did not place an executable at $BINARY_PATH"
ok "binary installed at $BINARY_PATH"

[ -f "$CONFIG_FILE" ] || fail "installer did not write $CONFIG_FILE"
config_mode="$(stat -c '%a' "$CONFIG_FILE")"
[ "$config_mode" = "600" ] || fail "config mode is $config_mode, expected 600"
ok "config written with mode 600"

[ -f "$UNIT_FILE" ] || fail "installer did not write the systemd unit at $UNIT_FILE"
ok "systemd unit written"

step "checking the unit is loadable and enabled"
systemctl daemon-reload
# A unit that systemd cannot parse is the failure this test exists to catch.
systemd-analyze verify "$UNIT_FILE" >/dev/null 2>&1 \
    || fail "systemd rejected the generated unit file"
ok "unit passes systemd-analyze verify"

systemctl is-enabled "${SERVICE_NAME}.service" >/dev/null 2>&1 \
    || fail "unit was not enabled at boot"
ok "unit enabled at boot"

grep -q "^ExecStart=${BINARY_PATH} -config ${CONFIG_FILE}$" "$UNIT_FILE" \
    || fail "ExecStart does not point at the installed binary and config"
ok "ExecStart points at the installed binary and config"

echo
ok "bot install end-to-end test passed"