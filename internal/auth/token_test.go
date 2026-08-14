package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ThanadolU/hospital-middleware/internal/models"
)

const testSecret = "a-test-secret-that-is-long-enough-to-pass"

func newStaff() models.Staff {
	return models.Staff{ID: uuid.New(), HospitalID: uuid.New(), Username: "somchai"}
}

func newService(t *testing.T) *TokenService {
	t.Helper()

	service, err := NewTokenService(testSecret, time.Hour)
	require.NoError(t, err)
	return service
}

func TestTokenService_RoundTrip(t *testing.T) {
	service := newService(t)
	staff := newStaff()

	token, err := service.Issue(staff)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	claims, err := service.Verify(token)
	require.NoError(t, err)

	staffID, err := claims.StaffID()
	require.NoError(t, err)
	assert.Equal(t, staff.ID, staffID)
	assert.Equal(t, staff.HospitalID, claims.HospitalID, "the hospital scope must survive the round trip")
	assert.Equal(t, staff.Username, claims.Username)
}

// THE algorithm-confusion test. v1's keyfunc returned the signing secret
// without ever checking token.Method, so a token declaring a different
// algorithm was accepted on its own say-so.
func TestTokenService_RejectsWrongSigningMethod(t *testing.T) {
	service := newService(t)
	staff := newStaff()

	t.Run("alg none", func(t *testing.T) {
		claims := Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   staff.ID.String(),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			HospitalID: staff.HospitalID,
		}
		unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).
			SignedString(jwt.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)

		_, err = service.Verify(unsigned)
		assert.ErrorIs(t, err, ErrInvalidToken, "an unsigned token must never be accepted")
	})

	t.Run("different HMAC strength", func(t *testing.T) {
		claims := Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   staff.ID.String(),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			HospitalID: staff.HospitalID,
		}
		other, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString([]byte(testSecret))
		require.NoError(t, err)

		_, err = service.Verify(other)
		assert.ErrorIs(t, err, ErrInvalidToken, "only the pinned algorithm may be accepted")
	})
}

func TestTokenService_RejectsExpiredToken(t *testing.T) {
	service := newService(t)
	staff := newStaff()

	// Issue as if it were two hours ago, with a one-hour lifetime.
	service.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	token, err := service.Issue(staff)
	require.NoError(t, err)

	service.now = time.Now
	_, err = service.Verify(token)
	assert.ErrorIs(t, err, ErrExpiredToken)
	assert.NotErrorIs(t, err, ErrInvalidToken, "expiry is distinct from a malformed token")
}

func TestTokenService_RejectsTamperedAndMalformedTokens(t *testing.T) {
	service := newService(t)

	valid, err := service.Issue(newStaff())
	require.NoError(t, err)

	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"garbage", "not-a-token"},
		{"two segments", "header.payload"},
		{"tampered signature", valid[:len(valid)-4] + "AAAA"},
		{"tampered payload", "eyJhbGciOiJIUzI1NiJ9." + valid[len(valid)/3:]},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := service.Verify(tc.token)
			assert.Nil(t, claims)
			assert.ErrorIs(t, err, ErrInvalidToken)
		})
	}
}

// A token signed with a different secret must not verify.
func TestTokenService_RejectsForeignSignature(t *testing.T) {
	issuer, err := NewTokenService("another-secret-that-is-also-long-enough", time.Hour)
	require.NoError(t, err)

	foreign, err := issuer.Issue(newStaff())
	require.NoError(t, err)

	_, err = newService(t).Verify(foreign)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

// A token carrying no hospital cannot scope a search, so it is not usable.
func TestTokenService_RejectsTokenWithoutHospital(t *testing.T) {
	service := newService(t)

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.NewString(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		// HospitalID deliberately left zero.
	}
	token, err := jwt.NewWithClaims(signingMethod, claims).SignedString([]byte(testSecret))
	require.NoError(t, err)

	_, err = service.Verify(token)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

// A non-uuid subject must be a 401, not a panic. v1 asserted claim types
// unchecked, so a malformed token crashed the request.
func TestTokenService_RejectsNonUUIDSubjectWithoutPanicking(t *testing.T) {
	service := newService(t)

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "not-a-uuid",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		HospitalID: uuid.New(),
	}
	token, err := jwt.NewWithClaims(signingMethod, claims).SignedString([]byte(testSecret))
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		_, err := service.Verify(token)
		assert.ErrorIs(t, err, ErrInvalidToken)
	})
}

// A token with no expiry must not be accepted as valid forever.
func TestTokenService_RequiresAnExpiry(t *testing.T) {
	service := newService(t)

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: uuid.NewString()},
		HospitalID:       uuid.New(),
	}
	token, err := jwt.NewWithClaims(signingMethod, claims).SignedString([]byte(testSecret))
	require.NoError(t, err)

	_, err = service.Verify(token)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestNewTokenService_RejectsWeakConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		secret string
	}{
		{"empty", ""},
		{"whitespace", "    "},
		{"too short", "supersecretkey"}, // v1's example.env value
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, err := NewTokenService(tc.secret, time.Hour)
			assert.Nil(t, service)
			assert.Error(t, err, "a weak secret must fail at boot, not at first login")
		})
	}
}

func TestTokenService_Issue_RequiresIdentity(t *testing.T) {
	service := newService(t)

	_, err := service.Issue(models.Staff{HospitalID: uuid.New()})
	assert.Error(t, err, "a token with no subject identifies nobody")

	_, err = service.Issue(models.Staff{ID: uuid.New()})
	assert.Error(t, err, "a token with no hospital cannot scope a search")
}
