package controllers

import (
	"indico_be/app/repositories"
	"indico_be/app/services"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResetStock(t *testing.T) {
	repo, err := repositories.NewInventoryRepository(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Seed("item_1", 10); err != nil {
		t.Fatal(err)
	}
	controller := NewInventoryController(services.NewInventoryService(repo))
	r := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/reset", strings.NewReader(`{"item_id":"item_1","total_stock":25}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	controller.Reset(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want %d, got %d", http.StatusOK, w.Code)
	}
	stock, ok := repo.GetStock("item_1")
	if !ok || stock.TotalStock != 25 || stock.ReservedQty != 0 {
		t.Fatalf("stock not reset: %+v", stock)
	}
}
