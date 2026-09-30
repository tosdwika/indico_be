package services

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"indico_be/app/models"
	"indico_be/app/repositories"
)

const ReservationTTL = 5 * time.Minute

var (
	ErrItemNotFound        = repositories.ErrItemNotFound
	ErrInsufficientStock   = repositories.ErrInsufficientStock
	ErrReservationNotFound = repositories.ErrReservationNotFound
	ErrReservationExpired  = repositories.ErrReservationExpired
	ErrAlreadyConfirmed    = repositories.ErrAlreadyConfirmed
)

type InventoryService struct {
	repo *repositories.InventoryRepository
}

func NewInventoryService(repo *repositories.InventoryRepository) *InventoryService {
	return &InventoryService{repo: repo}
}

func (s *InventoryService) Reserve(userID, itemID string, qty int) (*models.Reservation, error) {
	res := &models.Reservation{
		ID:        newID("res"),
		UserID:    userID,
		ItemID:    itemID,
		Quantity:  qty,
		Status:    "active",
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(ReservationTTL),
	}
	if err := s.repo.CreateReservation(res); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *InventoryService) Confirm(id string) (*models.Reservation, error) {
	res, ok := s.repo.GetReservation(id)
	if !ok {
		return nil, ErrReservationNotFound
	}
	// lazy expiry: release stock before rejecting
	if res.Status == "active" && time.Now().UTC().After(res.ExpiresAt) {
		s.repo.ExpireReservation(id)
		return nil, ErrReservationExpired
	}
	return s.repo.ConfirmReservation(id, time.Now().UTC())
}

func (s *InventoryService) Reset(itemID string, total int) (*models.Stock, error) {
	if err := s.repo.ResetStock(itemID, total); err != nil {
		return nil, err
	}
	stock, _ := s.repo.GetStock(itemID)
	return stock, nil
}

func (s *InventoryService) Stock(itemID string) (*models.Stock, error) {
	if s, ok := s.repo.GetStock(itemID); ok {
		return s, nil
	}
	return nil, ErrItemNotFound
}

func (s *InventoryService) CleanupExpired() {
	for _, id := range s.repo.ReservationIDs() {
		s.repo.ExpireReservation(id) // idempotent: skips non-expired
	}
}

func newID(prefix string) string {
	b := make([]byte, 6)
	rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}
