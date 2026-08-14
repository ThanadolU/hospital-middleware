package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/ThanadolU/hospital-middleware/internal/models"
)

// SecretEnv names the environment variable holding the signing secret.
const SecretEnv = "JWT_SECRET"

// signingMethod is the one algorithm this service issues and accepts.
//
// Pinning it is what closes the JWT algorithm-confusion hole: a verifier that
// trusts the token's own `alg` header will happily validate a token the
// attacker chose the algorithm for. v1's keyfunc returned the secret without
// ever inspecting the method.
var signingMethod = jwt.SigningMethodHS256

// DefaultTokenTTL is how long an issued token stays valid.
const DefaultTokenTTL = 12 * time.Hour

// minSecretLength rejects trivially guessable secrets at construction, so a
// weak deployment fails at boot rather than silently issuing weak tokens.
const minSecretLength = 32

var (
	// ErrInvalidToken covers every reason a token is unacceptable except
	// expiry: bad signature, wrong algorithm, malformed, missing claims.
	ErrInvalidToken = errors.New("auth: invalid token")

	// ErrExpiredToken is separated so the API can tell a caller to log in
	// again rather than implying their credentials were wrong.
	ErrExpiredToken = errors.New("auth: token expired")
)

// Claims is the token payload.
//
// It is a typed struct rather than jwt.MapClaims on purpose. v1 read claims out
// of a map with unchecked assertions (`claims["id"].(string)`), so a token with
// an unexpected claim type panicked the request instead of returning 401.
// Decoding into a struct makes a malformed token a parse error.
type Claims struct {
	jwt.RegisteredClaims

	// HospitalID is the scope every patient search is bound to. It is read
	// from here and nowhere else — never from query input — which is what
	// keeps hospital isolation enforceable at the HTTP layer.
	HospitalID uuid.UUID `json:"hospital_id"`
	Username   string    `json:"username"`
}

// StaffID returns the authenticated staff member's id.
func (c Claims) StaffID() (uuid.UUID, error) {
	id, err := uuid.Parse(c.Subject)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: subject is not a uuid", ErrInvalidToken)
	}
	return id, nil
}

// TokenService issues and verifies staff tokens.
type TokenService struct {
	secret []byte
	ttl    time.Duration

	// now is injectable so expiry can be tested without sleeping.
	now func() time.Time
}

// NewTokenService builds a token service. It fails when the secret is missing
// or too short: there is deliberately no fallback default, because a hardcoded
// development secret that reaches production forges every token in the system.
func NewTokenService(secret string, ttl time.Duration) (*TokenService, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, fmt.Errorf("auth: %s must be set", SecretEnv)
	}
	if len(secret) < minSecretLength {
		return nil, fmt.Errorf("auth: %s must be at least %d bytes", SecretEnv, minSecretLength)
	}
	if ttl <= 0 {
		ttl = DefaultTokenTTL
	}
	return &TokenService{secret: []byte(secret), ttl: ttl, now: time.Now}, nil
}

// Issue returns a signed token identifying staff and the hospital they belong to.
func (s *TokenService) Issue(staff models.Staff) (string, error) {
	if staff.ID == uuid.Nil || staff.HospitalID == uuid.Nil {
		return "", errors.New("auth: staff must have an id and a hospital")
	}

	issuedAt := s.now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   staff.ID.String(),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(issuedAt.Add(s.ttl)),
			NotBefore: jwt.NewNumericDate(issuedAt),
		},
		HospitalID: staff.HospitalID,
		Username:   staff.Username,
	}

	signed, err := jwt.NewWithClaims(signingMethod, claims).SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("auth: sign token: %w", err)
	}
	return signed, nil
}

// Verify parses and validates a token, returning its claims.
func (s *TokenService) Verify(token string) (*Claims, error) {
	claims := &Claims{}

	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		// Belt and braces alongside WithValidMethods below: reject anything
		// that is not the HMAC family before the secret is ever handed over.
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: unexpected signing method %v", ErrInvalidToken, t.Header["alg"])
		}
		return s.secret, nil
	},
		jwt.WithValidMethods([]string{signingMethod.Alg()}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if claims.HospitalID == uuid.Nil {
		return nil, fmt.Errorf("%w: no hospital in token", ErrInvalidToken)
	}
	if _, err := claims.StaffID(); err != nil {
		return nil, err
	}
	return claims, nil
}
