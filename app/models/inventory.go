package models

import "time"

type Stock struct {
	ItemID       string
	TotalStock   int
	ReservedQty  int // sum of active (unexpired, unconfirmed) reservations
}

func (s Stock) Available() int { return s.TotalStock - s.ReservedQty }

type Reservation struct {
	ID        string
	UserID    string
	ItemID    string
	Quantity  int
	ExpiresAt time.Time
	Status    string // "active", "confirmed"
	CreatedAt time.Time
}

type ReserveRequest struct {
	UserID   string `json:"user_id"`
	ItemID   string `json:"item_id"`
	Quantity int    `json:"quantity"`
}

type ConfirmRequest struct {
	ReservationID string `json:"reservation_id"`
}

type ReserveResponse struct {
	Status        string    `json:"status"`
	ReservationID string    `json:"reservation_id"`
	ItemID        string    `json:"item_id"`
	Quantity      int       `json:"quantity"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type ConfirmResponse struct {
	Status        string    `json:"status"`
	ReservationID string    `json:"reservation_id"`
	ConfirmedAt   time.Time `json:"confirmed_at"`
}

type StockResponse struct {
	ItemID         string `json:"item_id"`
	TotalStock     int    `json:"total_stock"`
	ReservedStock  int    `json:"reserved_stock"`
	AvailableStock int    `json:"available_stock"`
}
