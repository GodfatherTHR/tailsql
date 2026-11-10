package dbmanager

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	databaseNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)
	usernamePattern     = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
	passwordAlphabet    = []byte("ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789")
)

// NormalizeName trims and lowercases a requested database slug.
func NormalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// ValidateName enforces a filesystem-safe, URL-safe H2 database name.
func ValidateName(name string) error {
	if !databaseNamePattern.MatchString(name) {
		return errors.New("database name must start with a letter and contain only lowercase letters, digits, and underscores (2-32 characters)")
	}
	return nil
}

// ValidateUsername enforces the H2 username character set used by the generator.
func ValidateUsername(username string) error {
	if !usernamePattern.MatchString(username) {
		return errors.New("username must be 1-64 characters using letters, digits, dot, underscore, or hyphen")
	}
	return nil
}

// ValidatePassword accepts 8-128 printable ASCII characters.
func ValidatePassword(password string) error {
	if len(password) < 8 || len(password) > 128 {
		return errors.New("password must be between 8 and 128 characters")
	}
	for _, r := range password {
		if r < 0x20 || r > 0x7e {
			return errors.New("password must contain printable ASCII characters only")
		}
	}
	return nil
}

// GeneratePassword returns a cryptographically random 16-character alphanumeric
// password, excluding visually ambiguous characters.
func GeneratePassword() (string, error) {
	return randomString(16, passwordAlphabet)
}

// GenerateServerPassword creates the secret used by H2's tcpShutdown tool.
func GenerateServerPassword() (string, error) {
	return randomString(24, passwordAlphabet)
}

func randomString(length int, alphabet []byte) (string, error) {
	if len(alphabet) == 0 {
		return "", errors.New("empty alphabet")
	}
	buf := make([]byte, length)
	max := byte(256 - (256 % len(alphabet)))
	for i := 0; i < length; {
		var b [1]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", fmt.Errorf("generate random value: %w", err)
		}
		if b[0] >= max {
			continue
		}
		buf[i] = alphabet[int(b[0])%len(alphabet)]
		i++
	}
	return string(buf), nil
}

// NewID returns a 128-bit opaque instance identifier.
func NewID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate instance id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}
