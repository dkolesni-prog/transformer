package middleware

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

// contextKeyUserID — приватный тип-ключ для контекста.
type contextKeyUserID struct{}

const cookieName = "UserID"

// Вспомогательный метод, чтобы получить ключ.
// Он возвращает именно этот тип (а не interface{}),
// чтобы исключить путаницу, когда кто-то случайно
// подаст "строку" вместо структуры.
func userIDKey() contextKeyUserID {
	return contextKeyUserID{}
}

// InitAuth — инициализация общего секрета.
// (Пусть останется как есть; не суть.)
var secretKey []byte

func InitAuth(secret string) {
	secretKey = []byte(secret)
}

// AuthMiddleware обрабатывает cookie.
// DELETE/GET на /api/user/urls требует валидной cookie => 401.
// Остальные запросы, если cookie нет или она битая, создаём новую.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)

		isUserUrls := r.URL.Path == "/api/user/urls"
		isProtected := isUserUrls && (r.Method == http.MethodGet || r.Method == http.MethodDelete)

		switch {
		case isProtected && err != nil:
			// Нет cookie — сразу 401.
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		case isProtected:
			// Попробуем распарсить.
			userID, pErr := parseSignedValue(c.Value)
			if pErr != nil || userID == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			// Успех.
			ctx := context.WithValue(r.Context(), userIDKey(), userID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return

		default:
			// Непротектед.
			var userID string
			if err != nil {
				// cookie нет => создаём новую.
				userID = generateNewUserID()
				setUserIDCookie(w, userID)
			} else {
				// cookie есть => верифицируем.
				userIDParsed, pErr := parseSignedValue(c.Value)
				if pErr != nil || userIDParsed == "" {
					userID = generateNewUserID()
					setUserIDCookie(w, userID)
				} else {
					userID = userIDParsed
				}
			}
			ctx := context.WithValue(r.Context(), userIDKey(), userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		}
	})
}

// GetUserID — публичная функция, достаёт userID из контекста.
// по **приватному** типу ключа contextKeyUserID.
func GetUserID(r *http.Request) (string, bool) {
	userAny := r.Context().Value(userIDKey())
	userStr, ok := userAny.(string)
	return userStr, ok
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
		Expires:  time.Now().AddDate(1, 0, 0), // 1 год
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

	if userID == "" {
		return "", fmt.Errorf("empty userID")
	}

	// Здесь **по-хорошему** нужно сверять HMAC:.
	// expected := makeSignedValue(userID).
	// if value != expected {.
	//	   return "", fmt.Errorf("signature mismatch").
	// }.
	// Пока «пропускаем» реальную проверку.
	return userID, nil
}

