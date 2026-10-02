#!/usr/bin/env bash
# Gamaj Bot installer — professional, automatic installation.
#
# What it does:
#   1. Finds the project root no matter where it is invoked from.
#   2. Reuses an existing gamajbot binary when present; otherwise installs Go,
#      builds the binary from source, or downloads the official release binary.
#   3. Creates /opt/gamaj-bot with a hardened configuration file.
#   4. Installs and starts the systemd service and verifies it is healthy.
#
# Usage (as root):  sudo ./scripts/install.sh
set -euo pipefail

SERVICE_NAME="gamaj-bot"
INSTALL_DIR="/opt/gamaj-bot"
CONFIG_FILE="$INSTALL_DIR/.gamaj-bot.json"
BINARY_PATH="$INSTALL_DIR/gamajbot"

say()  { printf '\033[36m==>\033[0m %s\n' "$*"; }
ok()   { printf '\033[32m ✓\033[0m %s\n' "$*"; }
warn() { printf '\033[33m !\033[0m %s\n' "$*"; }
die()  { printf '\033[31m ✗ %s\033[0m\n' "$*" >&2; exit 1; }

# ---------------------------------------------------------------- root check
[ "$(id -u)" -eq 0 ] || die "run this installer as root: sudo $0"

# ------------------------------------------------------------- project root
# Resolve the directory that contains this script, then step up to the repo
# root — so `bash scripts/install.sh`, `./install.sh` or a piped stdin run all
# behave the same and "command not found" can never happen again.
SCRIPT_SOURCE="${BASH_SOURCE[0]:-$0}"
if [ -n "${BASH_SOURCE[0]:-}" ] && [ -f "${BASH_SOURCE[0]}" ]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
else
    # Piped through curl | bash: fall back to the current directory.
    SCRIPT_DIR="$(pwd)"
    PROJECT_ROOT="$(pwd)"
fi
cd "$PROJECT_ROOT"

say "Gamaj Bot installer"
ok "project root: $PROJECT_ROOT"

# ------------------------------------------------------------------ helpers
apt_install() {
    if command -v apt-get >/dev/null 2>&1; then
        DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a apt-get install -y -qq "$@" >/dev/null 2>&1 || return 1
    elif command -v dnf >/dev/null 2>&1; then
        dnf install -y -q "$@" >/dev/null 2>&1 || return 1
    elif command -v yum >/dev/null 2>&1; then
        yum install -y -q "$@" >/dev/null 2>&1 || return 1
    else
        return 1
    fi
}

go_major() {
    command -v go >/dev/null 2>&1 || { echo 0; return; }
    go version 2>/dev/null | sed -nE 's/.*go([0-9]+)\..*/\1/p'
}

# ------------------------------------------------------------ binary source
mkdir -p "$INSTALL_DIR"

if [ -x "$BINARY_PATH" ]; then
    ok "existing binary found at $BINARY_PATH (kept)"
elif [ -f "$PROJECT_ROOT/gamajbot" ]; then
    install -m 0755 "$PROJECT_ROOT/gamajbot" "$BINARY_PATH"
    ok "built binary found in repository; installed to $BINARY_PATH"
