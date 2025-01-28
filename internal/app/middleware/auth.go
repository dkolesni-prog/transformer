package middleware

// Auth.go
import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

const cookieName = "UserID"

var secretKey []byte

// InitAuth - чтобы не было "unused function"
func InitAuth(secret string) {
	secretKey = []byte(secret)
}

// AuthMiddleware ...
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		var userID string

		if err != nil {
			// Нет куки
			userID = generateNewUserID()
			setUserIDCookie(w, userID)
		} else {
			parsedUser, parseErr := parseSignedValue(c.Value)
			if parseErr != nil {
				userID = generateNewUserID()
				setUserIDCookie(w, userID)
			} else {
				userID = parsedUser
			}
		}

		ctx := context.WithValue(r.Context(), "userID", userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func generateNewUserID() string {
	// Убрали rand.Seed(...)
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
		return "", errors.New("invalid cookie format")
	}
	userID := parts[0]
	signature := parts[1]

	// Считаем ожидаемый сигнат
	mac := hmac.New(sha256.New, secretKey)
	_, _ = io.WriteString(mac, userID)
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if signature != expectedSig {
		return "", errors.New("invalid signature")
	}
	return userID, nil
}
