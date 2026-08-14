// Package auth holds the credential primitives: password hashing today, token
// issuing and verification alongside it.
package auth

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// HashCost is one above bcrypt.DefaultCost (10). Each increment doubles the
// work an attacker must do per guess, and at 11 a hash still takes well under
// 200ms on commodity hardware — imperceptible on a login path that happens
// once per session, meaningful against an offline attack on a leaked table.
//
// This guards patient records, so the extra factor is worth the milliseconds.
const HashCost = bcrypt.DefaultCost + 1

// bcrypt silently truncates anything past 72 bytes, so a longer password would
// be accepted while only its first 72 bytes were ever checked. Rejecting is
// honest; truncating is not.
const maxPasswordBytes = 72

// MinPasswordLength is a floor, not a policy. The brief specifies no password
// rules, so this only rejects the obviously unusable.
const MinPasswordLength = 8

var (
	// ErrPasswordTooShort and ErrPasswordTooLong are caller errors: the input
	// never reaches bcrypt.
	ErrPasswordTooShort = errors.New("auth: password is too short")
	ErrPasswordTooLong  = fmt.Errorf("auth: password exceeds %d bytes", maxPasswordBytes)

	// ErrPasswordMismatch means the password does not match the hash. It is
	// deliberately indistinguishable from "no such user" at the service layer,
	// so a caller cannot enumerate valid usernames.
	ErrPasswordMismatch = errors.New("auth: password does not match")
)

// HashPassword returns a bcrypt hash of password. The plaintext is never
// stored, logged, or returned.
func HashPassword(password string) (string, error) {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return "", ErrPasswordTooShort
	}
	if len(password) > maxPasswordBytes {
		return "", ErrPasswordTooLong
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), HashCost)
	if err != nil {
		// Deliberately not wrapping the input: an error string must never
		// carry the password.
		return "", fmt.Errorf("auth: hash password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword reports whether password matches hash, returning
// ErrPasswordMismatch when it does not.
//
// bcrypt comparison is constant-time with respect to the hash, so this does not
// leak how much of a password was correct.
func VerifyPassword(hash, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrPasswordMismatch
		}
		return fmt.Errorf("auth: verify password: %w", err)
	}
	return nil
}
