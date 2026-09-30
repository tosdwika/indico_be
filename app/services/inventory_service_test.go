package services

import (
	"fmt"
	"indico_be/app/repositories"
	"path/filepath"
	"sync"
	"testing"
)

func newTestSvc(t *testing.T, item string, stock int) *InventoryService {
	t.Helper()
	repo, err := repositories.NewInventoryRepository(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err := repo.Seed(item, stock); err != nil {
		t.Fatal(err)
	}
	return NewInventoryService(repo)
}

func TestReserveInsufficient(t *testing.T) {
	svc := newTestSvc(t, "item_1", 5)
	_, err := svc.Reserve("u1", "item_1", 6)
	if err != repositories.ErrInsufficientStock {
		t.Fatalf("want ErrInsufficientStock, got %v", err)
	}
}

func TestReserveItemNotFound(t *testing.T) {
	svc := newTestSvc(t, "item_1", 5)
	if _, err := svc.Reserve("u1", "nope", 1); err != repositories.ErrItemNotFound {
		t.Fatalf("want ErrItemNotFound, got %v", err)
	}
}

func TestConfirmNotFound(t *testing.T) {
	svc := newTestSvc(t, "item_1", 5)
	if _, err := svc.Confirm("res_missing"); err != ErrReservationNotFound {
		t.Fatalf("want ErrReservationNotFound, got %v", err)
	}
}

func TestReserveConfirmFlow(t *testing.T) {
	svc := newTestSvc(t, "item_1", 10)
	res, err := svc.Reserve("u1", "item_1", 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(res.ID); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	s, _ := svc.Stock("item_1")
	if s.TotalStock != 7 || s.ReservedQty != 0 || s.Available() != 7 {
		t.Fatalf("post-confirm stock wrong: %+v", s)
	}
}

func TestResetRestoresStockAndExpiresReservation(t *testing.T) {
	svc := newTestSvc(t, "item_1", 10)
	res, err := svc.Reserve("u1", "item_1", 3)
	if err != nil {
		t.Fatal(err)
	}
	stock, err := svc.Reset("item_1", 25)
	if err != nil {
		t.Fatal(err)
	}
	if stock.TotalStock != 25 || stock.ReservedQty != 0 || stock.Available() != 25 {
		t.Fatalf("stock not reset: %+v", stock)
	}
	if _, err := svc.Confirm(res.ID); err != ErrReservationNotFound {
		t.Fatalf("old reservation should be invalid, got %v", err)
	}
}

func TestSQLitePersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "indico.db")
	repo, err := repositories.NewInventoryRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Seed("item_1", 10); err != nil {
		t.Fatal(err)
	}
	svc := NewInventoryService(repo)
	res, err := svc.Reserve("u1", "item_1", 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}

	repo, err = repositories.NewInventoryRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	svc = NewInventoryService(repo)
	stored, ok := repo.GetReservation(res.ID)
	if !ok || stored.Status != "active" {
		t.Fatalf("reservation not persisted: %+v", stored)
	}
	stock, err := svc.Stock("item_1")
	if err != nil || stock.ReservedQty != 3 || stock.Available() != 7 {
		t.Fatalf("stock not persisted: %+v, %v", stock, err)
	}
}

// Stress: 500 goroutines race for 100 stock — exactly 100 must win, 0 oversell.
func TestStressNoOversell(t *testing.T) {
	const stock = 100
	const goroutines = 500
	svc := newTestSvc(t, "hot", stock)

	var mu sync.Mutex
	success := 0
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			// each user asks for 1 unit
			if _, err := svc.Reserve(fmt.Sprintf("user_%d", n), "hot", 1); err == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if success != stock {
		t.Fatalf("oversell/miscount: want exactly %d successful reservations, got %d", stock, success)
	}
	s, _ := svc.Stock("hot")
	if s.ReservedQty != stock || s.Available() != 0 {
		t.Fatalf("stock inconsistent after stress: %+v", s)
	}
}

// Stress: concurrent confirms of same reservation — exactly one wins.
func TestStressConcurrentConfirmIdempotency(t *testing.T) {
	svc := newTestSvc(t, "item_1", 10)
	res, _ := svc.Reserve("u1", "item_1", 2)

	var success int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.Confirm(res.ID); err == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if success != 1 {
		t.Fatalf("want exactly 1 successful confirm, got %d", success)
	}
	s, _ := svc.Stock("item_1")
	if s.TotalStock != 8 {
		t.Fatalf("stock decremented more than once: %+v", s)
	}
}
