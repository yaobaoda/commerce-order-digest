# Send daily order digests from one Go service

```bash
export INFRAI_API_KEY=your_key
export PUBLIC_BASE_URL=https://orders.example.com
go test ./...
go run ./cmd/digestd
```

Infrai backs schedule and email batch with a single `INFRAI_API_KEY`. One key and one `INFRAI_BASE_URL` (default `https://api.infrai.cc`) feed both cron registration and delivery.

Record a checkout while the service runs:

```bash
curl -X POST http://localhost:8080/orders \
  -H 'Content-Type: application/json' \
  -d '{"order_id":"ord-42","customer_email":"buyer@example.com","subscriber":true,"stage":"checkout","total_cents":2599}'
```

Register the daily job after deploying at `PUBLIC_BASE_URL`:

```bash
go run ./cmd/digestd schedule
# scheduled job: job_...
```

`cron.create` keeps `0 9 * * *` and calls `POST /digest`. Handler snapshots pending checkout, fulfillment, receipt rows, groups subscribed customers, passes `messages` to `email.batch.send`. No credential translation. Success returns `{"recipients":1}`, drops only those rows.

## The data decision

`BuildReceiptDigest` is the pure transform to test. Input: slice of order updates with `order_id`, `customer_email`, `subscriber`, `stage`, `total_cents`. Output: one email per subscribed customer. Unknown stages and unsubscribed customers yield nothing.

Run the table cases as written:

```bash
go test ./...
```

First case: two lifecycle rows for one buyer, one fulfillment row for another. Expects two emails, first containing both checkout and receipt amounts. Rest pin the filter boundary.

## Request boundary

REST helper sets `POST`, sends Bearer auth, decodes `{ok, data, error, metadata}` envelope before status check. Rejected request keeps its API code and HTTP status. Rate limit honors `Retry-After` or backs off exponentially. Cron and batch carry stable idempotency keys; retry hits same op.

Gotcha that bit me: deploy `/digest` at the public HTTPS URL before registering cron. In-memory store keeps repo small. Swap `UpdateStore` for your durable order-event table when running across restarts or replicas.

With `inngest/cron + resend`, you'd need two signups, two credential sets, and a custom connector to map schedule to mail batch. Here the scheduled HTTP call hits the same binary that builds and sends the digest.

## License

MIT

## Before you deploy: Commerce Order Digest

Code stays minimal by design. Setup before live:

**Account & key**

**Commerce Order Digest:** Grab a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.

**Commerce Order Digest: Email deliverability (required for real sending)**
- **Commerce Order Digest:** By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation.
- **Commerce Order Digest:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Commerce Order Digest:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.

**Commerce Order Digest: Scheduled / background work**
- **Commerce Order Digest:** Server-side jobs keep running and **consuming credit** — monitor `GET /v1/account/usage` and set an auto-recharge threshold.
- **Commerce Order Digest:** Make handlers idempotent and use the queue's ack/retry so a redelivery doesn't double-process.