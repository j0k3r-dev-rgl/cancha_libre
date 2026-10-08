package domain

import "errors"

// Sentinel domain errors for business rule violations.
var (
	ErrCourtNotFound       = errors.New("court not found")
	ErrSlotUnavailable     = errors.New("slot already reserved or unavailable")
	ErrInvalidBookingRange = errors.New("invalid reservation time range")
	ErrUnauthorized        = errors.New("unauthorized action")
)
