<p align="center">
  <a href="../README.md">فارسی</a> /
  <a href="./README-en.md">English</a> /
  <a href="./README-ru.md">Русский</a> /
  <a href="./README-zh-cn.md">简体中文</a>
</p>

<h1>GAMAJ</h1>

### Bot

**Gamaj Bot** — официальный торговый бот экосистемы **Gamaj**: автономный бинарник Go без зависимостей от базы данных. Всё работает через **Gamaj API** с выделенным ключом API бота.

- Репозиторий: `GitHub.com/TheGamaj/Bot`
- Канал: [t.me/TheGamaj](https://t.me/TheGamaj) · Поддержка: [t.me/GamajGP](https://t.me/GamajGP)
- Версия: `is.0.0.1` — Mini App: `is.0.0.1`

## Возможности

- **Полный магазин внутри Telegram**: старт → тарифы → заказ → оплата → подтверждение → автосоздание сервиса → выдача → «мои сервисы»
- **Оплата из кошелька** покупателя (транзакционный ledger в Gamaj Panel)
- **Онлайн-оплата настраивается в панели Gamaj** (платёжные настройки принадлежат панели; бот использует их только через Gamaj API, а оплата подтверждается официальным endpoint'ом панели)
- **Изоляция реселлеров**: каждый ключ API привязан к одному админу; бот видит только его тарифы и покупателей
- **Mini App** (`is.0.0.1`) на `/miniapp` в чёрно-белой теме Gamaj
- **Без базы данных**: полностью stateless
- Безопасный webhook с `secret_token` и сравнением за постоянное время

## Архитектура

```
Gamaj Bot  →  Gamaj API  →  Gamaj Panel Core  →  Database / Nodes
```

Бот никогда не подключается к базе напрямую. Создание пользователя после покупки проходит через то же ядро прав/квот панели — если реселлер исчерпал лимит, заказ отклоняет сама панель.

## Установка

### 1) Сборка

```bash
go build -o gamajbot ./cmd/gamajbot
```

### 2) Установка на сервер

```bash
sudo ./scripts/install.sh
```

Заполните `/opt/gamaj-bot/.gamaj-bot.json`:

| Ключ | Описание |
|---|---|
| `panel_url` | Адрес Gamaj Panel, напр. `http://127.0.0.1:616` |
| `api_key` | Ключ Gamaj API (`gm_...`) из My Account панели |
| `bot_token` | Токен от [@BotFather](https://t.me/BotFather) |
| `admin_id` | Числовой Telegram ID админа |
| `webhook_secret` | Случайный секрет (заполняется установщиком панели) |
| `webhook_base_url` | Публичный HTTPS-адрес webhook |
| `listen_host` / `listen_port` | Адрес сервиса webhook (по умолчанию `127.0.0.1:8080`) |

### 3) Регистрация webhook

```bash
sudo /opt/gamaj-bot/gamajbot -config /opt/gamaj-bot/.gamaj-bot.json -setup-webhook
sudo systemctl restart gamaj-bot
```

### Управление сервисом

```bash
sudo ./scripts/manage.sh {start|stop|restart|status|logs|update|uninstall}
```

## Установка из панели Gamaj (рекомендуется)

Откройте **Applications** в панели и установите **Gamaj Bot** — панель сама скачает бинарник, создаст ключ `gm_...`, запишет конфиг и зарегистрирует webhook.

## Прямой доступ (без webhook)

Без публичного HTTPS запустите бота в режиме long-polling (оставьте `webhook_base_url` пустым) — `/healthz` и Mini App продолжают работать.

## API, который использует бот

| Метод | Путь | Описание |
|---|---|---|
| GET | `/api/bot/plans` | Тарифы админа-владельца ключа |
| POST | `/api/bot/plans/{id}/orders` | Заказ для `telegram_id` |
| GET | `/api/bot/orders/{id}` | Статус заказа |
| GET | `/api/bot/wallet?telegram_id=` | Баланс кошелька |
| POST | `/api/bot/orders/{id}/pay-wallet` | Оплата из кошелька |
| POST | `/api/bot/orders/{id}/payment-link` | Ссылка онлайн-оплаты |
| POST | `/api/bot/orders/{id}/verify` | Подтверждение с `pay_id` (обязательный inquiry) |
| GET | `/api/bot/service?telegram_id=` | Сервис и данные выдачи |
| GET | `/api/bot/payment/callback` | Callback оплаты (только запись) |

## Безопасность

- Ответы оплаты проверяются до доверия; сумма должна совпадать с заказом.
- Callback оплаты защищён от повторов и не проводит оплату без официального запроса.
- Секреты никогда не пишутся в логи.

## Лицензия

© Gamaj — все права защищены. Coded by AsliCode.
