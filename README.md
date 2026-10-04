<div align="center">

<img src="./docs/assets/gamaj-logo.svg" alt="Gamaj" width="104" height="104">

<h1>GAMAJ</h1>

### Bot

</div>

<p align="center">
  <a href="./README.md">فارسی</a> /
  <a href="./docs/README-en.md">English</a> /
  <a href="./docs/README-ru.md">Русский</a> /
  <a href="./docs/README-zh-cn.md">简体中文</a>
</p>

<p align="center">
  <a href="https://github.com/TheGamaj/Bot/releases"><img alt="Release" src="https://img.shields.io/badge/release-is.0.0.1-2ea043?style=flat-square" /></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go&logoColor=white" />
  <img alt="Platform" src="https://img.shields.io/badge/platform-Telegram-26A5E4?style=flat-square" />
  <img alt="License" src="https://img.shields.io/badge/license-AGPL--3.0-lightgrey?style=flat-square" />
</p>

**Gamaj Bot** ربات رسمی فروش اکوسیستم **Gamaj | گمج** است — یک باینری مستقل Go بدون هیچ وابستگی به دیتابیس. تمام عملیات از طریق **Gamaj API** با کلید API اختصاصی بات انجام می‌شود.

- Repository: `GitHub.com/TheGamaj/Bot`
- کانال تلگرام گمج: **t.me/TheGamaj**
- گروه تلگرام گمج: **t.me/GamajGP**
- کانال سازنده: **t.me/AsliCode**
- نسخه: `is.0.0.1` — Mini App: `is.0.0.1`

> Coded by AsliCode

---

## ویژگی‌ها

- **فروش کامل از داخل تلگرام**: شروع → پلن‌ها → سفارش → پرداخت → تأیید → ساخت خودکار سرویس → تحویل اطلاعات → سرویس‌های من
- **پرداخت از کیف پول** خریدار (ledger تراکنشی و ضد دوباره‌پرداخت در Gamaj Panel)
- **پرداخت آنلاین از طریق تنظیمات پنل گمج** (تنظیمات پرداخت متعلق به پنل است و بات فقط با Gamaj API از آن استفاده می‌کند — کلید API و Base URL فقط از تنظیمات ادمین پنل، هرگز hard-code نمی‌شود؛ تأیید پرداخت همیشه با endpoint رسمی پنل انجام می‌شود)
- **Isolation ریسلر**: هر کلید API متعلق به یک ادمین است؛ بات فقط پلن‌ها و خریداران همان ادمین را می‌بیند
- **Mini App** (is.0.0.1) با تم مشکی/سفید گمج روی `/miniapp`
- **بدون دیتابیس**: هیچ فایل دیتابیسی، هیچ اتصال MySQL — کاملاً stateless
- Webhook امن با `secret_token` و مقایسه constant-time

## معماری

```
Gamaj Bot  →  Gamaj API  →  Gamaj Panel Core  →  Database / Nodes
```

بات هرگز مستقیم به دیتابیس وصل نمی‌شود. ساخت کاربر پس از خرید دقیقاً از همان هسته‌ی permissions/quota پنل عبور می‌کند؛ یعنی اگر ریسلر به سقف حجم یا تعداد کاربر رسیده باشد، سفارش او در پنل رد می‌شود.

## نصب یک‌خطی (توصیه‌شده)

```bash
curl -fsSL https://raw.githubusercontent.com/TheGamaj/Bot/Asli/scripts/install.sh | sudo bash
```

اینستالر به‌صورت کاملاً خودکار:

- از هر مسیری اجرا می‌شود و پوشه‌ی پروژه را خودش پیدا می‌کند؛
- اگر باینری وجود داشته باشد از همان استفاده می‌کند؛ وگرنه اول از ریلیز رسمی گیت‌هاب دانلود می‌کند و در نبود اینترنت به گیت‌هاب، **Go را نصب و باینری را از سورس می‌سازد**؛
- `/opt/gamaj-bot` را با کانفیگ محافظت‌شده (perm 600) می‌سازد؛
- سرویس systemd را نصب، فعال و **بعد از استارت، سلامتش را verify می‌کند**؛
- پیام next-step با مسیر دقیق کانفیگ و دستور ثبت webhook چاپ می‌کند.

پوشه‌ی پیشنهادی برای کلون دستی:

```bash
cd /opt && git clone https://github.com/TheGamaj/Bot.git Gamaj-Bot && cd Gamaj-Bot
```

## نصب از ریلیز رسمی (بدون گیت و بدون Go)

