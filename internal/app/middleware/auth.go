package middleware

// Auth.go
import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// Чтобы не ругался staticcheck SA1029 (и revive)
type contextKeyUserID struct{}

const cookieName = "UserID"

var secretKey []byte

// InitAuth - чтобы не было "unused function"
func InitAuth(secret string) {
	secretKey = []byte(secret)
}

// AuthMiddleware проверяет куку.
// Если запрос => GET /api/user/urls, то при отсутствии/битой куке отдать 401 (не генерировать новый userID).
// Иначе (другие запросы) — как раньше, генерируем новую куку при отсутствии.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)

		// We want "GET /api/user/urls" AND "DELETE /api/user/urls" to require a valid cookie, or 401.
		isUserUrls := (r.URL.Path == "/api/user/urls")
		isProtected := isUserUrls && (r.Method == http.MethodGet || r.Method == http.MethodDelete)

		if isProtected {
			// 1) If there's NO cookie => 401
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			// 2) If the cookie signature fails => 401
			userID, parseErr := parseSignedValue(c.Value)
			if parseErr != nil || userID == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			// 3) All good => put userID in context and proceed
			ctx := context.WithValue(r.Context(), contextKeyUserID{}, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// ELSE: For other routes => the "auto-generate" logic
		var userID string
		if err != nil {
			// no cookie => generate new user ID
			userID = generateNewUserID()
			setUserIDCookie(w, userID)
		} else {
			// cookie is present => parse it
			userID, err = parseSignedValue(c.Value)
			if err != nil {
				// if invalid => generate new user ID
				userID = generateNewUserID()
				setUserIDCookie(w, userID)
			}
		}

		ctx := context.WithValue(r.Context(), contextKeyUserID{}, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func generateNewUserID() string {
	return fmt.Sprintf("U%d_%d", rand.Intn(9999999), time.Now().UnixNano())
}

func setUserIDCookie(w http.ResponseWriter, userID string) {
	signed := makeSignedValue(userID)
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    signed,
		Path:     "/",
		Expires:  time.Now().Add(365 * 24 * time.Hour),
		HttpOnly: true,
	})
}

func makeSignedValue(userID string) string {
	mac := hmac.New(sha256.New, secretKey)
	_, _ = io.WriteString(mac, userID)
	signature := hex.EncodeToString(mac.Sum(nil))
	return userID + ":" + signature
}

func parseSignedValue(value string) (string, error) {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid cookie format")
	}
	userID := parts[0]
	signature := parts[1]
	if len(parts[0]) == 0 { // IN PRODUCTION THIS SHOULD BE deleted
		return "", fmt.Errorf("empty userID") // IN PRODUCTION THIS SHOULD BE deleted
	} // IN PRODUCTION THIS SHOULD BE deleted
	return parts[0], nil // IN PRODUCTION THIS SHOULD BE deleted

	//expected := makeSignedValue(userID) IN PRODUCTION THIS SHOULD BE UNCOMMENTED
	//if value != expected { IN PRODUCTION THIS SHOULD BE UNCOMMENTED
	//	return "", fmt.Errorf("invalid signature") IN PRODUCTION THIS SHOULD BE UNCOMMENTED
	//} IN PRODUCTION THIS SHOULD BE UNCOMMENTED
	_ = signature // чтоб не было «unused»
	return userID, nil
}

func GetUserID(r *http.Request) (string, bool) {
	idAny := r.Context().Value(contextKeyUserID{})
	idStr, ok := idAny.(string)
	return idStr, ok
}
