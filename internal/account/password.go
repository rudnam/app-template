package account

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Passwords use PBKDF2-HMAC-SHA256 from the standard library with 600,000
// iterations (the OWASP recommendation for this algorithm). The stored format
// names its parameters, so the cost can be raised later:
//
//	pbkdf2-sha256$<iterations>$<salt, base64>$<key, base64>
const (
	iterations = 600_000
	saltLen    = 16
	keyLen     = 32
	scheme     = "pbkdf2-sha256"

	MinPasswordLen = 10
	MaxPasswordLen = 256 // bounds the hashing cost of one request
)

var b64 = base64.RawStdEncoding

// HashPassword returns an encoded hash of password with a fresh random salt.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, keyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s$%d$%s$%s", scheme, iterations, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// CheckPassword reports whether password matches the encoded hash, in time
// that does not depend on how many bytes match.
func CheckPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != scheme {
		return false, errors.New("unrecognized password hash format")
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false, errors.New("bad iteration count in password hash")
	}
	salt, err := b64.DecodeString(parts[2])
	if err != nil {
		return false, err
	}
	want, err := b64.DecodeString(parts[3])
	if err != nil {
		return false, err
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// dummyHash is checked when a sign-in names an unknown email, so a failed
// sign-in takes as long whether or not the account exists.
var dummyHash, _ = HashPassword("timing-equalizer-not-a-real-password")

func burnTime(password string) { _, _ = CheckPassword(dummyHash, password) }

// newSessionToken returns a random token for the cookie and its SHA-256 hash
// for the database. Only the hash is stored.
func newSessionToken() (token string, hash []byte, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, hashToken(token), nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
