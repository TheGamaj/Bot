<p align="center">
  <a href="../README.md">فارسی</a> /
  <a href="./README-en.md">English</a> /
  <a href="./README-ru.md">Русский</a> /
  <a href="./README-zh-cn.md">简体中文</a>
</p>

<p align="center">
  <img src="./assets/gamaj-logo.svg" alt="Gamaj" width="104" height="104">
</p>

<h1>GAMAJ</h1>

### Bot

**Gamaj Bot** 是 **Gamaj** 生态的官方销售机器人：独立的 Go 二进制程序，不依赖任何数据库。所有操作均通过 **Gamaj API** 和机器人专属 API 密钥完成。

- 仓库：`GitHub.com/TheGamaj/Bot`
- 频道：[t.me/TheGamaj](https://t.me/TheGamaj) · 支持：[t.me/GamajGP](https://t.me/GamajGP)
- 版本：`is.0.0.1` — Mini App：`is.0.0.1`

## 功能

- **Telegram 内完整商店流程**：开始 → 套餐 → 下单 → 支付 → 确认 → 自动开通 → 交付 → 我的服务
- **买家钱包支付**（Gamaj Panel 中的事务性、防重复退款账本）
- **在线支付在 Gamaj 面板中配置**（支付设置属于面板；机器人仅通过 Gamaj API 使用它们，支付始终由面板官方接口确认）
- **经销商隔离**：每个 API 密钥绑定一个管理员；机器人只显示该管理员的套餐和买家
- **Mini App**（`is.0.0.1`），位于 `/miniapp`，使用 Gamaj 黑白主题
- **无数据库**：完全无状态
- 安全 Webhook：`secret_token` 与恒定时间比较

## 架构

```
Gamaj Bot  →  Gamaj API  →  Gamaj Panel Core  →  Database / Nodes
```

机器人从不直接连接数据库。购买后的用户创建经过与面板完全相同的权限/配额核心——如果经销商达到流量或用户上限，面板会拒绝订单。

## 安装

### 1) 构建

```bash
go build -o gamajbot ./cmd/gamajbot
```

### 2) 服务器安装

```bash
sudo ./scripts/install.sh
```

填写 `/opt/gamaj-bot/.gamaj-bot.json`：

| 键 | 说明 |
|---|---|
| `panel_url` | Gamaj 面板地址，如 `http://127.0.0.1:616` |
| `api_key` | Gamaj API 密钥（`gm_...`），来自面板 My Account |
| `bot_token` | 来自 [@BotFather](https://t.me/BotFather) 的令牌 |
| `admin_id` | 销售管理员的 Telegram 数字 ID |
| `webhook_secret` | 随机密钥（面板安装器自动填写） |
| `webhook_base_url` | Webhook 的公网 HTTPS 地址 |
| `listen_host` / `listen_port` | Webhook 服务地址（默认 `127.0.0.1:8080`） |

### 3) 注册 Webhook

```bash
sudo /opt/gamaj-bot/gamajbot -config /opt/gamaj-bot/.gamaj-bot.json -setup-webhook
sudo systemctl restart gamaj-bot
```

### 服务管理

```bash
sudo ./scripts/manage.sh {start|stop|restart|status|logs|update|uninstall}
```

## 从 Gamaj 面板安装（推荐）

在面板打开 **Applications** 并安装 **Gamaj Bot**——面板会自动下载二进制文件、创建 `gm_...` API 密钥、写入机器人配置并注册 Webhook。

## 直接访问（无 Webhook）

没有公网 HTTPS 时，以 long-polling 模式运行（将 `webhook_base_url` 留空）——`/healthz` 和 Mini App 仍正常服务。

## 机器人使用的 API

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/bot/plans` | 密钥所属管理员的可售套餐 |
| POST | `/api/bot/plans/{id}/orders` | 为 `telegram_id` 下单 |
| GET | `/api/bot/orders/{id}` | 订单状态 |
| GET | `/api/bot/wallet?telegram_id=` | 买家钱包余额 |
| POST | `/api/bot/orders/{id}/pay-wallet` | 钱包支付 |
| POST | `/api/bot/orders/{id}/payment-link` | 创建在线支付链接 |
| POST | `/api/bot/orders/{id}/verify` | 使用 `pay_id` 确认（必须 inquiry） |
| GET | `/api/bot/service?telegram_id=` | 买家服务与交付信息 |
| GET | `/api/bot/payment/callback` | 支付回调（仅记录） |

## 安全

- 支付响应先验证后信任；金额必须与订单完全一致。
- 支付回调防重放；没有官方确认绝不结算。
- 机密信息从不写入日志。

## 许可

© Gamaj — 保留所有权利。Coded by AsliCode。
