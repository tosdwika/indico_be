# indico_be — Inventory Reservation API (Go)

Backend untuk sistem flash-sale: banyak orang merebut barang yang sama di waktu yang sama, dan sistem ini memastikan tidak ada yang terjual lebih banyak dari stok yang ada.

Live: **https://indico_engine.dwika.tech**

## Alur Sistem

Sederhananya seperti memesan tiket:

1. **Pesan** — user memesan sejumlah unit. Stoknya langsung "disimpan" untuknya, tapi hanya ditahan selama **5 menit**.
2. **Bayar** — kalau user menyelesaikan pesanan sebelum 5 menit, stok benar-benar berkurang. Pesanan yang sudah selesai tidak bisa diselesaikan dua kali.
3. **Telat** — kalau lewat dari 5 menit, tahanannya dilepas otomatis dan stoknya kembali bisa dipesan orang lain.

Bagian yang paling dijaga ketat: pengecekan stok dan pengurangannya digabung jadi satu langkah yang tidak bisa disela — jadi dua orang yang memesan unit terakhir bersamaan, hanya satu yang berhasil. Sistemnya tidak mungkin menjual melebihi stok. Penjelasan lengkap ada di [ARCHITECTURE.md](ARCHITECTURE.md).

## Menjalankan

```bash
go run ./cmd/server          # jalan di port 8085
```

Stok awal bisa diatur sendiri, misalnya:

```bash
SEED_ITEMS="item_4021:100,item_9001:50" go run ./cmd/server
```

Tanpa diisi, default-nya `item_4021` dengan stok 100.

## Tes

```bash
go test -race -v ./...
```

Semua tes lolos, termasuk dua skenario ekstrem: ratusan pesanan bersamaan tidak boleh melebihi stok, dan pesanan yang dikonfirmasi bersamaan hanya boleh berhasil sekali.

## Docker

```bash
docker build -t indico_engine .
docker run -p 8085:8085 indico_engine
```

## API

Tiga endpoint, semuanya JSON:

| Method | Endpoint | Fungsi |
|---|---|---|
| `POST` | `/api/v1/inventory/reserve` | Pesan stok. Kirim `{"user_id", "item_id", "quantity"}` |
| `POST` | `/api/v1/inventory/confirm` | Selesaikan pesanan. Kirim `{"reservation_id"}` |
| `GET` | `/api/v1/inventory/stock?item_id=…` | Lihat stok saat ini |

Kalau ada yang salah, responsnya selalu berbentuk `{"error":{"code","message"}}` dengan arti yang jelas:

| Kode | Kapan terjadi |
|---|---|
| `INVALID_INPUT` (400) | ada field yang kosong, atau quantity di bawah 1 |
| `ITEM_NOT_FOUND` (404) | barangnya tidak ada |
| `INSUFFICIENT_STOCK` (409) | stoknya tidak cukup |
| `RESERVATION_NOT_FOUND` (404) | ID pesanan tidak dikenal |
| `RESERVATION_EXPIRED` (410) | sudah lewat 5 menit, stoknya sudah dikembalikan |
| `ALREADY_CONFIRMED` (409) | pesanan ini sudah selesai sebelumnya |

## Contoh Pemakaian

Coba langsung dari terminal:

```bash
# Lihat stok dulu
curl "https://indico_engine.dwika.tech/api/v1/inventory/stock?item_id=item_4021"
# → {"total_stock":100,"reserved_stock":0,"available_stock":100}

# Pesan 2 unit — catat reservation_id dari hasilnya
curl -X POST https://indico_engine.dwika.tech/api/v1/inventory/reserve \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"usr_9981","item_id":"item_4021","quantity":2}'
# → {"status":"success","reservation_id":"res_883291","expires_at":"..."}

# Selesaikan pesanan (sebelum 5 menit habis)
curl -X POST https://indico_engine.dwika.tech/api/v1/inventory/confirm \
  -H 'Content-Type: application/json' \
  -d '{"reservation_id":"res_883291"}'
# → {"status":"success","confirmed_at":"..."}
```

Tampilannya (dashboard) ada di repo [indico_fe](../indico_fe).
