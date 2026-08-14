package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

const validPassword = "correct-horse-battery"

// Traceability 3.3: the password is never stored in plaintext.
func TestHashPassword_NeverReturnsThePlaintext(t *testing.T) {
	hash, err := HashPassword(validPassword)
	require.NoError(t, err)

	assert.NotEqual(t, validPassword, hash)
	assert.NotContains(t, hash, validPassword)
	assert.True(t, strings.HasPrefix(hash, "$2"), "expected a bcrypt hash, got %q", hash)
}

// Two hashes of the same password must differ, or the salt is not doing its
// job and identical passwords would be visible as identical rows.
func TestHashPassword_IsSalted(t *testing.T) {
	first, err := HashPassword(validPassword)
	require.NoError(t, err)
	second, err := HashPassword(validPassword)
	require.NoError(t, err)

	assert.NotEqual(t, first, second, "identical hashes mean the hash is unsalted")
	assert.NoError(t, VerifyPassword(first, validPassword))
	assert.NoError(t, VerifyPassword(second, validPassword))
}

func TestHashPassword_UsesTheConfiguredCost(t *testing.T) {
	hash, err := HashPassword(validPassword)
	require.NoError(t, err)

	cost, err := bcrypt.Cost([]byte(hash))
	require.NoError(t, err)
	assert.Equal(t, HashCost, cost)
	assert.Greater(t, cost, bcrypt.DefaultCost, "patient data warrants more than the default cost")
}

func TestVerifyPassword(t *testing.T) {
	hash, err := HashPassword(validPassword)
	require.NoError(t, err)

	t.Run("accepts the right password", func(t *testing.T) {
		assert.NoError(t, VerifyPassword(hash, validPassword))
	})

	t.Run("rejects the wrong password", func(t *testing.T) {
		assert.ErrorIs(t, VerifyPassword(hash, "wrong-horse-battery"), ErrPasswordMismatch)
	})

	t.Run("rejects the empty password", func(t *testing.T) {
		assert.ErrorIs(t, VerifyPassword(hash, ""), ErrPasswordMismatch)
	})

	t.Run("rejects a prefix of the right password", func(t *testing.T) {
		assert.ErrorIs(t, VerifyPassword(hash, validPassword[:5]), ErrPasswordMismatch)
	})

	t.Run("rejects a garbage hash without panicking", func(t *testing.T) {
		assert.Error(t, VerifyPassword("not-a-bcrypt-hash", validPassword))
	})
}

func TestHashPassword_RejectsUnusableInput(t *testing.T) {
	t.Run("too short", func(t *testing.T) {
		_, err := HashPassword("short")
		assert.ErrorIs(t, err, ErrPasswordTooShort)
	})

	t.Run("empty", func(t *testing.T) {
		_, err := HashPassword("")
		assert.ErrorIs(t, err, ErrPasswordTooShort)
	})

	// bcrypt truncates past 72 bytes. Accepting a longer password would mean
	// silently ignoring the tail, so two different passwords sharing a 72-byte
	// prefix would both unlock the account.
	t.Run("beyond bcrypt's 72-byte limit", func(t *testing.T) {
		_, err := HashPassword(strings.Repeat("a", maxPasswordBytes+1))
		assert.ErrorIs(t, err, ErrPasswordTooLong)
	})

	t.Run("at exactly the limit", func(t *testing.T) {
		_, err := HashPassword(strings.Repeat("a", maxPasswordBytes))
		assert.NoError(t, err)
	})
}

// An error surfaced to a log or a client must not carry the credential.
// The fixtures are chosen not to appear in the error text for unrelated
// reasons — a password of "short" would make this pass or fail by coincidence.
func TestHashPassword_ErrorsNeverCarryThePassword(t *testing.T) {
	for _, password := range []string{"hunter2", strings.Repeat("z", maxPasswordBytes+1)} {
		_, err := HashPassword(password)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), password)
	}
}
