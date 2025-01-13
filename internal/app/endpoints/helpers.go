// Internal/app/endpoints/helpers.go

package endpoints

import (
	"crypto/rand"
	"github.com/dkolesni-prog/transformer/internal/app/middleware"
	"math/big"
)

func RandStringRunes(n int) (string, error) {
	letterRunes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, n)
	for i := range b {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letterRunes))))
		if err != nil {
			middleware.Log.Printf("Error generating random number: %v", err)
			return "", err
		}
		b[i] = letterRunes[num.Int64()]
	}
	return string(b), nil
}

func EnsureTrailingSlash(rawURL string) string {
	if len(rawURL) == 0 {
		return rawURL
	}
	if rawURL[len(rawURL)-1] != '/' {
		return rawURL + "/"
	}
	return rawURL
}
