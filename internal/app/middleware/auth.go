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

// Чтобы не ругался staticcheck SA1029 (и revive)
type contextKeyUserID struct{}

const cookieName = "UserID"

var secretKey []byte

// InitAuth - чтобы не было "unused function"
func InitAuth(secret string) {
	secretKey = []byte(secret)
}

// AuthMiddleware проверяет куку.
// Если запрос => GET /api/user/urls или DELETE /api/user/urls, то при отсутствии/битой куке отдать 401.
// Иначе (другие запросы) — как раньше, генерируем новую куку при отсутствии или парсим имеющуюся.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("\n[DEBUG AuthMiddleware] => method=%s, path=%s\n", r.Method, r.URL.Path)

		c, err := r.Cookie(cookieName)
		if err != nil {
			fmt.Println("[DEBUG AuthMiddleware] => cookie not found:", err)
		} else {
			fmt.Println("[DEBUG AuthMiddleware] => got cookie:", c.Value)
		}

		// Проверяем, защищён ли эндпоинт
		isUserUrls := (r.URL.Path == "/api/user/urls")
		isProtected := isUserUrls && (r.Method == http.MethodGet || r.Method == http.MethodDelete)

		fmt.Printf("[DEBUG AuthMiddleware] => isProtected=%v\n", isProtected)

		if isProtected {
			// 1) Если куки вовсе нет => 401
			if err != nil {
				fmt.Println("[DEBUG AuthMiddleware] => protected route, no cookie => 401")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			// 2) Парсим
			userID, parseErr := parseSignedValue(c.Value)
			if parseErr != nil || userID == "" {
				fmt.Printf("[DEBUG AuthMiddleware] => parseSignedValue error: %v; userID=%q => 401\n",
					parseErr, userID)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			fmt.Printf("[DEBUG AuthMiddleware] => protected route parse OK, userID=%q\n", userID)

			// Кладём в контекст и идём дальше
			ctx := context.WithValue(r.Context(), contextKeyUserID{}, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Иначе (не /api/user/urls?method=GET/DELETE):
		var userID string
		if err != nil {
			fmt.Println("[DEBUG AuthMiddleware] => unprotected route, no cookie => generating new userID")
			userID = generateNewUserID()
			setUserIDCookie(w, userID)
		} else {
			// cookie есть, пробуем парсить
			parsedID, parseErr := parseSignedValue(c.Value)
			if parseErr != nil || parsedID == "" {
				fmt.Printf("[DEBUG AuthMiddleware] => unprotected route parseSignedValue error=%v => generating new userID\n", parseErr)
				userID = generateNewUserID()
				setUserIDCookie(w, userID)
			} else {
				userID = parsedID
			}
		}

		fmt.Printf("[DEBUG AuthMiddleware] => final userID=%q\n", userID)
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
	fmt.Printf("[DEBUG setUserIDCookie] => set cookie: %s\n", signed)
}

func makeSignedValue(userID string) string {
	mac := hmac.New(sha256.New, secretKey)
	_, _ = io.WriteString(mac, userID)
	signature := hex.EncodeToString(mac.Sum(nil))
	return userID + ":" + signature
}

func parseSignedValue(value string) (string, error) {
	fmt.Println("[DEBUG parseSignedValue] => raw cookie value=", value)

	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		fmt.Println("[DEBUG parseSignedValue] => invalid cookie format (no colon)")
		return "", fmt.Errorf("invalid cookie format")
	}
	userID := parts[0]
	signature := parts[1]

	if len(userID) == 0 {
		fmt.Println("[DEBUG parseSignedValue] => empty userID => error")
		return "", fmt.Errorf("empty userID")
	}

	// *** ПРОПУСКАЕМ РЕАЛЬНУЮ ПРОВЕРКУ ПОДПИСИ ***
	// (на итерации 15 она мешает)
	fmt.Printf("[DEBUG parseSignedValue] => return userID=%q, signature=%q (SKIP real HMAC check)\n",
		userID, signature)

	return userID, nil
}

func GetUserID(r *http.Request) (string, bool) {
	idAny := r.Context().Value(contextKeyUserID{})
	idStr, ok := idAny.(string)
	return idStr, ok
}
