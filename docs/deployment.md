# Safe demo deployment

This deployment profile is intended for a public portfolio demo. It must not share PostgreSQL or Redis with a production environment.

## Safety model

- Checkout uses the in-repository payment simulator and never contacts an acquirer.
- `DEMO_MODE=true` disables SMTP even if SMTP credentials are present.
- The OTP returned by the API is shown in the sign-in UI. This is intentional for the demo and is not real authentication.
- `DEMO_RESET_ON_START=true` truncates the connected database and flushes the selected Redis database before seeding a fresh demo drop.
- The destructive reset refuses to run unless `DEMO_MODE=true`.
- JWT, payment webhook, database, Redis, and admin credentials are environment-managed. No deploy secret is stored in Git.

## Local deployment

Requirements: Docker Engine with Compose v2.

Copy `.env.example` to `.env` and replace the three credential placeholders with unique values. The Compose profile reads `JWT_SECRET`, `PAYMENT_WEBHOOK_SECRET`, and `ADMIN_PASSWORD` from that file.

```bash
docker compose -f compose.demo.yml up --build -d
docker compose -f compose.demo.yml ps
curl --fail http://localhost:8080/health/live
curl --fail http://localhost:8080/health/ready
curl --fail http://localhost:3000/health
```

Open `http://localhost:3000`. Any email-shaped value works in demo mode; the UI displays the six-digit portfolio demo code after it is requested. Payment scenarios are simulations and cannot create a real charge.

Stop the stack with:

```bash
docker compose -f compose.demo.yml down
```

Add `--volumes` only when intentionally discarding the local PostgreSQL volume.

## Render Blueprint

1. Fork or connect this repository to a Render workspace.
2. Create a Blueprint and select the repository. Render reads `render.yaml`.
3. Review the four resources before applying: web, API, PostgreSQL, and Key Value.
4. After the first successful deployment, open the `doomsday-web` `onrender.com` URL.
5. Verify `/health` on the web service and `/health/live` plus `/health/ready` on the API service.

The Blueprint uses free plans. Render free web services can spin down while idle, free Key Value storage is non-persistent, and free PostgreSQL databases expire after 30 days. Treat this as an evaluation deployment, not durable hosting. Recreate the Blueprint or move PostgreSQL to a paid plan before its expiration if the public URL must remain usable.

`SITE_URL` and `CORS_ORIGINS` on the API contain a deliberately non-routable placeholder because browser traffic reaches the API through the Next.js same-origin proxy. Replace both values with the final web URL before enabling direct browser-to-API requests.

## Reset demo data

The default profile resets automatically whenever the API starts. A manual reset is also available to an authenticated admin:

```bash
curl --fail \
  -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  https://YOUR_API_HOST/api/admin/demo/reset
```

The reset recreates `demo-wraith-jacket` with 120 units and a new 24-hour drop window.

## Rollback

1. In Render, open each web service's deploy history.
2. Select the last known-good deploy and choose **Rollback**.
3. Roll back the API before the web service when an API contract changed.
4. Check all three health endpoints.
5. Restart the API if disposable demo data needs to be recreated.

Database migrations are additive and idempotent. A code rollback does not reverse schema changes. For a destructive future migration, take a database backup and provide a separate, reviewed down migration before deployment.

For the local stack, check out the last known-good Git revision and rebuild:

```bash
git switch --detach LAST_KNOWN_GOOD_SHA
docker compose -f compose.demo.yml up --build -d
```
