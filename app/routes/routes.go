package routes

import (
	"indico_be/app/http/controllers"
	"net/http"
)

// Register wires all routes — the equivalent of routes/web.php + routes/api.php.
func Register(mux *http.ServeMux, ic *controllers.InventoryController) {
	mux.HandleFunc("POST /api/v1/inventory/reserve", ic.Reserve)
	mux.HandleFunc("POST /api/v1/inventory/confirm", ic.Confirm)
	mux.HandleFunc("GET /api/v1/inventory/stock", ic.Stock)
}
