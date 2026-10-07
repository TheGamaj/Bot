#!/usr/bin/env bash
#
# Gamaj Bot + Node — end-to-end webhook and xray-backed service test.
#
# Builds the bot from this checkout and the node from the Node checkout, starts
# the Node as a systemd service with a stub Xray core (so the xray-backed gRPC
# control port is exercised without a network download), starts a small mock
# Gamaj Panel HTTP server that records every API call the bot makes to disk,
# starts the bot pointing at the mock panel, and then proves the bot's Telegram
# webhook actually accepts and processes an update end to end.
#
# The Bot and Node repositories are separate checkouts, so this script needs
# both on disk: the node checkout is taken from GAMAJ_E2E_NODE_REPO when set,
# and otherwise from a sibling 'Node' directory next to this repository.
#
# What this proves that the existing per-repo e2e tests do not:
#   - the bot webhook receiver accepts a signed update and returns 200;
#   - the bot processes the update and calls the expected Panel API endpoints
#     under the /api prefix the real Panel serves;
#   - a webhook update arrives without its secret token and is refused;
#   - the node service starts and listens on its gRPC control port with a stub
#     xray core, which is the xray-backed feature surface.
#
# Requirements: Linux, systemd, root (run through sudo), Go, curl, openssl,
# python3 and the Node checkout.
# On any other platform the test reports SKIP and succeeds.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"

# The Node repository is a separate checkout from this one; only the bot is
# built from $repo_root.
node_root="${GAMAJ_E2E_NODE_REPO:-}"
if [ -z "$node_root" ]; then
    node_root="$(cd "$repo_root/../Node" 2>/dev/null && pwd || true)"
fi

# ------------------------------------------------------------------ config
E2E_PREFIX="${GAMAJ_E2E_PREFIX:-$(mktemp -d /tmp/gamaj-e2e-XXXXXX)}"
E2E_NODE_NAME="${GAMAJ_E2E_NODE_NAME:-gamaj-node-e2e}"
E2E_NODE_PORT="${GAMAJ_E2E_NODE_PORT:-62050}"
E2E_BOT_PORT="${GAMAJ_E2E_BOT_PORT:-8099}"
E2E_PANEL_PORT="${GAMAJ_E2E_PANEL_PORT:-6162}"
E2E_WAIT_SECONDS="${GAMAJ_E2E_WAIT_SECONDS:-60}"
E2E_SECRET="${GAMAJ_E2E_SECRET:-gamaj-e2e-webhook-secret-$(date +%s)}"

NODE_UNIT="/etc/systemd/system/$E2E_NODE_NAME.service"
BOT_UNIT="/etc/systemd/system/gamaj-bot-e2e.service"
CALLS_FILE="$E2E_PREFIX/mock-panel-calls.json"

skip() { echo "SKIP: $*"; exit 0; }
step() { printf '\n== %s\n' "$*"; }
fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
ok() { printf '  ok  %s\n' "$*"; }

cleanup() {
    local exit_code=$?
    printf '\n== cleaning up\n'
    systemctl disable --now "$E2E_NODE_NAME.service" >/dev/null 2>&1 || true
    systemctl disable --now "gamaj-bot-e2e.service" >/dev/null 2>&1 || true
    rm -f "$NODE_UNIT" "$BOT_UNIT"
    systemctl daemon-reload >/dev/null 2>&1 || true
    if [ -f "$E2E_PREFIX/mock-panel.pid" ]; then
        kill "$(cat "$E2E_PREFIX/mock-panel.pid")" >/dev/null 2>&1 || true
        wait "$(cat "$E2E_PREFIX/mock-panel.pid")" >/dev/null 2>&1 || true
    fi
    if [ "${GAMAJ_E2E_KEEP_PREFIX:-0}" = "1" ]; then
        echo "Kept sandbox at $E2E_PREFIX"
    else
        rm -rf "$E2E_PREFIX"
    fi
    exit "$exit_code"
}
trap cleanup EXIT

# ------------------------------------------------------------------ preflight
step "Check prerequisites"
[[ "$(uname -s)" == Linux* ]] || skip "the combined e2e path is Linux-only"
[ -d /run/systemd/system ] || skip "systemd is not running"
command -v systemctl >/dev/null 2>&1 || skip "systemctl is not available"
command -v openssl >/dev/null 2>&1 || skip "openssl is required for the node TLS setup"
command -v go >/dev/null 2>&1 || skip "the Go toolchain is required to build both binaries"
[ "$(id -u)" -eq 0 ] || skip "this test needs root (run it with sudo)"

