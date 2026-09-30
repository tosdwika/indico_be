# indico_be — Inventory Reservation API (Go)

Flash-sale inventory reservation service: atomic reserve / confirm / stock endpoints with automatic 5-minute expiry, graceful shutdown, and race-tested concurrency.

Live: **https://indico_engine.dwika.tech**

## Run

```bash
go run ./cmd/server          # listens on :8085
```

Optional seed: `SEED_ITEMS="item_4021:100,item_9001:50" go run ./cmd/server` (default: `item_4021:100`).

## Test

```bash
go test -race -v ./...
```

## Docker

```bash
docker build -t indico_be .
docker run -p 8085:8085 indico_be
```

## Endpoints

| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/inventory/reserve` | Reserve stock. Body: `{"user_id","item_id","quantity"}`. Returns reservation + `expires_at` (5 min TTL). |
| POST | `/api/v1/inventory/confirm` | Commit a reservation. Body: `{"reservation_id"}`. |
| GET | `/api/v1/inventory/stock?item_id=...` | Real-time stock breakdown. |

Errors: `{"error":{"code":"...","message":"..."}}` — codes `INVALID_INPUT`, `ITEM_NOT_FOUND`, `INSUFFICIENT_STOCK` (409), `RESERVATION_NOT_FOUND` (404), `RESERVATION_EXPIRED` (410), `ALREADY_CONFIRMED` (409).

See [ARCHITECTURE.md](ARCHITECTURE.md) for design decisions.

## Penggunaan (curl walkthrough)

```bash
# 1. Cek stok — total/reserved/available
curl "https://indico_engine.dwika.tech/api/v1/inventory/stock?item_id=item_4021"
# {"item_id":"item_4021","total_stock":100,"reserved_stock":0,"available_stock":100}

# 2. Reservasi 2 unit untuk seorang user
curl -X POST https://indico_engine.dwika.tech/api/v1/inventory/reserve \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"usr_9981","item_id":"item_4021","quantity":2}'
# {"status":"success","reservation_id":"res_883291","item_id":"item_4021",
#  "quantity":2,"expires_at":"2026-09-30T16:35:00Z"}

# 3. Konfirmasi sebelum 5 menit — stok permanen berkurang
curl -X POST https://indico_engine.dwika.tech/api/v1/inventory/confirm \
  -H 'Content-Type: application/json' \
  -d '{"reservation_id":"res_883291"}'
# {"status":"success","reservation_id":"res_883291","confirmed_at":"2026-09-30T16:32:00Z"}

# 4. (Opsional) Cek stok lagi — total_stock turun 2, reserved kembali 0
curl "https://indico_engine.dwika.tech/api/v1/inventory/stock?item_id=item_4021"
```

Tidak dikonfirmasi dalam 5 menit → reservasi otomatis kedaluwarsa (reaper 10 detik + lazy expiry saat disentuh), stok dikembalikan, dan confirm berikutnya ditolak `RESERVATION_EXPIRED` (410).

Frontend dashboard: lihat [indico_fe](../indico_fe).
