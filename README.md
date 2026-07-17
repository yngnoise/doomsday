# Doomsday

> The public path from local MVP to portfolio case study is tracked in [ROADMAP.md](ROADMAP.md).

MVP платформы лимитированных дропов одежды (модель в духе Supreme/Nike SNKRS): таймер старта/окончания продажи, ограниченный сток по размерам, бронирование товара с автоматическим сбросом при неоплате.

Архитектура и разработка (включая AI-ассистированное кодирование) — [@yngnoise](https://github.com/yngnoise). Локальный MVP, не задеплоен, без адаптивной вёрстки.

## Стек

**Backend:** Go, PostgreSQL, Redis
**Frontend:** Next.js, TypeScript

## Ключевая механика

- **Защита от оверселлинга** — атомарная проверка и списание стока через Lua-скрипт в Redis: rate-limit, проверка дубля резервации и декремент остатка выполняются одной операцией, что исключает гонку при одновременном спросе на последнюю единицу товара.
- **Лист ожидания** — Redis sorted set: если бронь истекла без оплаты, сток автоматически возвращается, и следующий в очереди получает email с предложением купить.
- **Real-time обновление остатков** через SSE — фронтенд видит актуальный сток без перезагрузки страницы.
- **Фоновый scheduler** — очищает истёкшие резервации каждые 30 секунд.
- **OTP-авторизация** — вход по коду из письма, без пароля.
- **Оплата** — реализован флоу оформления заказа (доставка, подтверждение) без интеграции реального платёжного провайдера; после оформления на почту отправляется письмо с подтверждением.

## Модули (`drop/`)

- **auth / otp_handler** — OTP-авторизация по email
- **handler** — резервация товара, оформление заказа
- **scheduler** — фоновая очистка истёкших резерваций
- **sse** — real-time обновление остатков на клиенте
- **email** — транзакционные письма (SMTP)
- **admin_handler** — админка

## Локальный запуск

Требуется Go 1.22+, Node.js, локальные PostgreSQL и Redis (порты по умолчанию: Postgres 5432, Redis 6379).

```bash
# 1. База данных
psql -f migrations/migrate.sql
psql -f seed_drops.sql   # тестовые данные (опционально)

# 2. Переменные окружения
cp .env.example .env               # задать DB, JWT и admin credentials

# Локальная ротация JWT/admin-секретов без вывода значений в консоль
powershell -ExecutionPolicy Bypass -File scripts/rotate-local-secrets.ps1

# 3. Backend
go run main.go

# 4. Frontend
npm install
npm run dev
```

## End-to-end browser tests

The Playwright suite starts the Go API and Next.js app, creates isolated drops through the admin API, and exercises OTP sign-in, reservation, simulated payment, retry, expiry, waitlist, confirmation, and refund flows.

```bash
npx playwright install chromium
npm run test:e2e
```

The fixed OTP and reservation-expiry controls are available only when `APP_ENV=test`; they are not registered in production.

## Safe portfolio demo

The repository includes production container images, a local four-service demo stack, and a Render Blueprint. The demo uses simulated payments, disables outbound email, exposes its OTP in the sign-in UI, and recreates disposable data on each API start.

```bash
docker compose -f compose.demo.yml up --build
```

Open `http://localhost:3000`. See [docs/deployment.md](docs/deployment.md) for deployment, verification, data reset, and rollback instructions.

## Статус проекта

Локальный MVP: основная механика (бронирование, антиоверселлинг, лист ожидания, OTP) реализована и работает. Нет: адаптивной вёрстки, интеграции с реальным платёжным провайдером, деплоя.
