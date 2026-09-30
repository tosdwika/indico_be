# Arsitektur Sistem Reservasi Stok

Dokumen ini menjelaskan cara sistem menjaga stok tetap akurat saat banyak pengguna melakukan reservasi secara bersamaan, keterbatasan arsitektur saat ini, dan perubahan yang diperlukan jika aplikasi dijalankan pada banyak server.

## 1. Desain Arsitektur dan Pengelolaan Akses Bersamaan

### Penyimpanan data

Data stok dan reservasi saat ini disimpan di memori aplikasi. Pendekatan ini dipilih karena sederhana, cepat, dan cukup untuk ruang lingkup tugas ini. Konsekuensinya, seluruh data akan hilang ketika aplikasi dihentikan atau dimulai ulang.

Repository menggunakan dua jenis lock:

1. **Lock global (`sync.Mutex`)** hanya digunakan untuk mengakses map yang menyimpan stok, reservasi, dan daftar lock per item.
2. **Lock per item** digunakan ketika stok atau status reservasi untuk item tersebut berubah.

Lock per item memungkinkan transaksi untuk barang yang berbeda diproses secara bersamaan. Sebagai contoh, reservasi untuk `item_4021` tidak perlu menunggu reservasi untuk `item_9001` selesai.

### Pencegahan overselling

Proses reservasi melakukan dua langkah penting dalam satu bagian yang dilindungi lock:

1. memeriksa apakah stok tersedia mencukupi; dan
2. menambahkan jumlah yang dipesan ke stok yang sedang direservasi.

Karena kedua langkah dilakukan tanpa melepas lock, proses lain tidak dapat mengubah stok di antara tahap pemeriksaan dan reservasi. Jika dua pengguna mencoba mengambil unit terakhir pada saat yang sama, hanya satu permintaan yang akan berhasil.

Konfirmasi reservasi menggunakan prinsip yang sama. Pemeriksaan status, pengurangan stok, dan perubahan status menjadi `confirmed` dilakukan dalam satu lock. Dengan demikian, reservasi yang sama tidak dapat dikonfirmasi dua kali meskipun beberapa request datang bersamaan.

Perilaku ini diuji melalui `TestStressNoOversell` dan `TestStressConcurrentConfirmIdempotency` dengan menjalankan banyak operasi secara paralel menggunakan race detector Go.

### Reservasi kedaluwarsa

Setiap reservasi aktif berlaku selama 5 menit. Sistem mengembalikan stok dari reservasi yang kedaluwarsa melalui dua jalur:

- proses latar belakang memeriksa seluruh reservasi setiap 10 detik; dan
- endpoint konfirmasi kembali memeriksa waktu kedaluwarsa sebelum memproses transaksi.

Pemeriksaan kedua diperlukan agar reservasi yang baru saja kedaluwarsa tetap ditolak dengan benar, meskipun proses latar belakang belum menjalankan pemeriksaan berikutnya. Pengembalian stok dibuat idempotent, sehingga stok tidak akan bertambah dua kali jika kedua jalur memproses reservasi yang sama.

### Format respons dan error

Respons error menggunakan format yang konsisten:

```json
{
  "error": {
    "code": "INSUFFICIENT_STOCK",
    "message": "not enough available stock"
  }
}
```

`code` digunakan oleh frontend untuk membedakan jenis error, sedangkan `message` dapat langsung ditampilkan kepada pengguna. Setiap kondisi dipetakan ke status HTTP yang sesuai, misalnya:

- `400` untuk input yang tidak valid;
- `404` untuk item atau reservasi yang tidak ditemukan;
- `409` untuk stok tidak cukup atau reservasi yang sudah dikonfirmasi; dan
- `410` untuk reservasi yang sudah kedaluwarsa.

## 2. Skalabilitas dan Risiko Kegagalan

### Batasan arsitektur saat ini

Arsitektur in-memory ini aman selama hanya ada satu instance aplikasi. Jika aplikasi langsung dijalankan pada 10 instance, setiap instance akan memiliki salinan stok dan reservasinya sendiri.

