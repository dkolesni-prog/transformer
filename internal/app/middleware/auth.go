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

		// Проверяем: если это GET /api/user/urls
		if r.Method == http.MethodGet && r.URL.Path == "/api/user/urls" {
			// Если куки нет => 401
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			// Кука есть, но проверяем подпись
			userID, parseErr := parseSignedValue(c.Value)
			if parseErr != nil || userID == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			// Всё ок, кладём userID в контекст
			ctx := context.WithValue(r.Context(), contextKeyUserID{}, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Иначе (POST /, POST /api/shorten, …) — логика "старого"кода (...iter13] ):
		var userID string
		if err != nil {
			// Если нет куки => сгенерировать новый userID
			userID = generateNewUserID()
			setUserIDCookie(w, userID)
		} else {
			// Кука есть, но может быть битая
			userID, err = parseSignedValue(c.Value)
			if err != nil {
				// подпись не верна => генерим новый
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
	expected := makeSignedValue(userID)
	if value != expected {
		return "", fmt.Errorf("invalid signature")
	}
	_ = signature // чтоб не было «unused»
	return userID, nil
}

func GetUserID(r *http.Request) (string, bool) {
	idAny := r.Context().Value(contextKeyUserID{})
	idStr, ok := idAny.(string)
	return idStr, ok
}
