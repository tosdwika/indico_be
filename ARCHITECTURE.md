# Architecture — High-Throughput Inventory Reservation System

## 1. Architectural Design & Synchronization

**State:** single-process, in-memory, sharded by item. Two lock layers:
- A global `sync.Mutex` guards only the map structures (insert/lookup of stocks, reservations, per-item locks).
- A per-item `sync.Mutex` guards all stock mutations and reservation status transitions for that item.

The critical section (`CreateReservation`: check `available >= qty` → increment `reserved`) runs entirely under the per-item lock, so the check-and-hold is atomic — oversell is impossible by construction, not by retry. Confirm/double-confirm races are handled the same way: status transition + stock decrement happen atomically under the item lock, so 50 concurrent confirms yield exactly one success (see `TestStressConcurrentConfirmIdempotencyency`).

**Expiry:** two-tiered. A background reaper sweeps every 10s and releases expired reservations; `Confirm` also performs lazy expiry — an expired reservation is released on touch, then rejected. Stock therefore never leaks as long as either path runs.

**Error/response format:** a single envelope `{"error": {"code": "...", "message": "..."}}` with machine-readable codes (`INSUFFICIENT_STOCK`, `RESERVATION_EXPIRED`, ...) mapped to correct HTTP statuses (409, 410, 404, 400). Clients branch on `code`, humans read `message`.

## 2. Distributed Scaling & Failure Modes

**What breaks at 10 instances:** everything, because state is per-process memory. Each instance has its own stock map, so reservations confirm on a different node fail as `RESERVATION_NOT_FOUND`, and available-stock checks diverge — ten instances each believe they own all 100 units and oversell 10×.

**Stateless redesign:** move the stock row to Postgres and make the hot path a single atomic statement:

```sql
UPDATE stocks SET reserved = reserved + $qty
WHERE item_id = $1 AND total - reserved >= $qty;
```

Zero rows affected → insufficient stock. Reservation confirm uses the same pattern guarded by a status transition (`WHERE reservation_id = $id AND status = 'active' AND expires_at > now()`). The reaper becomes a single leader-elected job (or a `WHERE expires_at < now()` UPDATE run by any node — idempotent). The per-item mutex maps cleanly to row-level locks; hot items are naturally serialized by the database, and the app tier becomes stateless and horizontally scalable.

## 3. Engineering Trade-offs & AI Transparency

**Trade-offs in the 4–8h window:** in-memory store (no persistence — restart loses state) bought atomicity and zero infra for the concurrency-critical path. A TTL reaper (O(n) sweep every 10s) instead of an expiry heap: simple, and at flash-sale reservation volumes the map stays small enough that a scan is microseconds. Frontend uses 3s polling instead of WebSockets: one `setInterval`, no server changes, and live-enough for a mini dashboard.

**AI scenario where a suggestion was flawed:** an AI assistant proposed using a single `sync.RWMutex` around the whole inventory map "for simplicity." Under flash-sale load this serializes *all* items behind one lock — item A's buyers block item B's — turning the service into a concurrency of 1. The flaw was invisible in toy tests (single item, low contention) and only appears under multi-item stress. The fix was the per-item sharded lock design described above, validated by `go test -race` stress tests.