Dampaknya:

- setiap instance dapat menganggap stok yang sama masih tersedia;
- total penjualan dapat melebihi stok sebenarnya;
- reservasi yang dibuat di satu instance tidak ditemukan ketika request konfirmasi masuk ke instance lain; dan
- data hilang ketika container dimulai ulang.

Karena itu, menambahkan load balancer dan memperbanyak instance tanpa memindahkan state bukanlah solusi yang aman.

### Desain untuk banyak instance

Untuk deployment terdistribusi, data stok dan reservasi perlu dipindahkan ke penyimpanan bersama seperti PostgreSQL. Pemeriksaan dan penambahan stok reservasi dapat dilakukan dengan satu query atomik:

```sql
UPDATE stocks
SET reserved = reserved + $2
WHERE item_id = $1
  AND total - reserved >= $2;
```

Jika tidak ada baris yang berubah, berarti stok tidak mencukupi. Pendekatan ini membuat database menjadi sumber data utama dan mencegah beberapa instance menjual stok yang sama.

Konfirmasi juga harus menggunakan perubahan status bersyarat, misalnya hanya memperbarui reservasi yang masih `active` dan belum melewati `expires_at`. Transaksi database atau row-level lock digunakan agar perubahan status reservasi dan pengurangan stok terjadi sebagai satu kesatuan.

Pembersihan reservasi kedaluwarsa dapat dilakukan dengan query yang idempotent. Beberapa worker boleh menjalankannya bersamaan selama query hanya memperbarui reservasi yang masih aktif. Dengan desain tersebut, backend menjadi stateless dan dapat ditambah jumlah instancenya secara horizontal.

## 3. Pertimbangan Teknis dan Transparansi Penggunaan AI

### Keputusan dalam batas waktu pengerjaan

Beberapa keputusan dibuat agar solusi tetap dapat diselesaikan dan diuji dengan baik dalam waktu yang tersedia:

- **Penyimpanan in-memory** dipilih untuk menghindari kebutuhan infrastruktur database dan menjaga fokus pada kebenaran proses konkurensi. Kekurangannya, data tidak persisten dan aplikasi hanya aman untuk satu instance.
- **Pemeriksaan berkala setiap 10 detik** dipilih daripada struktur data khusus berdasarkan waktu kedaluwarsa. Implementasinya lebih sederhana, tetapi membutuhkan pemindaian seluruh reservasi. Jika jumlah reservasi menjadi sangat besar, pendekatan ini sebaiknya diganti dengan indeks berdasarkan waktu kedaluwarsa atau antrean terjadwal.
- **Polling frontend setiap 3 detik** dipilih daripada WebSocket. Untuk dashboard kecil, polling lebih mudah dioperasikan dan sudah cukup cepat. WebSocket baru diperlukan jika jumlah pengguna atau kebutuhan pembaruan real-time meningkat secara signifikan.

### Penggunaan AI dan evaluasi hasilnya

AI digunakan sebagai alat bantu selama pengembangan, tetapi setiap saran tetap diperiksa terhadap kebutuhan sistem dan diuji sebelum digunakan.

Salah satu saran awal adalah menggunakan satu `sync.RWMutex` untuk seluruh data inventory karena implementasinya lebih sederhana. Pendekatan tersebut memang aman dari data race, tetapi membuat semua item memakai antrean lock yang sama. Reservasi pada satu item akan menghambat reservasi pada item lain, sehingga throughput menurun saat trafik meningkat.

Saran tersebut tidak digunakan. Implementasi akhirnya memakai lock terpisah untuk setiap item, sementara lock global hanya menjaga struktur map. Hasilnya, perubahan pada item yang sama tetap aman dan berurutan, tetapi item yang berbeda masih dapat diproses secara paralel. Keputusan ini divalidasi menggunakan unit test, stress test, dan `go test -race -v ./...`.
