# CLAUDE.md — Idempotency Gateway Project Context

> This file tells the AI assistant (Claude, Copilot, Cursor, etc.) everything it needs
> to know about this project so it can give you accurate, context-aware help.
> Keep this file in the ROOT of your project folder.

---

## What This Project Is

This is a **RESTful API** built in **Go** that acts as an **Idempotency Gateway** for a
payment processor called FinSafe Transactions Ltd.

**The core problem it solves**: When a client retries a payment request (due to network
timeout), this service ensures the payment is only processed ONCE — no double charges.

**How it works in one sentence**: Every payment request carries a unique
`Idempotency-Key` header. The server stores the result of the first request. Any
duplicate request with the same key gets the stored result back immediately — no
re-processing.

---

## Tech Stack

- **Language**: Go (Golang)
- **Router**: `net/http` standard library (or `gorilla/mux` if added)
- **Storage**: In-memory `map[string]*IdempotencyRecord` protected by `sync.RWMutex`
- **No external database** — data lives in memory (resets on server restart)
- **Testing**: Postman for manual API testing

---

## Project File Structure

```
app/
├── config/
│   └── config.go          # App config: port, processing delay duration
├── database/
│   └── store.go           # In-memory store: map + RWMutex + all CRUD methods
├── models/
│   └── payment.go         # Structs: PaymentRequest, PaymentResponse, IdempotencyRecord
├── middleware/
│   └── idempotency.go     # Core logic: checks key, handles cache-hit, conflict, in-flight
├── handlers/
│   └── payment.go         # HTTP handler: simulates processing, builds response, saves to store
├── main.go                # Entry point: creates store, wires routes, starts server
└── README.md              # Public documentation (NOT these instructions)
```

---

## The One Endpoint

```
POST /process-payment
```

### Request Headers
| Header            | Required | Description                          |
|-------------------|----------|--------------------------------------|
| Idempotency-Key   | YES      | A unique string (e.g. UUID) per transaction |
| Content-Type      | YES      | application/json                     |

### Request Body
```json
{
  "amount": 100,
  "currency": "GHS"
}
```

### Response Scenarios

| Scenario                        | Status | Extra Header         | Body                                          |
|---------------------------------|--------|----------------------|-----------------------------------------------|
| New request (first time)        | 201    | —                    | `{"message": "Charged 100 GHS", "status": "success"}` |
| Duplicate (same key, same body) | 200    | X-Cache-Hit: true    | Same body as original response                |
| Same key, DIFFERENT body        | 409    | —                    | `{"error": "Idempotency key already used for a different request body."}` |
| Missing Idempotency-Key header  | 400    | —                    | `{"error": "Idempotency-Key header is required"}` |
| Same key arrives while first is still processing | 200 | X-Cache-Hit: true | Waits, then returns result of first request |

---

## Key Data Structures (models/payment.go)

```go
// What the client sends
type PaymentRequest struct {
    Amount   float64 `json:"amount"`
    Currency string  `json:"currency"`
}

// What we send back to the client
type PaymentResponse struct {
    Message string `json:"message"`
    Status  string `json:"status"`
}

// What we store internally in the map
type IdempotencyRecord struct {
    RequestHash  string          // SHA-256 hash of the request body
    Response     PaymentResponse // The saved response
    StatusCode   int             // The saved HTTP status code
    IsProcessing bool            // TRUE while first request is still running (in-flight flag)
    CreatedAt    time.Time       // For TTL expiry (Developer's Choice feature)
    Done         chan struct{}    // Closed when processing finishes — lets waiting requests unblock
}
```

---

## The Store (database/store.go)

The store is a simple struct:

```go
type Store struct {
    mu      sync.RWMutex
    records map[string]*IdempotencyRecord
}
```

### Methods the store must have:
- `Get(key string) (*IdempotencyRecord, bool)` — fetch a record
- `Set(key string, record *IdempotencyRecord)` — save a record
- `IsExpired(record *IdempotencyRecord) bool` — check if TTL has passed (24 hours)

### Why RWMutex (not just Mutex)?
- `RLock()` / `RUnlock()` — for reading (multiple goroutines can read at the same time)
- `Lock()` / `Unlock()` — for writing (only one goroutine can write at a time)
- This is more performant than a regular `sync.Mutex` for a read-heavy store

