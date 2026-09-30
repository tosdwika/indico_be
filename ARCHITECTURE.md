# Arsitektur Sistem Reservasi Stok

Dokumen ini menjelaskan cara backend menyimpan data di SQLite, menjaga stok tetap akurat ketika banyak request datang bersamaan, dan batasan yang perlu dipahami sebelum sistem dikembangkan lebih lanjut.

## 1. Gambaran Umum

Backend terdiri dari tiga bagian utama:

1. **Controller** menerima request HTTP, memeriksa input, dan menyusun respons JSON.
2. **Service** mengatur alur reservasi, masa berlaku reservasi, dan konfirmasi pembelian.
3. **Repository** menjalankan query dan transaksi ke database SQLite.

SQLite menyimpan data di satu file, yaitu `data/indico.db` secara default. File ini berisi tabel stok dan reservasi. Karena data tidak lagi hanya berada di memori, restart aplikasi tidak menghapus stok maupun riwayat reservasi.

## 2. Struktur Database

### Tabel `stocks`

Tabel ini menyimpan stok setiap item:

- `item_id`: identitas unik item;
- `total_stock`: jumlah stok fisik yang belum terjual; dan
- `reserved_qty`: jumlah stok yang sedang ditahan oleh reservasi aktif.

Stok yang bisa dipesan dihitung dengan rumus:

```text
available_stock = total_stock - reserved_qty
```

Database memiliki constraint agar nilai stok tidak negatif dan jumlah reservasi tidak melebihi stok total.

### Tabel `reservations`

Tabel ini menyimpan:

- ID reservasi dan pengguna;
- item serta jumlah yang dipesan;
- waktu pembuatan dan kedaluwarsa; dan
- status `active`, `confirmed`, atau `expired`.

Index pada kolom status dan waktu kedaluwarsa membantu proses pembersihan mencari reservasi aktif yang sudah melewati batas waktu.

## 3. Cara Sistem Mencegah Overselling

Saat membuat reservasi, repository menjalankan query bersyarat:

```sql
UPDATE stocks
SET reserved_qty = reserved_qty + ?
WHERE item_id = ?
  AND total_stock - reserved_qty >= ?;
```

SQLite mengunci proses penulisan selama query dijalankan. Nilai `reserved_qty` hanya bertambah jika stok masih cukup. Jika dua request mencoba mengambil unit terakhir pada saat yang sama, query pertama akan berhasil dan query berikutnya akan melihat nilai stok terbaru lalu ditolak.

Pembaruan stok dan penyimpanan reservasi dilakukan dalam satu transaksi. Jika penyimpanan reservasi gagal, perubahan stok ikut dibatalkan. Dengan demikian, tidak ada stok yang tertahan tanpa data reservasi.

## 4. Konfirmasi dan Kedaluwarsa

### Konfirmasi reservasi

Konfirmasi hanya dapat mengubah reservasi yang masih `active` dan belum kedaluwarsa. Dalam satu transaksi, backend:

1. mengubah status reservasi menjadi `confirmed`;
2. mengurangi `total_stock`; dan
3. mengurangi `reserved_qty`.

Perubahan status menggunakan syarat di query, sehingga konfirmasi kedua untuk ID yang sama tidak dapat berhasil.

### Reservasi kedaluwarsa

Reservasi aktif berlaku selama 5 menit. Backend memeriksa reservasi kedaluwarsa melalui dua jalur:

- proses latar belakang berjalan setiap 10 detik; dan
- endpoint konfirmasi memeriksa waktu kedaluwarsa sebelum melanjutkan.

Saat reservasi kedaluwarsa, status berubah menjadi `expired` dan `reserved_qty` dikurangi dalam satu transaksi. Query hanya menerima reservasi yang masih aktif, sehingga proses ini aman meskipun pemeriksaan dijalankan lebih dari sekali.

### Reset stok

Reset dijalankan dalam satu transaksi. Backend mengubah stok item ke jumlah baru, mengosongkan `reserved_qty`, lalu mengubah seluruh reservasi aktif pada item tersebut menjadi `expired`.

Halaman `https://indico.dwika.tech/reset` menggunakan endpoint `POST /api/v1/inventory/reset`. Endpoint mewajibkan bearer token yang dibandingkan dengan environment variable `RESET_TOKEN`. Reservasi lama tidak dapat dikonfirmasi lagi setelah reset dilakukan.