```bash
curl -fL -o gamajbot https://github.com/TheGamaj/Bot/releases/latest/download/gamaj-bot-linux-amd64
chmod +x gamajbot
```

سپس اینستالر را اجرا کنید تا سرویس ساخته شود.

## کانفیگ

فایل `/opt/gamaj-bot/.gamaj-bot.json` را پر کنید:

| کلید | توضیح |
|---|---|
| `panel_url` | آدرس Gamaj Panel، مثل `http://127.0.0.1:616` |
| `api_key` | کلید API گمج (`gm_...`) — از My Account پنل یا از نصب از داخل Applications |
| `bot_token` | توکن ربات از **t.me/BotFather** |
| `admin_id` | شناسه عددی تلگرام ادمین فروش |
| `webhook_secret` | مقدار تصادفی مخفی (خودکار در نصب از پنل) |
| `webhook_base_url` | آدرس عمومی HTTPS برای webhook، مثل `https://bot.example.com` |
| `listen_host` / `listen_port` | آدرس سرویس webhook (پیش‌فرض `127.0.0.1:8080`) |

سپس:

```bash
sudo /opt/gamaj-bot/gamajbot -config /opt/gamaj-bot/.gamaj-bot.json -setup-webhook
sudo systemctl restart gamaj-bot
```

## نصب از داخل Gamaj Panel (توصیه‌شده)

در پنل به **Applications** بروید و **Gamaj Bot** را نصب کنید. پنل به‌صورت خودکار:

1. باینری بات را از `github.com/TheGamaj/Bot` دانلود و systemd سرویس را می‌سازد؛
2. یک کلید API اختصاصی `gm_...` برای ادمین می‌سازد؛
3. فایل کانفیگ مخفی بات را با `panel_url`، `api_key` و توکن ربات می‌نویسد؛
4. webhook را ثبت می‌کند.

## مدیریت سرویس

```bash
sudo ./scripts/manage.sh {status|logs|start|stop|restart|config|webhook|update|uninstall}
```

## دسترسی مستقیم (بدون webhook)

اگر HTTPS عمومی ندارید، بات را در حالت long-polling راه‌اندازی کنید (webhook_base_url را خالی بگذارید) — سرویس `/healthz` و Mini App همچنان بالا می‌مانند.

## API که بات استفاده می‌کند

| متد | مسیر | توضیح |
|---|---|---|
| GET | `/api/bot/plans` | پلن‌های قابل فروش ادمین مالک کلید |
| POST | `/api/bot/plans/{id}/orders` | ثبت سفارش برای `telegram_id` |
| GET | `/api/bot/orders/{id}` | وضعیت سفارش |
| GET | `/api/bot/wallet?telegram_id=` | موجودی کیف پول خریدار |
| POST | `/api/bot/orders/{id}/pay-wallet` | پرداخت از کیف پول |
| POST | `/api/bot/orders/{id}/payment-link` | ساخت لینک پرداخت آنلاین |
| POST | `/api/bot/orders/{id}/verify` | تأیید پرداخت با `pay_id` (استعلام اجباری) |
| GET | `/api/bot/service?telegram_id=` | سرویس و اطلاعات تحویل خریدار |
| GET | `/api/bot/payment/callback` | کال‌بک پرداخت (فقط ثبت — تسویه با استعلام رسمی) |

## مجوزها و امنیت

- کلید API فقط Bearer است و به یک ادمین گره خورده؛ سقف حجم/تعداد همان ادمین روی فروش اعمال می‌شود.
- پاسخ‌های پرداخت قبل از اعتماد validate می‌شوند؛ مقدار پرداخت باید دقیقاً با سفارش برابر باشد.
- کال‌بک پرداخت replay-safe است و هیچ‌گاه بدون استعلام رسمی تسویه نمی‌کند.
- secretها هرگز در لاگ چاپ نمی‌شوند.

## برند و آیکن‌ها

ست آیکن‌های `docs/assets/` از هندسهٔ خود حرف G رندر می‌شود، نه دستی:

```bash
node tools/render-brand-assets.mjs docs/assets
bash scripts/ci/brand-assets-check.sh   # رندر مجدد و مقایسه با brand-assets.sha256
```

اگر خروجی رندر تازه با فایل‌های commit‌شده فرق کند، اسکریپت خطا می‌دهد. همین بررسی
داخل جاب `workflow-guard` در CI هم اجرا می‌شود.

## لایسنس

© Gamaj | گمج — تمام حقوق محفوظ است.

Coded by AsliCode
