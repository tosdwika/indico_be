package controllers

import (
	"indico_be/app/repositories"
	"indico_be/app/services"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResetRequiresTokenAndResetsStock(t *testing.T) {
	repo, err := repositories.NewInventoryRepository(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Seed("item_1", 10); err != nil {
		t.Fatal(err)
	}
	controller := NewInventoryController(services.NewInventoryService(repo), "secret")

	request := func(token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/reset", strings.NewReader(`{"item_id":"item_1","total_stock":25}`))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		controller.Reset(w, r)
		return w
	}

	if status := request("").Code; status != http.StatusUnauthorized {
		t.Fatalf("without token: want %d, got %d", http.StatusUnauthorized, status)
	}
	if status := request("wrong").Code; status != http.StatusUnauthorized {
		t.Fatalf("wrong token: want %d, got %d", http.StatusUnauthorized, status)
	}
	if status := request("secret").Code; status != http.StatusOK {
		t.Fatalf("valid token: want %d, got %d", http.StatusOK, status)
	}
	stock, ok := repo.GetStock("item_1")
	if !ok || stock.TotalStock != 25 || stock.ReservedQty != 0 {
		t.Fatalf("stock not reset: %+v", stock)
	}
}
