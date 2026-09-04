package session

import (
	"crypto/rand"
	"encoding/hex"
)

// codeAlphabet has no I, O, 0 or 1: the code is read aloud and typed on phones.
const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// NewCode returns a 6-character session code.
func NewCode() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	for i := range b {
		b[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	return string(b)
}

// NewToken returns the 32-hex-character remote secret.
func NewToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// ValidCode reports whether s looks like a code (format only).
func ValidCode(s string) bool {
	if len(s) != 6 {
		return false
	}
	for i := 0; i < len(s); i++ {
		found := false
		for j := 0; j < len(codeAlphabet); j++ {
			if s[i] == codeAlphabet[j] {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
