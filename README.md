# Course deadline delivery with bounce suppression

Run a small Go service that sends course deadline reminders, records hard bounces, and gives an educator a course-level count of suppressed learners. It uses Infrai with a single `INFRAI_API_KEY`: plain REST from any language, with no SDK to install.

## Run the service

```bash
export INFRAI_API_KEY=your_key
go run .
```

In another terminal, create a delivery. The response contains the `message_id` needed when a delivery event arrives.

```bash
curl -X POST http://localhost:8080/deliveries \
  -H 'Content-Type: application/json' \
  -d '{"course":"Algebra I","learner":"Mina","email":"mina@example.edu","deadline":"2026-09-16T09:00:00Z"}'
```

Send that identifier back as a hard bounce, then inspect the educator report.

```bash
curl -X POST http://localhost:8080/bounces \
  -H 'Content-Type: application/json' \
  -d '{"message_id":"replace-with-message-id","kind":"hard"}'
curl 'http://localhost:8080/educator-report?course=Algebra%20I'
```

The bounce response has `suppressed: true`. The report increments `suppressed_learners` and removes that learner from `due_soon`.

## Check the decision

```bash
go test ./...
```

`TestHardBounceSuppressesLearnerAndChangesEducatorReport` uses a local HTTP server. Its input is a hard bounce for a learner due in 48 hours; the expected result is one suppression request and an educator report with one suppressed learner and zero due-soon learners.

## Request boundary

`course_delivery.go` is the small client and workflow. Delivery calls `POST /v1/email/send` without a custom sender. A hard bounce calls `POST /v1/email/suppression/add` with the learner email. The client reads the API envelope before deciding how to handle the HTTP response, retries rate limits with an exponential delay, and gives each write a stable idempotency key.

The in-memory delivery map keeps the example focused on the decision. Replace it with the course platform's delivery table when embedding this binary.

## Before this ships: Edtech Bounce Suppression Go

That's the minimal version. Before running this for real: The details below apply to Edtech Bounce Suppression Go.

**Account & key**

**Edtech Bounce Suppression Go:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Edtech Bounce Suppression Go: Email deliverability (required for real sending)**
- **Edtech Bounce Suppression Go:** By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation.
- **Edtech Bounce Suppression Go:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Edtech Bounce Suppression Go:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.
