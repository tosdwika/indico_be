# indico_be — Inventory Reservation API (Go)

Layanan reservasi stok untuk flash-sale: ribuan user merebut barang yang sama tanpa oversell, dengan reservasi otomatis kedaluwarsa dalam 5 menit.

Live: **https://indico_engine.dwika.tech**

## Cara Kerja (singkat)

1. **Reserve** — user memesan N unit. Sistem mengunci stok untuknya selama **5 menit**.
2. **Confirm** — user membayar sebelum habis waktu → stok benar-benar berkurang. Konfirmasi ganda ditolak.
3. **Tidak konfirmasi** → stok otomatis dikembalikan setelah 5 menit, dan orang lain bisa memesan lagi.

Kuncinya semua di satu tempat: setiap operasi stok berjalan di dalam *lock per-item*, jadi cek-stok dan pengurangan-stok tidak bisa disisipi proses lain — oversell mustahil secara konstruksi. Detail desain: [ARCHITECTURE.md](ARCHITECTURE.md).

## Menjalankan

```bash
go run ./cmd/server          # jalan di :8085
```

Ganti data awal (opsional): `SEED_ITEMS="item_4021:100,item_9001:50" go run ./cmd/server` — default `item_4021:100`.

## Tes

```bash
go test -race -v ./...
```

Termasuk stress test: ratusan reserve bersamaan tidak boleh melebihi stok, dan konfirmasi paralel hanya boleh sukses satu kali.

## Docker

```bash
docker build -t indico_engine .
docker run -p 8085:8085 indico_engine
```

## API

| Method | Endpoint | Fungsi |
|---|---|---|
| `POST` | `/api/v1/inventory/reserve` | Pesan stok — body `{"user_id","item_id","quantity"}` |
| `POST` | `/api/v1/inventory/confirm` | Konfirmasi pesanan — body `{"reservation_id"}` |
| `GET` | `/api/v1/inventory/stock?item_id=…` | Lihat stok saat ini |

Error selalu berbentuk `{"error":{"code","message"}}`:

| Kode | Arti |
|---|---|
| `INVALID_INPUT` (400) | field kosong / quantity < 1 |
| `ITEM_NOT_FOUND` (404) | item tidak ada |
| `INSUFFICIENT_STOCK` (409) | stok kurang |
| `RESERVATION_NOT_FOUND` (404) | ID reservasi tidak dikenal |
| `RESERVATION_EXPIRED` (410) | kehabisan waktu 5 menit, stok sudah balik |
| `ALREADY_CONFIRMED` (409) | sudah dikonfirmasi sebelumnya |

## Contoh Pakai

```bash
# 1. Lihat stok
curl "https://indico_engine.dwika.tech/api/v1/inventory/stock?item_id=item_4021"
# → {"total_stock":100,"reserved_stock":0,"available_stock":100}

# 2. Pesan 2 unit (simpan reservation_id dari response)
curl -X POST https://indico_engine.dwika.tech/api/v1/inventory/reserve \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"usr_9981","item_id":"item_4021","quantity":2}'
# → {"status":"success","reservation_id":"res_883291","expires_at":...}

# 3. Konfirmasi sebelum 5 menit
curl -X POST https://indico_engine.dwika.tech/api/v1/inventory/confirm \
  -H 'Content-Type: application/json' \
  -d '{"reservation_id":"res_883291"}'
# → {"status":"success","confirmed_at":...}
```

Frontend-nya ada di [indico_fe](../indico_fe).
