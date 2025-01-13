// Internal/app/version.go.

package endpoints

import (
	"github.com/dkolesni-prog/transformer/internal/app/middleware"
	"net/http"
)

const errSomethingWentWrong = "Something went wrong"

func GetVersion(w http.ResponseWriter, r *http.Request, version string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only use GET!", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(version))
	if err != nil {
		http.Error(w, errSomethingWentWrong, http.StatusInternalServerError)
		middleware.Log.Printf("Error writing version response: %v", err)
		return
	}
}
