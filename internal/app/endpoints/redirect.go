// Internal/app/endpoints/redirect.go

package endpoints

import (
	"context"
	"github.com/dkolesni-prog/transformer/internal/app/middleware"
	"github.com/dkolesni-prog/transformer/internal/store"
	"github.com/go-chi/chi/v5"
	"net/http"
)

func GetFullURL(ctx context.Context, w http.ResponseWriter, r *http.Request, s store.Store) {
	id := chi.URLParam(r, "id")

	long, ok := s.Load(ctx, id)
	if ok != nil {
		http.Error(w, "Short URL not found", http.StatusNotFound)
		middleware.Log.Printf("Could not find a short URL")
		return
	}
	http.Redirect(w, r, long.String(), http.StatusTemporaryRedirect)
}
