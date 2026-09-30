# Indico Backend

API reservasi stok untuk skenario flash sale. Backend menangani pemeriksaan stok, pembuatan reservasi, konfirmasi pembelian, dan pengembalian stok dari reservasi yang kedaluwarsa.

Data disimpan di SQLite agar stok dan reservasi tetap tersedia setelah aplikasi atau container dimulai ulang.

- **API production:** https://indico_engine.dwika.tech
- **Frontend:** https://indico.dwika.tech
- **Dokumentasi arsitektur:** [ARCHITECTURE.md](ARCHITECTURE.md)

## Fitur Utama

- Stok dan reservasi tersimpan secara persisten di SQLite.
- Halaman `https://indico.dwika.tech/reset` dapat mengembalikan stok item ke jumlah tertentu dan membatalkan reservasi aktifnya.
- Reservasi berlaku selama 5 menit.
- Transaksi database mencegah perubahan stok yang tidak lengkap.
- Query bersyarat mencegah stok direservasi melebihi jumlah yang tersedia.
- Reservasi kedaluwarsa otomatis dikembalikan ke stok tersedia.
- Konfirmasi ganda pada reservasi yang sama ditolak.
- Server mendukung graceful shutdown.
- Format error konsisten dan mudah digunakan oleh frontend.

## Alur Reservasi

1. Client mengirim item dan jumlah yang ingin dipesan.
2. Backend meminta SQLite menambah jumlah stok yang sedang direservasi, tetapi hanya jika stok masih mencukupi.
3. Jika berhasil, backend menyimpan reservasi aktif dengan masa berlaku 5 menit.
4. Client mengonfirmasi reservasi sebelum batas waktunya habis.
5. Saat dikonfirmasi, stok total dikurangi dan status reservasi berubah menjadi `confirmed` dalam satu transaksi.
6. Reservasi yang tidak dikonfirmasi akan berubah menjadi `expired`, lalu stoknya kembali tersedia.

## Persyaratan

- Go 1.25 atau lebih baru
- Docker (opsional)

SQLite berjalan langsung di dalam aplikasi melalui driver Go. Tidak perlu memasang server database terpisah.

## Menjalankan Secara Lokal

```bash
go run ./cmd/server
```

Server berjalan di `http://localhost:8085`. Database otomatis dibuat di `data/indico.db`.

Untuk memakai lokasi database lain:

```bash
DATABASE_PATH=/var/lib/indico/indico.db go run ./cmd/server
```

Saat database pertama kali dibuat, backend menambahkan `item_4021` dengan stok 100. Stok awal dapat ditentukan melalui `SEED_ITEMS`:

```bash
SEED_ITEMS="item_4021:100,item_9001:50" go run ./cmd/server
```

Seed memakai `INSERT OR IGNORE`. Artinya, data yang sudah ada tidak ditimpa ketika aplikasi dimulai ulang.

## Menjalankan dengan Docker

Gunakan volume agar file SQLite tetap tersimpan ketika container diganti:

```bash
docker build -t indico_engine .
docker volume create indico_data

docker run --rm \
  -p 8085:8085 \
  -e DATABASE_PATH=/app/data/indico.db \
  -v indico_data:/app/data \
  --name indico_engine \
  indico_engine
```

Tanpa volume, database akan ikut hilang saat container dihapus.

## Endpoint API

### Melihat stok

```http
GET /api/v1/inventory/stock?item_id=item_4021
```

Contoh respons:

```json
{
  "item_id": "item_4021",
  "total_stock": 100,
  "reserved_stock": 0,
  "available_stock": 100
}
```

### Membuat reservasi

```http
POST /api/v1/inventory/reserve
Content-Type: application/json
```

```json
{
  "user_id": "usr_9981",
  "item_id": "item_4021",
  "quantity": 2
}
```

Contoh respons:

```json
{
  "status": "success",
  "reservation_id": "res_883291",
  "item_id": "item_4021",
  "quantity": 2,
  "expires_at": "2026-09-30T16:35:00Z"
}
```

### Mengonfirmasi reservasi

```http
POST /api/v1/inventory/confirm
Content-Type: application/json
```

```json
{
  "reservation_id": "res_883291"
}
```

Contoh respons:

```json
{
  "status": "success",
  "reservation_id": "res_883291",
  "confirmed_at": "2026-09-30T16:32:00Z"
}
```

### Mereset stok

```http
POST /api/v1/inventory/reset
Content-Type: application/json
```

```json
{
  "item_id": "item_4021",
  "total_stock": 100
}
```

Reset mengubah stok item ke jumlah baru, mengosongkan stok yang sedang direservasi, dan membatalkan seluruh reservasi aktif untuk item tersebut. Endpoint ini digunakan oleh halaman `https://indico.dwika.tech/reset`.

## Format Error

Semua error menggunakan struktur yang sama:

```json
{
  "error": {
    "code": "INSUFFICIENT_STOCK",
    "message": "not enough available stock"
  }
}
```

| Status | Kode | Keterangan |
|---|---|---|
| 400 | `INVALID_INPUT` | Request tidak lengkap atau quantity kurang dari 1 |
| 404 | `ITEM_NOT_FOUND` | Item tidak ditemukan |
| 404 | `RESERVATION_NOT_FOUND` | Reservasi tidak ditemukan |
| 409 | `INSUFFICIENT_STOCK` | Stok tersedia tidak mencukupi |
| 409 | `ALREADY_CONFIRMED` | Reservasi sudah pernah dikonfirmasi |
| 410 | `RESERVATION_EXPIRED` | Reservasi sudah kedaluwarsa |

## Contoh Penggunaan dengan curl

```bash
# Lihat stok
curl "http://localhost:8085/api/v1/inventory/stock?item_id=item_4021"

# Reservasi 2 unit
curl -X POST http://localhost:8085/api/v1/inventory/reserve \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"usr_9981","item_id":"item_4021","quantity":2}'

# Gunakan reservation_id dari respons sebelumnya
curl -X POST http://localhost:8085/api/v1/inventory/confirm \
  -H 'Content-Type: application/json' \
  -d '{"reservation_id":"res_883291"}'
```

Ganti `http://localhost:8085` dengan `https://indico_engine.dwika.tech` untuk mencoba API production.

## Pengujian

Jalankan seluruh unit test dan stress test dengan race detector:

```bash
go test -race -v ./...
```

Setiap test menggunakan database SQLite sementara di memori, sehingga tidak mengubah database development atau production. Pengujian mencakup:

- reservasi dengan stok cukup dan tidak cukup;
- item dan reservasi yang tidak ditemukan;
- alur reservasi hingga konfirmasi;
- reservasi serentak tanpa overselling; dan
- konfirmasi serentak yang hanya boleh berhasil satu kali.
