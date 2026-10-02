<p align="center">
  <a href="../README.md">فارسی</a> /
  <a href="./README-en.md">English</a> /
  <a href="./README-ru.md">Русский</a> /
  <a href="./README-zh-cn.md">简体中文</a>
</p>

<h1>GAMAJ</h1>

### Bot

**Gamaj Bot** is the official storefront bot of the **Gamaj** ecosystem — a standalone Go binary with zero database dependencies. Everything runs through the **Gamaj API** with the bot's dedicated API key.

- Repository: `GitHub.com/TheGamaj/Bot`
- Telegram channel: [t.me/TheGamaj](https://t.me/TheGamaj) · Support: [t.me/GamajGP](https://t.me/GamajGP)
- Version: `is.0.0.1` — Mini App: `is.0.0.1`

## Features

- **Full in-Telegram store flow**: start → plans → order → payment → verification → automatic service creation → delivery → my services
- **Buyer wallet payments** (transactional, double-refund-safe ledger in Gamaj Panel)
- **Online payments configured in the Gamaj Panel** (payment settings belong to the panel; the bot only uses them via the Gamaj API, and payment is always confirmed through the panel's official inquiry endpoint)
- **Reseller isolation**: each API key belongs to one admin; the bot only sees that admin's plans and buyers
- **Mini App** (`is.0.0.1`) on `/miniapp` with the Gamaj black/white theme
- **No database**: no database files, no MySQL connection — fully stateless
- Secure webhook with `secret_token` and constant-time comparison

## Architecture

```
Gamaj Bot  →  Gamaj API  →  Gamaj Panel Core  →  Database / Nodes
```

The bot never touches the database. Post-purchase user creation passes through the exact same permissions/quota core as the panel — if a reseller has reached their traffic or user cap, the order is rejected by the panel itself.

## Install

### 1) Build

```bash
go build -o gamajbot ./cmd/gamajbot
```

### 2) Server install

```bash
sudo ./scripts/install.sh
```

Then fill `/opt/gamaj-bot/.gamaj-bot.json`:

| Key | Description |
|---|---|
| `panel_url` | Gamaj Panel address, e.g. `http://127.0.0.1:616` |
| `api_key` | Gamaj API key (`gm_...`) from the panel's My Account or Applications install |
| `bot_token` | Bot token from [@BotFather](https://t.me/BotFather) |
| `admin_id` | Numeric Telegram ID of the selling admin |
| `webhook_secret` | Random secret (auto-filled by the panel installer) |
| `webhook_base_url` | Public HTTPS address for the webhook, e.g. `https://bot.example.com` |
| `listen_host` / `listen_port` | Webhook service address (default `127.0.0.1:8080`) |

### 3) Register the webhook

```bash
sudo /opt/gamaj-bot/gamajbot -config /opt/gamaj-bot/.gamaj-bot.json -setup-webhook
sudo systemctl restart gamaj-bot
```

### Service management

```bash
sudo ./scripts/manage.sh {start|stop|restart|status|logs|update|uninstall}
```

## Install from Gamaj Panel (recommended)

Open **Applications** in the panel and install **Gamaj Bot**. The panel automatically downloads the binary, creates a dedicated `gm_...` API key, writes the bot's secret config, and registers the webhook.

## Direct access (no webhook)

Without public HTTPS, run the bot in long-polling mode (leave `webhook_base_url` empty) — `/healthz` and the Mini App keep serving.

## API the bot consumes

| Method | Path | Description |
|---|---|---|
| GET | `/api/bot/plans` | Sellable plans of the key's admin |
| POST | `/api/bot/plans/{id}/orders` | Place an order for a `telegram_id` |
| GET | `/api/bot/orders/{id}` | Order status |
| GET | `/api/bot/wallet?telegram_id=` | Buyer wallet balance |
| POST | `/api/bot/orders/{id}/pay-wallet` | Pay from wallet |
| POST | `/api/bot/orders/{id}/payment-link` | Create an online payment link |
| POST | `/api/bot/orders/{id}/verify` | Verify payment with `pay_id` (mandatory inquiry) |
| GET | `/api/bot/service?telegram_id=` | Buyer's service and delivery info |
| GET | `/api/bot/payment/callback` | Gateway callback (records only — settled by inquiry) |

## Permissions and security

- The API key is Bearer-only and bound to one admin; that admin's traffic/user caps apply to sales.
- Gateway responses are validated before trust; the paid amount must match the order exactly.
- Gateway callbacks are replay-safe and never settle without an inquiry.
- Secrets are never logged.

## License

© Gamaj — all rights reserved. Coded by AsliCode.