---

## The Middleware (middleware/idempotency.go)

This is the **heart of the project**. It runs BEFORE the handler on every request.

### Decision flow the middleware must implement:

```
1. Read Idempotency-Key header
   └── Missing? → return 400 immediately

2. Hash the request body (SHA-256)
   └── This lets us compare "same body?" without storing raw JSON

3. Lock the store and look up the key
   └── NOT FOUND?
       ├── Create a new record with IsProcessing = true
       ├── Save it to the store
       ├── Unlock the store
       └── Call next handler (let the payment process)

   └── FOUND and IsProcessing = true (IN-FLIGHT)?
       ├── Unlock the store
       ├── Wait on record.Done channel (blocks until first request finishes)
       └── Return the saved response with X-Cache-Hit: true

   └── FOUND and IsProcessing = false?
       ├── Does RequestHash match? NO → return 409 Conflict
       └── YES → return saved response with X-Cache-Hit: true
```

### Why hash the body instead of storing raw JSON?
- Faster comparison (fixed-size string vs potentially large JSON)
- Cleaner code — `record.RequestHash != incomingHash` is simple

---

## The Handler (handlers/payment.go)

The handler only runs for NEW requests (middleware already handled duplicates).

```
1. Decode the JSON body into PaymentRequest struct
2. Simulate processing: time.Sleep(2 * time.Second)
3. Build the response: "Charged {amount} {currency}"
4. Save the response + status code into the store record
5. Close the record.Done channel (unblocks any waiting in-flight requests)
6. Set record.IsProcessing = false
7. Return 201 Created
```

---

## The Config (config/config.go)

```go
type Config struct {
    Port              string        // default: "8080"
    ProcessingDelay   time.Duration // default: 2 * time.Second
    IdempotencyTTL    time.Duration // default: 24 * time.Hour
}
```

Load from environment variables with fallback defaults. This is a good engineering
practice — no hardcoded values in business logic.

---

## Developer's Choice Feature: Key Expiry (TTL)

**What it does**: After 24 hours, an idempotency key expires and can be reused.

**Why it matters for Fintech**: Without TTL, the in-memory store grows forever. Also,
retries for a payment from last week are not the same as retries from 5 seconds ago.
This mirrors exactly what Stripe does in their production API.

**How to implement it**:
- Store `CreatedAt time.Time` in `IdempotencyRecord`
- In the middleware, when a key is FOUND, check `time.Since(record.CreatedAt) > 24h`
- If expired — treat it as a NEW request (delete old record, process fresh)

---

## Race Condition Handling (Bonus Story)

This is the most technically impressive part. Two identical requests arrive at the
same moment.

**The problem**: Without protection, both see "key not found", both create a record,
and both process the payment — double charge again.

**The solution** — a `chan struct{}` called `Done` on each record:

```go
// In the store — when creating a NEW record:
record := &IdempotencyRecord{
    IsProcessing: true,
    Done:         make(chan struct{}),  // open channel = "not done yet"
}

// In the handler — when processing finishes:
close(record.Done)  // closing a channel broadcasts to ALL waiting goroutines

// In the middleware — when a second request arrives and sees IsProcessing = true:
<-record.Done  // BLOCKS here until the channel is closed
// Now record has the result — return it
```

**Why `close()` instead of `record.Done <- struct{}{}`?**
Sending to a channel only unblocks ONE waiting goroutine. Closing the channel
unblocks ALL of them simultaneously. If 10 requests are waiting, all 10 get
the result at once.

---

## How to Build This Step by Step

### Phase 1 — Get something running (Day 1)
1. `go mod init github.com/yourname/idempotency-gateway`
2. Write `models/payment.go` — just the structs, nothing else
3. Write `config/config.go` — the Config struct with defaults
4. Write a basic `handlers/payment.go` — just the sleep + response, no store yet
5. Write `main.go` — one route, start server
6. Test in Postman: `POST /process-payment` works and returns 201 after 2 seconds ✓

### Phase 2 — Add the store (Day 1)
7. Write `database/store.go` — map, RWMutex, Get/Set methods
8. Update `handlers/payment.go` — save the response to the store after processing
9. Test in Postman: make a request, check you can read back the stored record ✓