else
    # Try the official release binary first — no Go toolchain needed.
    say "no binary present; trying the official release download"
    arch="$(uname -m)"
    case "$arch" in
        x86_64)  release_arch="amd64" ;;
        aarch64|arm64) release_arch="arm64" ;;
        *) release_arch="" ;;
    esac
    downloaded=0
    if [ -n "$release_arch" ]; then
        asset_url="$(curl -fsSL "https://api.github.com/repos/TheGamaj/Bot/releases/latest" 2>/dev/null \
            | sed -nE 's/.*"browser_download_url":[[:space:]]*"([^"]*gamaj-bot-linux-'"$release_arch"'[^"]*)".*/\1/p' \
            | head -n 1 || true)"
        if [ -n "${asset_url:-}" ]; then
            tmp_bin="$(mktemp)"
            if curl -fL "$asset_url" -o "$tmp_bin" 2>/dev/null && [ -s "$tmp_bin" ]; then
                install -m 0755 "$tmp_bin" "$BINARY_PATH"
                rm -f "$tmp_bin"
                ok "release binary downloaded from GitHub"
                downloaded=1
            fi
            rm -f "$tmp_bin" 2>/dev/null || true
        fi
    fi

    # Fallback: install Go and build from source (fully automatic).
    if [ "$downloaded" -ne 1 ]; then
        say "release download unavailable; building from source"
        if [ "$(go_major)" -lt 1 ]; then
            say "installing the Go toolchain"
            apt_install golang-go || die "could not install Go automatically; install Go 1.21+ and re-run"
            if [ "$(go_major)" -lt 1 ]; then
                # Distro Go is too old or missing: use the official tarball.
                say "distro Go unavailable; installing Go from go.dev"
                curl -fsSL "https://go.dev/dl/go1.24.5.linux-${release_arch:-amd64}.tar.gz" -o /tmp/gamaj-go.tgz \
                    || die "could not download the Go toolchain"
                rm -rf /usr/local/go
                tar -C /usr/local -xzf /tmp/gamaj-go.tgz
                ln -sf /usr/local/go/bin/go /usr/local/bin/go
                rm -f /tmp/gamaj-go.tgz
            fi
        fi
        ok "Go $(go version 2>/dev/null | sed -nE 's/.*go([0-9.]+).*/\1/p') ready"
        (cd "$PROJECT_ROOT" && go build -o gamajbot ./cmd/gamajbot) || die "build failed"
        install -m 0755 "$PROJECT_ROOT/gamajbot" "$BINARY_PATH"
        ok "binary built and installed to $BINARY_PATH"
    fi
fi

# ------------------------------------------------------------- configuration
if [ ! -f "$CONFIG_FILE" ]; then
    say "creating the configuration file"
    webhook_secret="$(head -c 24 /dev/urandom | base64 | tr -d '/+=' 2>/dev/null || date +%s%N)"
    cat > "$CONFIG_FILE" <<JSON
{
  "panel_url": "http://127.0.0.1:616",
  "api_key": "",
  "bot_token": "",
  "admin_id": "",
  "webhook_secret": "${webhook_secret}",
  "listen_host": "127.0.0.1",
  "listen_port": 8080,
  "webhook_base_url": ""
}
JSON
    ok "configuration written: $CONFIG_FILE"
fi
chmod 600 "$CONFIG_FILE"

# ------------------------------------------------------------- systemd unit
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

# --------------------------------------------------- start + health verification
say "starting the service"
systemctl restart "${SERVICE_NAME}.service"

healthy=0
for _ in 1 2 3 4 5 6; do
    if systemctl is-active --quiet "${SERVICE_NAME}.service"; then
        healthy=1
        break
    fi
    sleep 2
done

echo
if [ "$healthy" -eq 1 ]; then
    ok "Gamaj Bot service is running"
    cat <<EOF

  Directory : $INSTALL_DIR
  Config    : $CONFIG_FILE
  Service   : systemctl {status|restart|stop} $SERVICE_NAME
  Logs      : journalctl -u $SERVICE_NAME -f

Next steps:
  1. Edit $CONFIG_FILE and fill in:
       panel_url   — the Gamaj Panel address (example: http://127.0.0.1:616)
       api_key     — a Gamaj API key (gm_...) from the panel
       bot_token   — the bot token from @BotFather
       admin_id    — the numeric Telegram id of the sales admin
  2. Register the webhook:
       sudo $BINARY_PATH -config $CONFIG_FILE -setup-webhook
     sudo systemctl restart $SERVICE_NAME
EOF
else
    warn "the service did not become healthy — the configuration is not filled in yet (this is expected on a fresh install)"
    cat <<EOF

  Directory : $INSTALL_DIR
  Config    : $CONFIG_FILE
  Logs      : journalctl -u $SERVICE_NAME -e

Next steps:
  1. Edit $CONFIG_FILE and fill in panel_url, api_key, bot_token and admin_id.
  2. Run: sudo systemctl restart $SERVICE_NAME
  3. Register the webhook:
       sudo $BINARY_PATH -config $CONFIG_FILE -setup-webhook
     sudo systemctl restart $SERVICE_NAME
EOF
fi
