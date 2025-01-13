// Internal/app/endpoints/ping.go

package endpoints

import (
	"github.com/dkolesni-prog/transformer/internal/store"
	"net/http"
)

func Ping(w http.ResponseWriter, r *http.Request, s store.Store) {
	if err := s.Ping(r.Context()); err != nil {
		http.Error(w, "DB connection failed", http.StatusInternalServerError)
		return
	}
	// Otherwise, return 200 OK
	w.WriteHeader(http.StatusOK)
}