### Phase 3 — Add the middleware (Day 1-2)
10. Write `middleware/idempotency.go` — the full decision flow above
11. Wire the middleware in `main.go`
12. Test User Story 1 in Postman: first request → 201 ✓
13. Test User Story 2 in Postman: duplicate request → 200 + X-Cache-Hit: true ✓
14. Test User Story 3 in Postman: same key, different body → 409 ✓

### Phase 4 — Bonus + Developer's Choice (Day 2)
15. Add `Done chan struct{}` to IdempotencyRecord
16. Add in-flight handling to middleware
17. Test bonus: two simultaneous requests in Postman (or write a quick test)
18. Add TTL expiry check to middleware

### Phase 5 — Documentation (Day 2)
19. Write README.md
20. Add architecture diagram

---

## Interview Questions You Will Be Asked

### "Why did you use an in-memory store instead of a database?"
> For this scale and use case, an in-memory map with mutex is sufficient and has O(1)
> lookup. In production, I'd use Redis — it has native TTL support, is shared across
> multiple server instances (horizontal scaling), and is purpose-built for this pattern.
> The trade-off is that in-memory data is lost on restart, which is acceptable for
> idempotency keys since they're short-lived anyway.

### "What is a race condition and how did you solve it?"
> A race condition is when two concurrent operations read shared state, both see the
> same "empty" state, and both proceed to write — causing a conflict. I solved it with
> two mechanisms: (1) a sync.RWMutex around all store reads/writes so only one goroutine
> can write at a time, and (2) a `Done` channel on each in-flight record — any second
> request with the same key blocks on `<-record.Done` until the first request closes
> the channel, then reads the result.

### "Why hash the request body?"
> Instead of storing and comparing potentially large JSON payloads, I compute a
> SHA-256 hash of the raw request body once and store that fixed 64-character string.
> Comparison is then O(1) and memory usage is predictable regardless of payload size.

### "What happens if the server restarts?"
> All in-memory idempotency keys are lost. This means a retry after restart would
> be processed as a new payment. In production, you'd use Redis with AOF persistence
> so the store survives restarts. This is a known and acceptable trade-off for the
> interview scope.

### "What is idempotency and why does it matter?"
> Idempotency means that making the same request N times has the same effect as making
> it once. It matters in payment systems because networks are unreliable — clients
> retry timed-out requests, and without idempotency this causes double charges.
> HTTP GET is naturally idempotent. POST is not — which is why we need this layer.

### "Walk me through your middleware logic."
> Point to the middleware decision flow in this file and walk through it step by step.

---

## How to Ask the AI for Help

When you're stuck, use these prompt patterns:

**When you don't understand something:**
> "I'm building a Go idempotency gateway. I'm in [filename]. Explain what [concept]
> means in this context and show me an example."

**When you want code written:**
> "Write the [function name] function in [filename] for my idempotency gateway.
> Here is the struct it works with: [paste struct]. The function should [describe
> what it does from this CLAUDE.md]."

**When you have an error:**
> "I'm getting this error in [filename]: [paste error]. Here is my code: [paste code].
> What is wrong and how do I fix it?"

**When you want to understand your own code:**
> "Explain what this code does line by line as if I'm preparing for a technical
> interview: [paste code]"

---

## Postman Test Checklist

Set base URL to: `http://localhost:8080`

- [ ] **Story 1** — New request: `POST /process-payment`, header `Idempotency-Key: key-001`, body `{"amount":100,"currency":"GHS"}` → expect 201, 2 second delay
- [ ] **Story 2** — Duplicate: Same request again → expect 200, instant response, `X-Cache-Hit: true` header
- [ ] **Story 3** — Conflict: Same key `key-001`, but body `{"amount":500,"currency":"GHS"}` → expect 409
- [ ] **Error** — Missing key: Remove `Idempotency-Key` header → expect 400
- [ ] **Bonus** — Use Postman's "Send and Download" or Collection Runner to fire two requests simultaneously
- [ ] **TTL** — Set TTL to 5 seconds in config, wait, retry → expect 201 (fresh process)
