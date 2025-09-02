package main

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

func main() {
	// Current server configuration
	secret := "your-local-jwt-secret"
	issuer := "trade_company"

	// Create token for user ID 183 (from the logs)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"uid":   183,
		"email": "john.doe@example.com",
		"role":  "user",
		"iss":   issuer,
		"exp":   time.Now().Add(24 * time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	})

	// Sign token with correct secret
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		panic(err)
	}

	fmt.Printf("Fresh JWT Token for User ID 183:\n%s\n\n", tokenString)
	fmt.Printf("Token Details:\n")
	fmt.Printf("- User ID: 183\n")
	fmt.Printf("- Email: john.doe@example.com\n")
	fmt.Printf("- Role: user\n")
	fmt.Printf("- Issuer: %s\n", issuer)
	fmt.Printf("- Secret: %s\n", secret)
	fmt.Printf("- Expires: %s\n", time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05"))
}