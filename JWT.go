package main

import (
	"fmt"
	"github.com/golang-jwt/jwt"
	"time"
)

func ValidateJWT(tokenString string, secret []byte) (bool, error) {
	claims := jwt.StandardClaims{}

	token, err := jwt.ParseWithClaims(tokenString, &claims, func(token *jwt.Token) (interface{}, error) {
		return secret, nil
	})

	if err != nil || !token.Valid {
		return false, err
	}

	// Check if the exp claim exists
	if claims.ExpiresAt == 0 {
		return false, fmt.Errorf("token does not contain exp claim")
	}

	// Check if the token is expired
	if time.Unix(claims.ExpiresAt, 0).Before(time.Now()) {
		return false, fmt.Errorf("token has expired")
	}

	return true, nil
}
