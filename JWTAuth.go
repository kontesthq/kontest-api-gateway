package main

import (
	"errors"
	"net/http"
)

type JWTAuth struct {
	secret []byte
}

func NewJWTAuth(secret []byte) *JWTAuth {
	return &JWTAuth{
		secret: secret,
	}
}

func (j *JWTAuth) Authenticate(w http.ResponseWriter, r *http.Request) (bool, error) {
	// Extract the JWT from the Authorization header
	tokenString := r.Header.Get("Authorization")
	if tokenString == "" {
		return false, errors.New("missing Authorization header")
	}

	// Validate the JWT
	isValid, err := ValidateJWT(tokenString, j.secret)
	if err != nil {
		return false, err
	}

	return isValid, nil
}
