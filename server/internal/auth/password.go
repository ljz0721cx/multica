package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	passwordHashVersion = "v1"
	passwordIterations  = 120000
	passwordSaltBytes   = 16
)

var (
	ErrInvalidPasswordHash = errors.New("invalid password hash")
	ErrPasswordTooShort    = errors.New("password must be at least 8 characters")
	ErrPasswordTooLong     = errors.New("password must be at most 128 characters")
)

// ValidatePasswordStrength enforces a minimal length floor for admin-set passwords.
func ValidatePasswordStrength(password string) error {
	if len(password) < 8 {
		return ErrPasswordTooShort
	}
	if len(password) > 128 {
		return ErrPasswordTooLong
	}
	return nil
}

// HashPassword returns a salted iterative-SHA256 hash string.
// Format: v1$<iterations>$<base64-salt>$<base64-digest>
func HashPassword(password string) (string, error) {
	if err := ValidatePasswordStrength(password); err != nil {
		return "", err
	}
	salt := make([]byte, passwordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	digest := derivePassword(password, salt, passwordIterations)
	return fmt.Sprintf(
		"%s$%d$%s$%s",
		passwordHashVersion,
		passwordIterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(digest),
	), nil
}

// CheckPassword compares a plaintext password with a stored hash.
func CheckPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != passwordHashVersion {
		return false, ErrInvalidPasswordHash
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 1 {
		return false, ErrInvalidPasswordHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false, ErrInvalidPasswordHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false, ErrInvalidPasswordHash
	}
	got := derivePassword(password, salt, iterations)
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func derivePassword(password string, salt []byte, iterations int) []byte {
	sum := sha256.Sum256(append(append([]byte{}, salt...), []byte(password)...))
	digest := sum[:]
	for i := 1; i < iterations; i++ {
		next := sha256.Sum256(append(digest, salt...))
		digest = next[:]
	}
	out := make([]byte, len(digest))
	copy(out, digest)
	return out
}
