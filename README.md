# Idempotency Gateway — The Pay-Once Protocol

> A middleware service that guarantees payment requests are processed **exactly once**, no matter how many times a client retries.

Built for **FinSafe Transactions Ltd.** · Go · REST API · In-memory store

---

## Table of Contents

- [Overview](#overview)
- [Architecture Diagram](#architecture-diagram)
- [Project Structure](#project-structure)
- [Setup Instructions](#setup-instructions)
- [API Documentation](#api-documentation)
- [Postman Testing Guide](#postman-testing-guide)
- [Design Decisions](#design-decisions)
- [Developer's Choice Feature — TTL Key Expiry](#developers-choice-feature--ttl-key-expiry)

---

## Overview

### The Problem

E-commerce clients of FinSafe occasionally experience network timeouts. When this happens, their servers automatically retry payment requests. Without protection, FinSafe processes both requests and **charges the customer twice**.

### The Solution

This gateway intercepts every payment request and checks for a unique `Idempotency-Key` header:

- **First request** → process the payment, save the response, return `201 Created`
- **Duplicate request (same key, same body)** → return the saved response instantly with `X-Cache-Hit: true`, no re-processing
- **Tampered request (same key, different body)** → reject with `409 Conflict`
- **Simultaneous duplicate (race condition)** → block the second request until the first finishes, then return its result

---

## Architecture Diagram

```
┌──────────────────────────────────────────────────────────────┐
│                  CLIENT (e-commerce store)                   │
│              POST /process-payment                           │
│         Header: Idempotency-Key: <unique-string>             │
│         Body:   {"amount": 100, "currency": "GHS"}           │
└──────────────────────────┬───────────────────────────────────┘
                           │
                           ▼
┌──────────────────────────────────────────────────────────────┐
│              MIDDLEWARE  (middleware/idempotency.go)          │
│                                                              │
│  1. Is Idempotency-Key header present?                       │
│     └── NO  ──────────────────────────────► 400 Bad Request  │
│     └── YES                                                  │
│          │                                                   │
│  2. Hash the request body with SHA-256                       │
│          │                                                   │
│  3. Lock store → look up the key                             │
│     │                                                        │
│     ├── KEY NOT FOUND or EXPIRED ──────────────────────┐    │
│     │                                                   │    │
│     └── KEY FOUND                                       │    │
│          │                                              │    │
│          ├── IsProcessing = true  (IN-FLIGHT)           │    │
│          │    └── Wait on record.Done channel           │    │
│          │         └── First request finishes           │    │
│          │              └── 200 OK + X-Cache-Hit: true  │    │
│          │                                              │    │
│          └── IsProcessing = false (completed)           │    │
│               ├── Hash MATCHES → 200 OK + X-Cache-Hit   │    │
│               └── Hash MISMATCH → 409 Conflict          │    │
└────────────────────────────────────────────────────────-┼────┘
                                                          │
                    ┌─────────────────────────────────────┘
                    │  New record created (IsProcessing=true)
                    ▼
┌──────────────────────────────────────────────────────────────┐
│                 STORE  (database/store.go)                    │
│                                                              │
│   map[string]*IdempotencyRecord  +  sync.RWMutex             │
│                                                              │
│   IdempotencyRecord {                                        │
│       RequestHash  string        ← SHA-256 of body           │
│       Response     PaymentResponse                           │
│       StatusCode   int                                       │
│       IsProcessing bool          ← in-flight flag            │
│       CreatedAt    time.Time     ← for TTL expiry            │
│       Done         chan struct{}  ← broadcast signal         │
│   }                                                          │
└──────────────────────────┬───────────────────────────────────┘
                           │
                           ▼
┌──────────────────────────────────────────────────────────────┐
│               HANDLER  (handlers/payment.go)                 │
│                                                              │
│   1. Decode JSON body → PaymentRequest                       │
│   2. time.Sleep(2s)  ← simulates bank processing             │
│   3. Build response: "Charged {amount} {currency}"           │
│   4. Save response + status code into store record           │
│   5. IsProcessing = false                                    │
│   6. close(record.Done) ← unblocks ALL waiting goroutines    │
│   7. Return 201 Created                                      │
└──────────────────────────┬───────────────────────────────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │  201 Created    │
                  │ "Charged 100    │
                  │     GHS"        │
                  └─────────────────┘
```

### TTL Check (Developer's Choice)
```
On every KEY FOUND lookup:
    time.Since(record.CreatedAt) > 24h ?
        YES → delete old record, treat as NEW request → 201
        NO  → continue with cached response → 200 + X-Cache-Hit: true
```

---

## Project Structure

```
├── config/
│   └── config.go          # App settings: port, processing delay, TTL duration
├── database/
│   └── store.go           # In-memory store: map + RWMutex + CRUD methods
├── models/
│   └── payment_models.go  # Structs: PaymentRequest, PaymentResponse, IdempotencyRecord
├── middleware/
│   └── idempotency.go     # Core logic: all idempotency checks before the handler
├── handlers/
│   └── payment.go         # HTTP handler: simulates processing, saves result, signals done
├── main.go                # Entry point: wires store → middleware → handler → server
└── README.md              # This file
```

---

## Setup Instructions

### Prerequisites

- Go 1.21 or higher
- Git
- Postman (optional, for manual testing)

### Run the server

```bash
# 1. Clone the repository
git clone https://github.com/kwabsntim/idempotency-gateway.git
cd idempotency-gateway

# 2. Download dependencies
go mod tidy

# 3. Start the server
go run main.go
```

Server starts on `http://localhost:8080`

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Server port |

```bash
PORT=9000 go run main.go
```

---

## API Documentation

### `POST /process-payment`

Processes a payment. Safe to retry — identical requests are deduplicated automatically.

#### Request Headers

| Header | Required | Description |
|---|---|---|
| `Idempotency-Key` | Yes | Unique string per transaction. Reuse the same key on retries. |
| `Content-Type` | Yes | `application/json` |

#### Request Body

```json
{
  "amount": 100,
  "currency": "GHS"
}
```

#### Response Scenarios

| Status | Scenario | Extra Header |
|---|---|---|
| `201 Created` | New request, payment processed | — |
| `200 OK` | Duplicate request, cached response returned | `X-Cache-Hit: true` |
| `409 Conflict` | Same key reused with a different request body | — |
| `400 Bad Request` | `Idempotency-Key` header is missing | — |

#### `201 Created` — New payment

```json
{
  "message": "Charged 100 GHS",
  "status": "success"
}
```

> Takes ~2 seconds (simulated processing delay).

#### `200 OK` — Duplicate request

```json
{
  "message": "Charged 100 GHS",
  "status": "success"
}
```

> Instant response. `X-Cache-Hit: true` header confirms this is a replayed response.

#### `409 Conflict` — Key reused with different body

```json
{
  "error": "Idempotency key already used for a different request body."
}
```

#### `400 Bad Request` — Missing header

```json
{
  "error": "Idempotency-Key header is required"
}
```

---

## Postman Testing Guide

Base URL: `http://localhost:8080`

### Test 1 — New Payment (Happy Path)

```
POST /process-payment
Idempotency-Key: test-key-001
Content-Type: application/json

{"amount": 100, "currency": "GHS"}
```

Expected: `201 Created` after ~2 seconds.

---

### Test 2 — Duplicate Request

Send the exact same request as Test 1 again.

Expected: `200 OK` instantly. Header `X-Cache-Hit: true` present.

---

### Test 3 — Different Body, Same Key

```
POST /process-payment
Idempotency-Key: test-key-001
Content-Type: application/json

{"amount": 500, "currency": "GHS"}
```

Expected: `409 Conflict`.

---

### Test 4 — Missing Header

Remove `Idempotency-Key` header entirely.

Expected: `400 Bad Request`.

---

### Test 5 — Race Condition (Bonus)

Open two terminal tabs and fire both commands immediately one after the other:

```bash
# Tab 1
curl -X POST http://localhost:8080/process-payment \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: key-race" \
  -d '{"amount": 100, "currency": "GHS"}'

# Tab 2 (fire immediately after Tab 1)
curl -X POST http://localhost:8080/process-payment \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: key-race" \
  -d '{"amount": 100, "currency": "GHS"}'
```

Expected: One returns `201`, the other returns `200 + X-Cache-Hit: true`. Only one processed the payment.

---

### Test 6 — TTL Expiry

TTL is hardcoded to 24 hours. To test without waiting, temporarily change `IdempotencyTTL` in `config/config.go` to `5 * time.Second`, restart the server, then:

1. Send request with key `test-key-ttl` → `201 Created`
2. Wait 6 seconds
3. Send same request again → `201 Created` (key expired, fresh process)

---

## Design Decisions

### In-memory store instead of a database

Used a Go `map[string]*IdempotencyRecord` protected by `sync.RWMutex`. Zero setup, zero dependencies, O(1) lookup. Data is lost on server restart which is acceptable for this scope.

In production: **Redis** — native TTL support, shared across server instances, persists across restarts.

### `sync.RWMutex` instead of `sync.Mutex`

`RLock()` allows multiple goroutines to read simultaneously. Only writes are exclusive. A payment gateway is read-heavy (most requests are duplicate lookups), so this is more performant than a plain `Mutex`.

### SHA-256 body hashing instead of raw JSON comparison

Hashing the body produces a fixed 64-character string regardless of payload size. Comparison is always O(1). Any change to the body — even a single character — produces a completely different hash.

### `chan struct{}` for in-flight signalling

`close(record.Done)` broadcasts to ALL goroutines blocking on `<-record.Done` simultaneously. This is the idiomatic Go pattern for one-to-many signalling. Polling with `time.Sleep` would waste CPU and add latency.

### Middleware over handler logic

All idempotency checking happens in middleware before the handler runs. The handler only processes new payments. Clean separation of concerns — each unit has one job.

### Atomic check-and-create with manual lock

The middleware holds `store.Lock()` across both the `GetUnsafe` and `SetUnsafe` calls. This prevents two goroutines from both seeing "key not found" and both creating a record simultaneously — which would cause a double charge.

---

## Developer's Choice Feature — TTL Key Expiry

### What it does

Idempotency keys automatically expire after **24 hours**. After expiry, the same key can be used for a new payment and will be processed fresh.

### Why it matters

Without TTL:
1. The in-memory store grows forever — every key ever used stays in RAM
2. A retry from last week would incorrectly return a cached response from a long-dead transaction



### How it works

Every `IdempotencyRecord` stores a `CreatedAt time.Time`. In the middleware, when a key is found, the TTL is checked before anything else:

```go
if store.IsExpired(record) {
    // older than 24 hours — delete and treat as a brand new request
    store.DeleteUnsafe(key)
}
```

Cleanup is lazy — expired records are removed on next access, not by a background timer. No goroutines, no schedulers, no extra complexity.

---
