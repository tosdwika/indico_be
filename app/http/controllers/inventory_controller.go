package controllers

import (
	"indico_be/app/models"
	"net/http"
	"time"

	"indico_be/app/services"
)

type InventoryController struct {
	Svc *services.InventoryService
}

func NewInventoryController(svc *services.InventoryService) *InventoryController {
	return &InventoryController{Svc: svc}
}

// POST /api/v1/inventory/reserve
func (c *InventoryController) Reserve(w http.ResponseWriter, r *http.Request) {
	var req models.ReserveRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if req.UserID == "" || req.ItemID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "user_id and item_id are required")
		return
	}
	if req.Quantity < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "quantity must be at least 1")
		return
	}

	res, err := c.Svc.Reserve(req.UserID, req.ItemID, req.Quantity)
	if err != nil {
		switch err {
		case services.ErrItemNotFound:
			writeError(w, http.StatusNotFound, "ITEM_NOT_FOUND", "item does not exist")
		case services.ErrInsufficientStock:
			writeError(w, http.StatusConflict, "INSUFFICIENT_STOCK", "not enough available stock")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL", "unexpected error")
		}
		return
	}

	writeJSON(w, http.StatusCreated, models.ReserveResponse{
		Status:        "success",
		ReservationID: res.ID,
		ItemID:        res.ItemID,
		Quantity:      res.Quantity,
		ExpiresAt:     res.ExpiresAt,
	})
}

// POST /api/v1/inventory/confirm
func (c *InventoryController) Confirm(w http.ResponseWriter, r *http.Request) {
	var req models.ConfirmRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if req.ReservationID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "reservation_id is required")
		return
	}

	res, err := c.Svc.Confirm(req.ReservationID)
	if err != nil {
		switch err {
		case services.ErrReservationNotFound:
			writeError(w, http.StatusNotFound, "RESERVATION_NOT_FOUND", "reservation does not exist")
		case services.ErrReservationExpired:
			writeError(w, http.StatusGone, "RESERVATION_EXPIRED", "reservation has expired and stock was released")
		case services.ErrAlreadyConfirmed:
			writeError(w, http.StatusConflict, "ALREADY_CONFIRMED", "reservation already confirmed")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL", "unexpected error")
		}
		return
	}

	writeJSON(w, http.StatusOK, models.ConfirmResponse{
		Status:        "success",
		ReservationID: res.ID,
		ConfirmedAt:   time.Now().UTC(),
	})
}

// GET /api/v1/inventory/stock?item_id=...
func (c *InventoryController) Stock(w http.ResponseWriter, r *http.Request) {
	itemID := r.URL.Query().Get("item_id")
	if itemID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "item_id query parameter is required")
		return
	}

	s, err := c.Svc.Stock(itemID)
	if err != nil {
		if err == services.ErrItemNotFound {
			writeError(w, http.StatusNotFound, "ITEM_NOT_FOUND", "item does not exist")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL", "unexpected error")
		return
	}

	writeJSON(w, http.StatusOK, models.StockResponse{
		ItemID:         s.ItemID,
		TotalStock:     s.TotalStock,
		ReservedStock:  s.ReservedQty,
		AvailableStock: s.Available(),
	})
}
