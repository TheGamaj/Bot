# Gamaj Bot — راهنمای نصب دستی و آفلاین

**Gamaj Bot | گمج بات** — نسخه `is.0.0.1` — Mini App: `is.0.0.1`

> Coded by AsliCode

- Repository: `GitHub.com/TheGamaj/Bot`
- کانال تلگرام گمج: [t.me/TheGamaj](https://t.me/TheGamaj)
- گروه تلگرام گمج: [t.me/GamajGP](https://t.me/GamajGP)
- کانال تلگرام سازنده (AsliCode): [t.me/AsliCode](https://t.me/AsliCode)

گمج بات **مستقل** است و بدون پنل هم باینری‌اش اجرا می‌شود، اما برای فروش به **Gamaj API** (گمج پنل روی پورت **616**) نیاز دارد. هرگز مستقیم به دیتابیس وصل نمی‌شود.

---

## ۱. پیش‌نیازها

| نیاز | مقدار |
|---|---|
| سیستم‌عامل | Linux amd64/arm64 (systemd) |
| پنل | یک گمج پنل نصب‌شده روی `http(s)://PANEL:616` |
| کلید API | یک کلید `gm_...` متعلق به ادمین فروش (ریسلر یا ادمین اصلی) |
| توکن ربات | از [@BotFather](https://t.me/BotFather) |
| HTTPS عمومی | فقط برای webhook؛ در غیر این صورت long-polling |

## ۲. نصب خودکار از داخل پنل (توصیه‌شده)

پنل → **Applications → Gamaj Bot → Install**. پنل دانلود باینری، ساخت systemd، ساخت کلید API، نوشتن کانفیگ مخفی و ثبت webhook را کامل خودکار انجام می‌دهد.

## ۳. نصب دستی (روش ۱)

### ۳.۱ باینری

```bash
# از سورس
git clone https://github.com/TheGamaj/Bot.git Gamaj-Bot && cd Gamaj-Bot
sudo apt-get install -y golang-go
go build -o gamajbot ./cmd/gamajbot

# یا از Release (بدون گیت و بدون Go)
wget https://github.com/TheGamaj/Bot/releases/latest/download/gamaj-bot-linux-amd64 -O gamajbot
chmod +x gamajbot
```

### ۳.۲ نصب

```bash
sudo ./scripts/install.sh
```

### ۳.۳ کانفیگ

`/opt/gamaj-bot/.gamaj-bot.json` را پر کنید:

```json
{
  "panel_url": "https://panel.example.com:616",
  "api_key": "gm_............",
  "bot_token": "123456:ABC...",
  "admin_id": "123456789",
  "webhook_secret": "یک-مقدار-تصادفی-طولانی",
  "listen_host": "127.0.0.1",
  "listen_port": 8080,
  "webhook_base_url": "https://bot.example.com"
}
```

```bash
chmod 600 /opt/gamaj-bot/.gamaj-bot.json
```

### ۳.۴ ثبت webhook و اجرا

```bash
sudo /opt/gamaj-bot/gamajbot -config /opt/gamaj-bot/.gamaj-bot.json -setup-webhook
sudo systemctl restart gamaj-bot
```

بدون HTTPS عمومی: `webhook_base_url` را خالی بگذارید؛ بات به‌جای webhook رشد long-polling را مدیریت می‌کند (سرویس Mini App و healthz همچنان روی `listen_host:listen_port` بالا می‌مانند).

## ۴. نصب آفلاین (روش ۲)

### ۴.۱ پکیج‌سازی روی ماشین آنلاین

```bash
mkdir gamaj-bot-offline
cp gamajbot scripts/*.sh gamaj-bot-offline/
tar czf gamaj-bot-offline.tar.gz gamaj-bot-offline
```

### ۴.۲ نصب روی سرور آفلاین

```bash
tar xzf gamaj-bot-offline.tar.gz && cd gamaj-bot-offline
sudo ./install.sh
# کانفیگ را طبق بخش ۳.۳ پر کنید
sudo systemctl restart gamaj-bot
```

> توجه: در حالت آفلاین، دانلود خودکار باینری توسط پنل کار نمی‌کند؛ همان باینریِ همراه پکیج استفاده می‌شود.

## ۵. تنظیم فروش (در پنل)

1. **Settings → Sales (فروش)**: گزینه **پرداخت آنلاین** را فعال و Base URL + API Key را وارد کنید (کلید فقط ذخیره می‌شود و هرگز نمایش داده نمی‌شود).
2. پلن‌های فروش از طریق `/api/bot/plans` یا مستقیم از پنل/ریسلر ساخته می‌شوند.
3. سقف‌های ادمین (حجم ساخته‌شده و تعداد کاربر) دقیقاً روی فروش بات اعمال می‌شود؛ ریسلری با سقف پر شده نمی‌تواند از طریق بات هم بفروشد.

## ۶. مدیریت

```bash
sudo /opt/gamaj-bot/scripts/manage.sh {start|stop|restart|status|logs|update|uninstall}
# یا
sudo systemctl {status|restart} gamaj-bot
journalctl -u gamaj-bot -f
```

## ۷. Mini App

صفحه Mini App (`is.0.0.1`) با تم مشکی/سفید گمج روی `/miniapp` سرو می‌شود؛ آدرس آن را در BotFather از طریق Menu Button وصل کنید.

---

© Gamaj | گمج — Coded by AsliCode