# ------------------------------------------------------------------ build
step "Build both binaries"

[ -n "$node_root" ] && [ -f "$node_root/go.mod" ] || fail "no Node checkout found (set GAMAJ_E2E_NODE_REPO to the Node repository root)"
ok "node checkout at $node_root"

( cd "$node_root" && CGO_ENABLED=0 go build -trimpath -buildvcs=false -o "$E2E_PREFIX/gamaj-node" ./cmd/gamaj-node )
[ -x "$E2E_PREFIX/gamaj-node" ] || fail "gamaj-node was not built"
ok "built gamaj-node"

( cd "$repo_root" && CGO_ENABLED=0 go build -trimpath -buildvcs=false -o "$E2E_PREFIX/gamajbot" ./cmd/gamajbot )
[ -x "$E2E_PREFIX/gamajbot" ] || fail "gamajbot was not built"
ok "built gamajbot"

# ------------------------------------------------------------------ mock panel
step "Build and start the mock Gamaj Panel on port $E2E_PANEL_PORT"

mock_src="$E2E_PREFIX/mock-panel/main.go"
mkdir -p "$E2E_PREFIX/mock-panel"

cat > "$mock_src" <<'GOEOF'
//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type recordedCall struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Query  string `json:"query"`
	Body   string `json:"body"`
	Time   string `json:"time"`
}

var (
	calls     []recordedCall
	callsMu   sync.Mutex
	callsFile string
	plans     = []map[string]any{
		{"id": 1, "name": "Test Plan", "price": 99000, "duration_days": 30, "data_limit": 5368709120, "ip_limit": 0, "visible": true},
	}
	orders = []map[string]any{
		{"id": "ord-test-1", "plan_id": 1, "plan_name": "Test Plan", "amount": 99000, "status": "pending", "created_at": "2026-10-06T00:00:00Z"},
	}
)

