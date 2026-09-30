# Indico Backend

API reservasi stok untuk skenario flash sale. Layanan ini menangani pengecekan stok, pembuatan reservasi, konfirmasi pembelian, dan pengembalian stok dari reservasi yang kedaluwarsa.

- **API production:** https://indico_engine.dwika.tech
- **Frontend:** https://indico.dwika.tech
- **Dokumentasi arsitektur:** [ARCHITECTURE.md](ARCHITECTURE.md)

## Fitur Utama

- Reservasi stok berlaku selama 5 menit.
- Pengecekan dan reservasi stok dilakukan secara atomik untuk mencegah overselling.
- Setiap item memiliki lock sendiri, sehingga transaksi pada item yang berbeda tidak saling menghambat.
- Reservasi yang kedaluwarsa otomatis dikembalikan ke stok tersedia.
- Konfirmasi ganda pada reservasi yang sama ditolak.
- Server mendukung graceful shutdown agar request yang sedang diproses dapat diselesaikan dengan aman.
- Format error konsisten dan mudah digunakan oleh frontend.

## Alur Reservasi

1. Client meminta reservasi untuk suatu item dan jumlah tertentu.
2. Backend memeriksa stok yang tersedia.
3. Jika stok cukup, backend membuat reservasi aktif dengan masa berlaku 5 menit.
4. Client mengonfirmasi reservasi sebelum batas waktu berakhir.
5. Jika tidak dikonfirmasi, reservasi kedaluwarsa dan stok kembali tersedia.

## Persyaratan

- Go 1.25 atau lebih baru
- Docker (opsional)

## Menjalankan Secara Lokal

```bash
go run ./cmd/server
```

Server berjalan di `http://localhost:8085`.

Secara default tersedia item `item_4021` dengan stok 100. Untuk menentukan stok awal sendiri, gunakan environment variable `SEED_ITEMS`:

```bash
SEED_ITEMS="item_4021:100,item_9001:50" go run ./cmd/server
```

Format setiap item adalah `item_id:jumlah`, dipisahkan dengan koma.

## Menjalankan dengan Docker

```bash
docker build -t indico_engine .
docker run --rm -p 8085:8085 --name indico_engine indico_engine
```

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

# Konfirmasi reservasi; gunakan reservation_id dari respons sebelumnya
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

Pengujian mencakup:

- reservasi dengan stok cukup dan tidak cukup;
- item dan reservasi yang tidak ditemukan;
- alur reservasi hingga konfirmasi;
- reservasi serentak tanpa overselling; dan
- konfirmasi serentak yang hanya boleh berhasil satu kali.
