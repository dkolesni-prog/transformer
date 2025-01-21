// Internal/app/helpers/helpers.go.

package helpers

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"

	"github.com/dkolesni-prog/transformer/internal/app/middleware"
)

func RandStringRunes(n int) (string, error) {
	letterRunes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, n)

	for i := range b {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letterRunes))))
		if err != nil {
			wrappedErr := errors.New("error generating random number")
			middleware.Log.Error().Err(err).Msg("RandStringRunes failed")
			return "", wrappedErr
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

func Classify(dsn string) string {
	if dsn == "" {
		return ""
	}

	sliceOfStringsSeparatedByColon := strings.Split(dsn, ":")
	subSLiceSeparatedByAT := strings.Split(sliceOfStringsSeparatedByColon[2], "@")
	secretToClassify := subSLiceSeparatedByAT[0]
	classifiedString := strings.Replace(dsn, secretToClassify, "♦️ ♠️ ♥️ ♣️   ", 1)
	return classifiedString
}