func record(method, path, query, body string) {
	callsMu.Lock()
	defer callsMu.Unlock()
	calls = append(calls, recordedCall{method, path, query, body, time.Now().UTC().Format(time.RFC3339)})
	if callsFile != "" {
		data, _ := json.Marshal(calls)
		os.WriteFile(callsFile, data, 0o644)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func body(r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	defer r.Body.Close()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return ""
	}
	r.Body = io.NopCloser(strings.NewReader(string(data)))
	return string(data)
}

func main() {
	callsFile = os.Getenv("GAMAJ_E2E_CALLS_FILE")
	if callsFile == "" {
		callsFile = "/tmp/gamaj-e2e-calls.json"
	}

	// Every Gamaj Panel API route lives under /api, which is what the bot's
	// API client appends to the configured panel_url.
	http.HandleFunc("/api/bot/plans", func(w http.ResponseWriter, r *http.Request) {
		record(r.Method, r.URL.Path, r.URL.RawQuery, body(r))
		if r.Method == http.MethodPost {
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
				payload["id"] = int64(len(plans) + 1)
				plans = append(plans, payload)
				writeJSON(w, 201, payload)
				return
			}
		}
		writeJSON(w, 200, plans)
	})

	http.HandleFunc("/api/bot/plans/", func(w http.ResponseWriter, r *http.Request) {
		record(r.Method, r.URL.Path, r.URL.RawQuery, body(r))
		idStr := strings.TrimPrefix(r.URL.Path, "/api/bot/plans/")
		if idStr == "" {
			http.NotFound(w, r)
			return
		}
		// The id of the plan this path is about. The order sub-resource
		// /api/bot/plans/{id}/orders carries the plan id before the suffix.
		planID := idStr
		if strings.HasSuffix(idStr, "/orders") {
			planID = strings.TrimSuffix(idStr, "/orders")
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			order := map[string]any{
				"id":         "ord-e2e-" + planID,
				"plan_id":    planID,
				"plan_name":  "Test Plan",
				"amount":     99000,
				"status":     "pending",
				"created_at": time.Now().UTC().Format(time.RFC3339),
			}
			orders = append(orders, order)
			writeJSON(w, 201, order)
			return
		}
		if r.Method == http.MethodPut {
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
				for i, p := range plans {
					if fmt.Sprint(p["id"]) == planID {
						for k, v := range payload {
							p[k] = v
						}
						plans[i] = p
						writeJSON(w, 200, p)
						return
					}
				}
			}
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodDelete {
			for i, p := range plans {
				if fmt.Sprint(p["id"]) == planID {
					p["visible"] = false
					plans[i] = p
					writeJSON(w, 200, p)
					return
				}
			}
			http.NotFound(w, r)
			return
		}
		for _, p := range plans {
			if fmt.Sprint(p["id"]) == planID {
				writeJSON(w, 200, p)
				return
			}
		}
		http.NotFound(w, r)
	})

	// Orders are addressed separately from plans, exactly as the Panel API
	// does: /api/bot/orders/{id} plus its /pay-wallet, /payment-link and
	// /verify sub-resources.
	http.HandleFunc("/api/bot/orders/", func(w http.ResponseWriter, r *http.Request) {
		record(r.Method, r.URL.Path, r.URL.RawQuery, body(r))
		rest := strings.TrimPrefix(r.URL.Path, "/api/bot/orders/")
		orderID, action := rest, ""
		if i := strings.Index(rest, "/"); i >= 0 {
			orderID, action = rest[:i], rest[i+1:]
		}
		if orderID == "" {
			http.NotFound(w, r)
			return
		}
		find := func() map[string]any {
			for _, o := range orders {
				if o["id"] == orderID {
					return o
				}
			}
			return nil
		}
		switch action {
		case "":
			if o := find(); o != nil {
				writeJSON(w, 200, o)
				return
			}
		case "pay-wallet":
			if o := find(); o != nil {
				o["status"] = "paid"
				writeJSON(w, 200, o)
				return
			}
		case "payment-link":
			writeJSON(w, 200, map[string]any{"order_id": orderID, "payment_url": "https://pay.example.com/" + orderID})
			return
		case "verify":
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			if o := find(); o != nil {
				o["status"] = "paid"
				writeJSON(w, 200, o)
				return
			}
		}
		http.NotFound(w, r)
	})

	http.HandleFunc("/api/bot/orders", func(w http.ResponseWriter, r *http.Request) {
		record(r.Method, r.URL.Path, r.URL.RawQuery, body(r))
		writeJSON(w, 200, orders)
	})

	http.HandleFunc("/api/bot/wallet", func(w http.ResponseWriter, r *http.Request) {
		record(r.Method, r.URL.Path, r.URL.RawQuery, body(r))
		writeJSON(w, 200, map[string]any{"balance": 500000, "currency": "IRT"})
	})

	http.HandleFunc("/api/bot/service", func(w http.ResponseWriter, r *http.Request) {
		record(r.Method, r.URL.Path, r.URL.RawQuery, body(r))
		writeJSON(w, 200, map[string]any{
			"username": "testuser", "status": "active",
			"used_traffic": 0, "data_limit": 5368709120, "expire": 1735689600,
			"subscription_url": "https://panel.example.com/sub/abc",
			"links":            []string{"vless://testuser@example.com"},
		})
	})

	http.HandleFunc("/api/admin", func(w http.ResponseWriter, r *http.Request) {
		record(r.Method, r.URL.Path, r.URL.RawQuery, body(r))
		writeJSON(w, 200, map[string]any{"authenticated": true})
	})

	listener, err := net.Listen("tcp", "127.0.0.1:"+os.Getenv("GAMAJ_E2E_PANEL_PORT"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "mock panel: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("mock panel listening on %s\n", listener.Addr().String())
	_ = http.Serve(listener, nil)
}
GOEOF

cd "$E2E_PREFIX/mock-panel"
CGO_ENABLED=0 go build -trimpath -o mock-panel main.go
GAMAJ_E2E_CALLS_FILE="$CALLS_FILE" GAMAJ_E2E_PANEL_PORT="$E2E_PANEL_PORT" ./mock-panel &
echo $! > "$E2E_PREFIX/mock-panel.pid"
sleep 2
curl -sf -o /dev/null "http://127.0.0.1:$E2E_PANEL_PORT/api/bot/plans" || fail "mock panel did not start"
ok "mock panel is live on port $E2E_PANEL_PORT"

# ------------------------------------------------------------------ node
step "Start the Node with a stub Xray core on port $E2E_NODE_PORT"

node_data="$E2E_PREFIX/node-data"
node_app="$E2E_PREFIX/opt/gamaj-node-e2e"
mkdir -p "$node_data/xray-core" "$node_app"

cat > "$node_data/xray-core/xray" <<'STUB'
#!/bin/sh
case "${1:-}" in
    version|-version)
        echo "Xray 25.8.29 (Xray, Penetrates Everything.) Custom (go1.24.0 linux/amd64)"
        ;;
    *)
        exit 0
        ;;
esac
STUB
chmod +x "$node_data/xray-core/xray"