## 5. Penyimpanan File SQLite

Lokasi database diatur melalui environment variable `DATABASE_PATH`. Nilai defaultnya:

```text
data/indico.db
```

Saat menggunakan Docker, direktori `/app/data` harus dipasang sebagai volume:

```bash
docker run \
  -e DATABASE_PATH=/app/data/indico.db \
  -v indico_data:/app/data \
  indico_engine
```

Deployment production memasang direktori host `/data/indico_engine` ke `/app/data`. Container dapat diganti saat deployment tanpa menghapus database.

SQLite menggunakan mode WAL dan `busy_timeout` 5 detik. WAL membantu proses baca tetap berjalan ketika ada transaksi tulis, sedangkan `busy_timeout` memberi waktu bagi request untuk menunggu transaksi tulis sebelumnya selesai. Backend memakai satu koneksi database agar satu transaksi reservasi selesai sebelum transaksi tulis berikutnya dimulai; ini sesuai dengan model SQLite yang hanya memiliki satu penulis pada satu waktu.

## 6. Batasan SQLite

SQLite sesuai untuk aplikasi ini karena deployment backend hanya memakai satu container, tidak memerlukan server database terpisah, dan tetap menyediakan penyimpanan persisten serta transaksi.

Namun, SQLite hanya mengizinkan satu proses penulisan pada satu waktu. Sistem tetap aman ketika banyak request datang bersamaan, tetapi transaksi tulis akan diproses secara berurutan. Hal ini dapat menjadi hambatan jika trafik tulis meningkat sangat tinggi.

File SQLite juga tidak boleh dibagikan langsung ke beberapa server melalui network filesystem. Jika backend perlu dijalankan pada banyak instance, database perlu dipindahkan ke PostgreSQL atau layanan database lain yang memang dirancang untuk akses dari banyak mesin. Struktur service dan endpoint tidak perlu diubah; bagian repository yang menangani query database yang perlu diganti.

## 7. Pertimbangan Teknis

SQLite dipilih untuk memberikan penyimpanan data nyata tanpa menambah proses database terpisah. Dibandingkan penyimpanan in-memory, pendekatan ini memberi beberapa keuntungan:

- data tetap ada setelah restart;
- transaksi dan constraint dijaga oleh database;
- isi database dapat dicadangkan sebagai satu file; dan
- pengoperasian lokal maupun production tetap sederhana.

Konsekuensinya, kemampuan menulis data tidak dapat bertambah hanya dengan menambah instance backend. Batas ini masih sesuai untuk dashboard dan ruang lingkup aplikasi saat ini.

Frontend tetap mengambil data stok setiap 3 detik. WebSocket belum diperlukan karena polling sudah cukup untuk jumlah pengguna dan frekuensi perubahan saat ini.

## 8. Pengujian

Test menggunakan SQLite `:memory:` agar setiap test memiliki database bersih dan tidak meninggalkan file. Pemeriksaan yang dijalankan meliputi:

- alur reservasi dan konfirmasi;
- penolakan saat stok tidak cukup;
- item atau reservasi yang tidak ditemukan;
- 500 request yang bersaing untuk 100 stok; dan
- 50 konfirmasi bersamaan untuk satu reservasi.

Seluruh test dapat dijalankan dengan race detector:

```bash
go test -race -v ./...
```

Pengujian konkurensi tetap diperlukan walaupun SQLite menangani transaksi, karena kode Go di sekitar transaksi juga harus memastikan status reservasi dan respons API tetap konsisten.

## 9. Penggunaan AI dan Evaluasi Hasilnya

AI digunakan sebagai alat bantu selama pengembangan, tetapi saran yang diberikan tetap diperiksa terhadap kebutuhan sistem dan hasil pengujian.

Salah satu rancangan awal menyimpan data di memori dengan lock terpisah untuk setiap item. Rancangan tersebut cepat dan aman untuk satu proses, tetapi seluruh data hilang saat aplikasi dimulai ulang. Setelah kebutuhan penyimpanan persisten ditetapkan, repository dipindahkan ke SQLite dan perlindungan konkurensi diserahkan kepada transaksi, query bersyarat, serta constraint database.

Perubahan ini tidak hanya mengganti tempat penyimpanan. Alur reservasi dan konfirmasi juga diuji ulang untuk memastikan transaksi tetap mencegah overselling dan konfirmasi ganda.
