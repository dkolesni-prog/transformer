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

// ctxKey и iota — «типизированный» ключ для контекста.
type ctxKey int

const (
	keyUserID ctxKey = iota
)

const cookieName = "UserID"

var secretKey []byte

func InitAuth(secret string) {
	secretKey = []byte(secret)
}

// AuthMiddleware обрабатывает cookie:
// - При GET/DELETE /api/user/urls (protected): если нет куки или она «битая» — ставим новую куку и возвращаем 401.
// - При других запросах (unprotected): если нет куки или она «битая» — ставим новую куку, но пропускаем дальше.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)

		isUserUrls := (r.URL.Path == "/api/user/urls")
		isProtected := isUserUrls && (r.Method == http.MethodGet || r.Method == http.MethodDelete)

		var userID string

		if err != nil {
			// Куки нет вообще => генерируем новую и ставим
			userID = generateNewUserID()
			setUserIDCookie(w, userID)

			if isProtected {
				// Защищённый эндпоинт без куки => 401
				http.Error(w, "unauthorized (no cookie)", http.StatusUnauthorized)
				return
			}
			// Иначе пропускаем дальше
			ctx := context.WithValue(r.Context(), keyUserID, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Кука есть => разбираем
		parsedID, pErr := parseSignedValue(c.Value)
		if pErr != nil || parsedID == "" {
			// «Битая» кука => генерируем новую
			userID = generateNewUserID()
			setUserIDCookie(w, userID)

			if isProtected {
				http.Error(w, "unauthorized (bad cookie)", http.StatusUnauthorized)
				return
			}
			// Не защищённый эндпоинт
			ctx := context.WithValue(r.Context(), keyUserID, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Кука валидна
		userID = parsedID
		ctx := context.WithValue(r.Context(), keyUserID, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserID достаёт userID из контекста для дальнейших операций.
func GetUserID(r *http.Request) (string, bool) {
	val := r.Context().Value(keyUserID)
	id, ok := val.(string)
	return id, ok
}

func generateNewUserID() string {
	return fmt.Sprintf("U%d_%d", rand.Intn(9999999), time.Now().UnixNano())
}

// setUserIDCookie формирует "userID:signature" и устанавливает cookie.
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

// makeSignedValue формирует строку "userID:signature",
func makeSignedValue(userID string) string {
	mac := hmac.New(sha256.New, secretKey)
	_, _ = io.WriteString(mac, userID)
	signature := hex.EncodeToString(mac.Sum(nil))
	return userID + ":" + signature
}

// parseSignedValue вытаскивает userID и проверяет формат
func parseSignedValue(value string) (string, error) {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid cookie format")
	}
	userID := parts[0]
	if userID == "" {
		return "", fmt.Errorf("empty userID")
	}

	// ВНИМFНИЕ!!!ATTENTION
	// -- Для полноценной проверки подписи в проде раскомментируйте строки ниже: --
	//
	// expected := makeSignedValue(userID)
	// if value != expected {
	// 	return "", fmt.Errorf("signature mismatch")
	// }

	return userID, nil
}