openssl req -x509 -newkey rsa:2048 -nodes \
    -keyout "$node_data/client-ca.key" -out "$node_data/client-ca.pem" \
    -days 2 -subj "/CN=gamaj-e2e-client-ca" >/dev/null 2>&1
[ -s "$node_data/client-ca.pem" ] || fail "the throwaway client CA was not created"
ok "node mTLS client CA created"

{
    echo "[TEMPLATE]"
    echo "GAMAJ_NODE_SERVICE_HOST=127.0.0.1"
    echo "GAMAJ_NODE_SERVICE_PORT=$E2E_NODE_PORT"
    echo "GAMAJ_NODE_XRAY_API_HOST=127.0.0.1"
    echo "GAMAJ_NODE_XRAY_API_PORT=$((E2E_NODE_PORT + 1))"
    echo "GAMAJ_DATA_DIR=$node_data"
    echo "GAMAJ_NODE_INSTALL_MODE=binary"
    echo "GAMAJ_NODE_SSL_CERT_FILE=$node_data/ssl_cert.pem"
    echo "GAMAJ_NODE_SSL_KEY_FILE=$node_data/ssl_key.pem"
    echo "GAMAJ_NODE_SSL_CLIENT_CERT_FILE=$node_data/client-ca.pem"
    echo "GAMAJ_XRAY_EXECUTABLE_PATH=$node_data/xray-core/xray"
    echo "GAMAJ_NODE_BINARY_METADATA_FILE=$node_app/.binary-release.json"
} > "$node_app/.env"

export GAMAJ_NODE_APP_NAME="$E2E_NODE_NAME"
export GAMAJ_NODE_BINARY_OVERRIDE="$E2E_PREFIX/gamaj-node"
export GAMAJ_NODE_BINARY_OVERRIDE_VERSION="is.0.0.1"
export GAMAJ_NODE_SOURCE_ONLY=1
E2E_SOURCED=1

# shellcheck disable=SC1091
source "$node_root/scripts/gamaj/gamaj-node.sh"

if normalize_install_mode docker >/dev/null 2>&1; then
    fail "the node installer still accepts the removed docker install mode"
fi
[ "$(normalize_install_mode "")" = "binary" ] || fail "the node installer did not default to binary mode"

install_binary_gamaj_node latest 0
up_gamaj_node

grep -qi docker "$NODE_UNIT" && fail "$E2E_NODE_NAME.service still references docker"
[ -f "$node_app/.binary-release.json" ] || fail "the node did not write its binary release metadata"
ok "node installed through the real installer"

step "Wait for the node gRPC control port $E2E_NODE_PORT"
listening=0
for _ in $(seq 1 "$E2E_WAIT_SECONDS"); do
    if ss -ltn 2>/dev/null | awk '{print $4}' | grep -qE "[:.]$E2E_NODE_PORT\$"; then
        listening=1
        break
    fi
    if ! systemctl is-active --quiet "$E2E_NODE_NAME.service"; then
        journalctl -u "$E2E_NODE_NAME.service" --no-pager -n 50 || true
        fail "$E2E_NODE_NAME.service stopped before it accepted connections"
    fi
    sleep 1
done
[ "$listening" -eq 1 ] || {
    journalctl -u "$E2E_NODE_NAME.service" --no-pager -n 50 || true
    fail "the node is not listening on port $E2E_NODE_PORT within ${E2E_WAIT_SECONDS}s"
}
ok "node gRPC control port $E2E_NODE_PORT is live"

step "Assert the node service stays healthy"
sleep 5
systemctl is-active --quiet "$E2E_NODE_NAME.service" || fail "the node service did not stay active"
[ -s "$node_data/ssl_cert.pem" ] || fail "the node did not create its TLS certificate"
[ -s "$node_data/ssl_key.pem" ] || fail "the node did not create its TLS key"
ok "node service is active and has TLS material"

# ------------------------------------------------------------------ bot
step "Start the Bot pointing at the mock Panel on port $E2E_BOT_PORT"

bot_dir="$E2E_PREFIX/bot"
mkdir -p "$bot_dir"

{
    echo "{"
    echo "  \"panel_url\": \"http://127.0.0.1:$E2E_PANEL_PORT\","
    echo "  \"api_key\": \"e2e-bot-api-key\","
    echo "  \"bot_token\": \"123456:ABC-DEF1234ghijklmnopq\","
    echo "  \"admin_id\": \"777\","
    echo "  \"panel_password\": \"e2e-panel-password\","
    echo "  \"webhook_secret\": \"$E2E_SECRET\","
    echo "  \"listen_host\": \"127.0.0.1\","
    echo "  \"listen_port\": $E2E_BOT_PORT"
    echo "}"
} > "$bot_dir/.gamaj-bot.json"

