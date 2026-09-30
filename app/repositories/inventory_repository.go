package repositories

import (
	"database/sql"
	"errors"
	"indico_be/app/models"
	"time"

	_ "modernc.org/sqlite"
)

type InventoryRepository struct {
	db *sql.DB
}

func NewInventoryRepository(path string) (*InventoryRepository, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, statement := range []string{
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS stocks (
			item_id TEXT PRIMARY KEY,
			total_stock INTEGER NOT NULL CHECK (total_stock >= 0),
			reserved_qty INTEGER NOT NULL DEFAULT 0 CHECK (reserved_qty >= 0 AND reserved_qty <= total_stock)
		)`,
		`CREATE TABLE IF NOT EXISTS reservations (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			item_id TEXT NOT NULL REFERENCES stocks(item_id),
			quantity INTEGER NOT NULL CHECK (quantity > 0),
			expires_at TEXT NOT NULL,
			status TEXT NOT NULL CHECK (status IN ('active', 'confirmed', 'expired')),
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS reservations_expiry_idx ON reservations(status, expires_at)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &InventoryRepository{db: db}, nil
}

func (r *InventoryRepository) Close() error { return r.db.Close() }

// Seed menambahkan stok awal jika item tersebut belum tersimpan.
func (r *InventoryRepository) Seed(itemID string, total int) error {
	_, err := r.db.Exec(`INSERT OR IGNORE INTO stocks(item_id, total_stock) VALUES (?, ?)`, itemID, total)
	return err
}

func (r *InventoryRepository) ResetStock(itemID string, total int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.Exec(`UPDATE stocks SET total_stock = ?, reserved_qty = 0 WHERE item_id = ?`, total, itemID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrItemNotFound
	}
	if _, err := tx.Exec(`UPDATE reservations SET status = 'expired' WHERE item_id = ? AND status = 'active'`, itemID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *InventoryRepository) GetStock(itemID string) (*models.Stock, bool) {
	s := &models.Stock{}
	err := r.db.QueryRow(`SELECT item_id, total_stock, reserved_qty FROM stocks WHERE item_id = ?`, itemID).
		Scan(&s.ItemID, &s.TotalStock, &s.ReservedQty)
	return s, err == nil
}

func (r *InventoryRepository) CreateReservation(res *models.Reservation) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.Exec(`UPDATE stocks SET reserved_qty = reserved_qty + ?
		WHERE item_id = ? AND total_stock - reserved_qty >= ?`, res.Quantity, res.ItemID, res.Quantity)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM stocks WHERE item_id = ?`, res.ItemID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return ErrItemNotFound
		}
		return ErrInsufficientStock
	}

	_, err = tx.Exec(`INSERT INTO reservations(id, user_id, item_id, quantity, expires_at, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, res.ID, res.UserID, res.ItemID, res.Quantity,
		formatTime(res.ExpiresAt), res.Status, formatTime(res.CreatedAt))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *InventoryRepository) GetReservation(id string) (*models.Reservation, bool) {
	res, err := scanReservation(r.db.QueryRow(`SELECT id, user_id, item_id, quantity, expires_at, status, created_at
		FROM reservations WHERE id = ?`, id))
	return res, err == nil
}

func (r *InventoryRepository) ConfirmReservation(id string, now time.Time) (*models.Reservation, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	res, err := scanReservation(tx.QueryRow(`UPDATE reservations SET status = 'confirmed'
		WHERE id = ? AND status = 'active' AND expires_at >= ?
		RETURNING id, user_id, item_id, quantity, expires_at, status, created_at`, id, formatTime(now)))
	if errors.Is(err, sql.ErrNoRows) {
		current, lookupErr := scanReservation(tx.QueryRow(`SELECT id, user_id, item_id, quantity, expires_at, status, created_at
			FROM reservations WHERE id = ?`, id))
		if errors.Is(lookupErr, sql.ErrNoRows) {
			return nil, ErrReservationNotFound
		}
		if lookupErr != nil {
			return nil, lookupErr
		}
		if current.Status == "expired" {
			return nil, ErrReservationNotFound
		}
		if current.Status == "confirmed" {
			return nil, ErrAlreadyConfirmed
		}
		return nil, ErrReservationExpired
	}
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(`UPDATE stocks SET total_stock = total_stock - ?, reserved_qty = reserved_qty - ?
		WHERE item_id = ?`, res.Quantity, res.Quantity, res.ItemID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return res, nil
}

func (r *InventoryRepository) ExpireReservation(id string) {
	tx, err := r.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()

	res, err := scanReservation(tx.QueryRow(`UPDATE reservations SET status = 'expired'
		WHERE id = ? AND status = 'active' AND expires_at <= ?
		RETURNING id, user_id, item_id, quantity, expires_at, status, created_at`, id, formatTime(time.Now().UTC())))
	if err != nil {
		return
	}
	if _, err := tx.Exec(`UPDATE stocks SET reserved_qty = reserved_qty - ? WHERE item_id = ?`, res.Quantity, res.ItemID); err != nil {
		return
	}
	_ = tx.Commit()
}

func (r *InventoryRepository) ReservationIDs() []string {
	rows, err := r.db.Query(`SELECT id FROM reservations WHERE status = 'active' AND expires_at <= ?`, formatTime(time.Now().UTC()))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

type rowScanner interface {
	Scan(...any) error
}

func scanReservation(row rowScanner) (*models.Reservation, error) {
	res := &models.Reservation{}
	var expiresAt, createdAt string
	if err := row.Scan(&res.ID, &res.UserID, &res.ItemID, &res.Quantity, &expiresAt, &res.Status, &createdAt); err != nil {
		return nil, err
	}
	var err error
	if res.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt); err != nil {
		return nil, err
	}
	if res.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return nil, err
	}
	return res, nil
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
