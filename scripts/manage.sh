#!/usr/bin/env bash
# Gamaj Bot service manager.
#
# Works from any directory, self-heals a missing systemd unit, and offers a
# simple numeric menu (a bare number typed at the shell also works).
#
# Usage: sudo gamaj-bot-manage [status|logs|start|stop|restart|config|webhook|update|uninstall]
set -euo pipefail

SERVICE_NAME="gamaj-bot"
INSTALL_DIR="/opt/gamaj-bot"
CONFIG_FILE="$INSTALL_DIR/.gamaj-bot.json"
BINARY_PATH="$INSTALL_DIR/gamajbot"

say()  { printf '\033[36m==>\033[0m %s\n' "$*"; }
ok()   { printf '\033[32m ✓\033[0m %s\n' "$*"; }
die()  { printf '\033[31m ✗ %s\033[0m\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root: sudo $0 $*"

# ------------------------------------------------------------- self healing
ensure_unit() {
    if [ -f "/etc/systemd/system/${SERVICE_NAME}.service" ]; then
        return
    fi
    [ -x "$BINARY_PATH" ] || die "the bot is not installed yet; run the installer first"
    say "service unit missing; recreating it"
    cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=Gamaj Bot
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=${INSTALL_DIR}
ExecStart=${BINARY_PATH} -config ${CONFIG_FILE}
Restart=on-failure
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable "${SERVICE_NAME}.service" >/dev/null 2>&1 || true
    ok "service unit restored"
}

service_status() {
    if systemctl is-active --quiet "${SERVICE_NAME}.service"; then
        ok "service is running"
    else
        printf '\033[31m ✗\033[0m service is stopped\n'
    fi
    systemctl status "${SERVICE_NAME}.service" --no-pager || true
}

# ---------------------------------------------------------------- commands
do_config() {
    [ -f "$CONFIG_FILE" ] || die "config not found at $CONFIG_FILE — run the installer first"
    say "editing $CONFIG_FILE"
    "${EDITOR:-nano}" "$CONFIG_FILE"
    chmod 600 "$CONFIG_FILE"
    systemctl restart "${SERVICE_NAME}.service" 2>/dev/null || true
    ok "config saved; service restarted"
}

do_webhook() {
    [ -f "$CONFIG_FILE" ] || die "config not found — run the installer and fill it first"
    say "registering the Telegram webhook"
    "$BINARY_PATH" -config "$CONFIG_FILE" -setup-webhook
    systemctl restart "${SERVICE_NAME}.service"
    ok "webhook registered"
}

do_update() {
    say "updating the bot binary"
    tmp="$(mktemp)"
    arch="$(uname -m)"
    case "$arch" in x86_64) a=amd64 ;; aarch64|arm64) a=arm64 ;; *) a="" ;; esac
    [ -n "$a" ] || die "unsupported architecture: $arch"
    url="$(curl -fsSL "https://api.github.com/repos/TheGamaj/Bot/releases/latest" 2>/dev/null \
        | sed -nE 's/.*"browser_download_url":[[:space:]]*"([^"]*gamaj-bot-linux-'"$a"'[^"]*)".*/\1/p' | head -n 1)"
    [ -n "${url:-}" ] || die "could not resolve the release asset; update from a git clone with go build"
    curl -fL "$url" -o "$tmp"
    install -m 0755 "$tmp" "$BINARY_PATH"
    rm -f "$tmp"
    systemctl restart "${SERVICE_NAME}.service"
    ok "bot updated and restarted"
}

do_uninstall() {
    say "stopping and removing the bot"
    systemctl stop "${SERVICE_NAME}.service" 2>/dev/null || true
    systemctl disable "${SERVICE_NAME}.service" 2>/dev/null || true
    rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
    systemctl daemon-reload
    rm -rf "$INSTALL_DIR"
    ok "Gamaj Bot removed"
}

menu() {
    echo
    echo "  1) status    2) logs    3) start    4) stop"
    echo "  5) restart   6) config  7) webhook  8) update"
    echo "  9) uninstall"
    echo
    read -r -p "Select option [1]: " choice
    case "${choice:-1}" in
        1) service_status ;;
        2) journalctl -u "${SERVICE_NAME}" -f --no-pager ;;
        3) systemctl start "${SERVICE_NAME}.service"; ok "started" ;;
        4) systemctl stop "${SERVICE_NAME}.service"; ok "stopped" ;;
        5) systemctl restart "${SERVICE_NAME}.service"; ok "restarted" ;;
        6) do_config ;;
        7) do_webhook ;;
        8) do_update ;;
        9) do_uninstall ;;
        *) echo "unknown option: $choice" ;;
    esac
}

ensure_unit

cmd="${1:-}"
if [ $# -eq 0 ]; then
    menu
    exit 0
fi

case "$cmd" in
    status)  service_status ;;
    logs)    journalctl -u "${SERVICE_NAME}" -f --no-pager ;;
    start)   systemctl start "${SERVICE_NAME}.service"; ok "started" ;;
    stop)    systemctl stop "${SERVICE_NAME}.service"; ok "stopped" ;;
    restart) systemctl restart "${SERVICE_NAME}.service"; ok "restarted" ;;
    config)  do_config ;;
    webhook) do_webhook ;;
    update)  do_update ;;
    uninstall) do_uninstall ;;
    # Numeric input (example: 21) selects the menu entry with that number.
    ''|*[!0-9]*) menu ;;
    *) menu ;;
esac
