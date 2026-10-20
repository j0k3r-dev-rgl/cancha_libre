package http

import "net/http"

// PageOffset translates the one-based public page contract to an internal row
// offset. Validate before multiplying so hostile inputs cannot overflow int.
func PageOffset(page, limit int) (int, error) {
	maxInt := int(^uint(0) >> 1)
	if page < 1 || limit < 1 || limit > 100 || page-1 > maxInt/limit {
		return 0, NewTypedError(http.StatusBadRequest, TypeValidation, "Page must be positive, limit must be between 1 and 100, and the row offset must fit an integer.")
	}
	return (page - 1) * limit, nil
}
