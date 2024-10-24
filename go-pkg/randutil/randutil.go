package randutil

import (
	"math/rand"
)

const (
	alphabetsLowerCase                 = "abcdefghijklmnopqrstuvwxyz"
	alphaLowerNumerics                 = "0123456789abcdefghijklmnopqrstuvwxyz"
	alphaNumericsWithSpecialCharacters = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ?!@#$%^&*()-=_+[]{}';:,.<>"
)

func AlphabetsLowerCase(n int) string {
	return string(randBytes(alphabetsLowerCase, n))
}

func StringAlphaNumeric(n int) string {
	return string(randBytes(alphaLowerNumerics, n))
}

func StringAlphaNumericWithSpecialCharacters(n int) string {
	return string(randBytes(alphaNumericsWithSpecialCharacters, n))
}

func randBytes(pattern string, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = pattern[rand.Intn(len(pattern))]
	}
	return b
}
