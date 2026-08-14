// Package middleware holds the Gin middleware, chiefly authentication.
package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ThanadolU/hospital-middleware/internal/auth"
)

// Context keys under which the authenticated identity is stored.
const (
	ContextStaffID    = "staff_id"
	ContextHospitalID = "hospital_id"
	ContextUsername   = "username"
)

// RequireAuth rejects any request without a valid bearer token, and puts the
// authenticated staff identity into the request context.
//
// The hospital placed here is the only scope a handler may search with. It
// comes from the signed token, so a client cannot choose the hospital whose
// patients it sees.
func RequireAuth(tokens *auth.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := bearerToken(c.GetHeader("Authorization"))
		if err != nil {
			abortUnauthorized(c, "missing or malformed Authorization header")
			return
		}

		claims, err := tokens.Verify(token)
		if err != nil {
			// Expiry is worth distinguishing: it tells the caller to log in
			// again rather than implying their credentials were wrong. Nothing
			// further about why a token failed is disclosed.
			if errors.Is(err, auth.ErrExpiredToken) {
				abortUnauthorized(c, "token expired")
				return
			}
			abortUnauthorized(c, "invalid token")
			return
		}

		staffID, err := claims.StaffID()
		if err != nil {
			abortUnauthorized(c, "invalid token")
			return
		}

		c.Set(ContextStaffID, staffID)
		c.Set(ContextHospitalID, claims.HospitalID)
		c.Set(ContextUsername, claims.Username)
		c.Next()
	}
}

// HospitalIDFrom returns the authenticated hospital scope.
//
// It returns an error rather than a zero value when absent, so a handler that
// is accidentally mounted outside RequireAuth fails closed instead of running
// an unscoped search.
func HospitalIDFrom(c *gin.Context) (uuid.UUID, error) {
	value, ok := c.Get(ContextHospitalID)
	if !ok {
		return uuid.Nil, errors.New("middleware: no authenticated hospital in context")
	}
	hospitalID, ok := value.(uuid.UUID)
	if !ok || hospitalID == uuid.Nil {
		return uuid.Nil, errors.New("middleware: authenticated hospital is invalid")
	}
	return hospitalID, nil
}

// bearerToken extracts the credential from an Authorization header, accepting
// the scheme case-insensitively as RFC 7235 requires.
func bearerToken(header string) (string, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", errors.New("empty header")
	}

	scheme, credential, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", errors.New("not a bearer token")
	}

	credential = strings.TrimSpace(credential)
	if credential == "" {
		return "", errors.New("empty credential")
	}
	return credential, nil
}

func abortUnauthorized(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": message})
}
