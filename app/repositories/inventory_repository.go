package repositories

import (
	"indico_be/app/models"
	"sync"
	"time"
)

// In-memory store with a per-item mutex-sharded map.
// NOTE: single-instance state; in distributed mode, replace with Postgres
// and use SELECT ... FOR UPDATE (or conditional UPDATE) for the hot path.
type InventoryRepository struct {
	mu    sync.Mutex // guards stocks/reservations maps themselves
	locks map[string]*sync.Mutex

	stocks       map[string]*models.Stock
	reservations map[string]*models.Reservation
}

func NewInventoryRepository() *InventoryRepository {
	return &InventoryRepository{
		locks:        make(map[string]*sync.Mutex),
		stocks:       make(map[string]*models.Stock),
		reservations: make(map[string]*models.Reservation),
	}
}

func (r *InventoryRepository) lockItem(itemID string) *sync.Mutex {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.locks[itemID]
	if !ok {
		l = &sync.Mutex{}
		r.locks[itemID] = l
	}
	return l
}

// mapOf guards raw map access (r.mu); the pointed-to values are mutated under the item lock.
func (r *InventoryRepository) stockOf(itemID string) *models.Stock {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stocks[itemID]
}

func (r *InventoryRepository) putReservation(res *models.Reservation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reservations[res.ID] = res
}

// Seed adds an item with the given total stock (idempotent reset).
func (r *InventoryRepository) Seed(itemID string, total int) {
	l := r.lockItem(itemID)
	l.Lock()
	defer l.Unlock()
	r.mu.Lock()
	r.stocks[itemID] = &models.Stock{ItemID: itemID, TotalStock: total}
	r.mu.Unlock()
}

func (r *InventoryRepository) GetStock(itemID string) (*models.Stock, bool) {
	l := r.lockItem(itemID)
	l.Lock()
	defer l.Unlock()
	s := r.stockOf(itemID)
	if s == nil {
		return nil, false
	}
	cp := *s
	return &cp, true
}

// CreateReservation atomically reserves qty if available. Returns the reservation or error.
func (r *InventoryRepository) CreateReservation(res *models.Reservation) error {
	l := r.lockItem(res.ItemID)
	l.Lock()
	defer l.Unlock()

	s := r.stockOf(res.ItemID)
	if s == nil {
		return ErrItemNotFound
	}
	if s.Available() < res.Quantity {
		return ErrInsufficientStock
	}
	s.ReservedQty += res.Quantity
	r.putReservation(res)
	return nil
}

// GetReservation returns a copy taken under the item lock
// (Status is mutated under the item lock in Confirm/Expire).
func (r *InventoryRepository) GetReservation(id string) (*models.Reservation, bool) {
	r.mu.Lock()
	res, ok := r.reservations[id]
	r.mu.Unlock()
	if !ok {
		return nil, false
	}
	l := r.lockItem(res.ItemID)
	l.Lock()
	defer l.Unlock()
	cp := *res
	return &cp, true
}

// ConfirmReservation atomically confirms an active reservation and decrements physical stock.
func (r *InventoryRepository) ConfirmReservation(id string, now time.Time) (*models.Reservation, error) {
	r.mu.Lock()
	res, ok := r.reservations[id]
	r.mu.Unlock()
	if !ok {
		return nil, ErrReservationNotFound
	}

	l := r.lockItem(res.ItemID)
	l.Lock()
	defer l.Unlock()

	if res.Status == "confirmed" {
		return nil, ErrAlreadyConfirmed
	}
	if res.Status == "expired" {
		return nil, ErrReservationNotFound
	}
	if now.After(res.ExpiresAt) {
		return nil, ErrReservationExpired
	}

	s := r.stockOf(res.ItemID)
	s.TotalStock -= res.Quantity
	s.ReservedQty -= res.Quantity
	res.Status = "confirmed"
	cp := *res
	return &cp, nil
}

// ExpireReservation releases an expired reservation's hold. Idempotent.
func (r *InventoryRepository) ExpireReservation(id string) {
	r.mu.Lock()
	res, ok := r.reservations[id]
	r.mu.Unlock()
	if !ok {
		return
	}

	l := r.lockItem(res.ItemID)
	l.Lock()
	defer l.Unlock()

	if res.Status != "active" || res.ExpiresAt.After(time.Now()) {
		return
	}
	if s := r.stockOf(res.ItemID); s != nil {
		s.ReservedQty -= res.Quantity
	}
	res.Status = "expired"
}

// ReservationIDs snapshots all reservation IDs for the cleanup sweep.
// NOTE: O(n) scan per sweep; if the reservation count grows large, maintain
// an expiry-ordered index (heap) instead.
func (r *InventoryRepository) ReservationIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.reservations))
	for id := range r.reservations {
		ids = append(ids, id)
	}
	return ids
}
