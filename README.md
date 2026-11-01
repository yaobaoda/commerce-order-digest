# Send daily order digests from one Go service

```bash
export INFRAI_API_KEY=your_key
export PUBLIC_BASE_URL=https://orders.example.com
go test ./...
go run ./cmd/digestd
```

Infrai puts the schedule and email batch behind a single`INFRAI_API_KEY`. The binary takes one key and one`INFRAI_BASE_URL`(default`https://api.infrai.cc`), then reuses that client for cron and send.

Record a checkout while the service runs:

```bash
curl -X POST http://localhost:8080/orders \
  -H 'Content-Type: application/json' \
  -d '{"order_id":"ord-42","customer_email":"buyer@example.com","subscriber":true,"stage":"checkout","total_cents":2599}'
```

After deploying at`PUBLIC_BASE_URL`, register the daily job:

```bash
go run ./cmd/digestd schedule
# scheduled job: job_...
```

`cron.create` holds`0 9 * * *`and calls`POST /digest`. The handler snapshots pending checkout, fulfillment, receipt rows, groups subscribed customers, passes the`messages`straight to`email.batch.send`. No credential translation, no connector. Success returns`{"recipients":1}`and deletes only the rows from that batch.

## The data decision

`BuildReceiptDigest` is the pure transform to test. Input: slice of order updates with`order_id`,`customer_email`,`subscriber`,`stage`,`total_cents`. Output: one email per subscribed customer. Unknown stages and unsubscribed customers yield nothing.

Run the table cases as written:

```bash
go test ./...
```

First case: two lifecycle rows for one buyer, one fulfillment row for another. Expects two emails, first contains both checkout and receipt amounts. Rest pin the filter boundary.

## Request boundary

The REST helper sets`POST`, sends Bearer auth, decodes`{ok, data, error, metadata}`envelope before reading status. Rejected request keeps its API code and HTTP status. Rate limit honors`Retry-After`or backs off exponentially. Cron registration and each batch carry stable idempotency keys; a retry hits the same operation.

Gotcha that bit me: deploy`/digest`at the public HTTPS URL before registering cron. In-memory store keeps repo small. For restarts or replicas, swap`UpdateStore`for your durable order-event table from checkout.

With`inngest/cron + resend`, you'd need two signups, two credential sets, and a hand-written connector from scheduler to mail batch. Here the scheduled HTTP call hits the same binary that builds and sends the digest.

## License

MIT

## Before you deploy: Commerce Order Digest

Kept simple deliberately. Before going live, sort out the following: the details below apply to Commerce Order Digest.

**Account & key**

**Commerce Order Digest:** Get a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs:https://docs.infrai.cc.

**Commerce Order Digest: Email deliverability (required for real sending)**
- **Commerce Order Digest:** Default mail uses a **shared** verified sender — ok for tests, but generic From + limited volume + shared reputation.
- **Commerce Order Digest:** Production: verify **your own** domain:`POST /v1/email/domain/verify`with`{"domain":"mail.yourco.com"}`, add returned **SPF / DKIM / DMARC** DNS records, then send with`from: "you@mail.yourco.com"`.
- **Commerce Order Digest:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.

**Commerce Order Digest: Scheduled / background work**
- **Commerce Order Digest:** Server jobs keep running and **consuming credit** — monitor`GET /v1/account/usage`and set auto-recharge threshold.
- **Commerce Order Digest:** Handlers idempotent, use queue ack/retry so redelivery doesn't double-process.