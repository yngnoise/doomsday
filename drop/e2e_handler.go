package drop

import (
	"net/http"
	"os"
	"time"
)

// ExpireReservationForTest is registered only in APP_ENV=test. It provides a
// deterministic browser-test seam without weakening production routes.
func (h *Handler) ExpireReservationForTest(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("APP_ENV") != "test" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	expiresAt := time.Now().UTC().Add(-time.Second)
	result, err := h.db.Exec(r.Context(), `
		UPDATE reservations SET expires_at=$2
		WHERE id=$1 AND status='pending'
	`, r.PathValue("reservationID"), expiresAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not expire reservation")
		return
	}
	if result.RowsAffected() != 1 {
		writeError(w, http.StatusNotFound, "pending reservation not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"expires_at": expiresAt.Format(time.RFC3339)})
}
