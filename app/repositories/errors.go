package repositories

import "errors"

var (
	ErrItemNotFound       = errors.New("item not found")
	ErrInsufficientStock  = errors.New("insufficient stock")
	ErrReservationNotFound = errors.New("reservation not found")
	ErrReservationExpired = errors.New("reservation expired")
	ErrAlreadyConfirmed   = errors.New("reservation already confirmed")
)
