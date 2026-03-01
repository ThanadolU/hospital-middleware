package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type AuthContext struct {
	StaffID    uuid.UUID
	Username   string
	HospitalID uuid.UUID
}

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			c.Abort()
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		secret := os.Getenv("JWT_SECRET")

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			c.Abort()
			return
		}

		staffID, err := uuid.Parse(claims["id"].(string))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid id"})
			c.Abort()
			return
		}

		hospitalID, err := uuid.Parse(claims["hospital_id"].(string))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid hospital_id"})
			c.Abort()
			return
		}

		username := claims["username"].(string)

		c.Set("auth", AuthContext{
			StaffID: staffID,
			Username: username,
			HospitalID: hospitalID,
		})

		c.Next()
	}
}