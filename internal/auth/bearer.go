package auth

import (
	"errors"
	"net/http"
	"strings"
)

func GetBearerToken(headers http.Header) (string, error) {
	authHeaderData := headers.Get("Authorization")
	if authHeaderData == "" {
		return "", errors.New("Authorization header is empty")
	}
	bearer, token, found := strings.Cut(authHeaderData, " ")
	if !found {
		return "", errors.New("could not find bearer token")
	}
	if bearer != "Bearer" {
		return "", errors.New("Not a bearer token")
	}
	return token, nil
}