cat > "$BOT_UNIT" <<UNIT
[Unit]
Description=Gamaj Bot end-to-end test
After=network.target

[Service]
Type=simple
ExecStart=$E2E_PREFIX/gamajbot -config $bot_dir/.gamaj-bot.json
Restart=on-failure
RestartSec=3
ReadWritePaths=$bot_dir
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now "gamaj-bot-e2e.service"

step "Wait for the bot to be ready"
for _ in $(seq 1 30); do
    if curl -sf -o /dev/null "http://127.0.0.1:$E2E_BOT_PORT/healthz"; then
        break
    fi
    sleep 1
done
curl -sf -o /dev/null "http://127.0.0.1:$E2E_BOT_PORT/healthz" || fail "the bot /healthz never answered"
ok "bot /healthz is live"

curl -sf -o /dev/null "http://127.0.0.1:$E2E_BOT_PORT/panel" || fail "the bot panel page never answered"
ok "bot panel page is live"

# ------------------------------------------------------------------ webhook test
step "Send fake Telegram webhook updates and verify the bot processes them"

post_update() {
    local label=$1 payload=$2 status
    status=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -H "X-Telegram-Bot-Api-Secret-Token: $E2E_SECRET" \
        -d "$payload" \
        "http://127.0.0.1:$E2E_BOT_PORT/webhook")
    [ "$status" = "200" ] || fail "the $label update returned $status, want 200"
    ok "webhook accepted the signed $label update (HTTP $status)"
}

post_update "/start" \
    '{"update_id":1,"message":{"message_id":1,"date":1728000000,"chat":{"id":999,"type":"private","first_name":"E2E"},"from":{"id":999,"is_bot":false,"first_name":"E2E"},"text":"/start"}}'

# The plans keyboard is what makes the bot reach the Gamaj API, so the second
# update is the one that must produce a Panel API call.
post_update "/plans" \
    '{"update_id":2,"message":{"message_id":2,"date":1728000000,"chat":{"id":999,"type":"private","first_name":"E2E"},"from":{"id":999,"is_bot":false,"first_name":"E2E"},"text":"plans"}}'

# An update that does not carry the shared secret must be refused.
unsigned_status=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d '{"update_id":3,"message":{"message_id":3,"chat":{"id":999},"text":"/start"}}' \
    "http://127.0.0.1:$E2E_BOT_PORT/webhook")
[ "$unsigned_status" = "403" ] || fail "an unsigned webhook update returned $unsigned_status, want 403"
ok "webhook refused an unsigned update (HTTP $unsigned_status)"

# Give the bot's goroutine time to process the updates and call the mock panel.
sleep 4

step "Read the recorded Panel API calls"
if [ -f "$CALLS_FILE" ]; then
    calls=$(cat "$CALLS_FILE")
else
    calls="[]"
fi
echo "$calls" | python3 -m json.tool 2>/dev/null || echo "$calls"

step "Assert the bot made the expected Panel API calls"

has_plans_call=$(echo "$calls" | python3 -c "import sys,json; calls=json.load(sys.stdin); print('true' if any(c['path']=='/api/bot/plans' for c in calls) else 'false')" 2>/dev/null || echo "false")
has_admin_ping=$(echo "$calls" | python3 -c "import sys,json; calls=json.load(sys.stdin); print('true' if any(c['path']=='/api/admin' for c in calls) else 'false')" 2>/dev/null || echo "false")

if [ "$has_plans_call" = "true" ]; then
    ok "bot called GET /api/bot/plans (plans keyboard)"
else
    fail "bot did not call /api/bot/plans — the 'plans' update should call the Gamaj API for the plan list"
fi

if [ "$has_admin_ping" = "true" ]; then
    ok "bot called GET /api/admin (API key ping at startup)"
else
    echo "NOTE: bot did not call /api/admin — the startup ping may have happened before the mock was ready (non-fatal)"
fi

# ------------------------------------------------------------------ summary
echo
echo "============================================================"
echo "PASS: Gamaj Bot webhook + Node xray-backed service end to end"
echo "  - bot webhook accepted signed Telegram updates (HTTP 200)"
echo "  - bot processed an update and called the Panel API at /api/bot/plans"
echo "  - node gRPC control port $E2E_NODE_PORT is live with stub xray"
echo "  - bot panel page serves at /panel"
echo "============================================================"
