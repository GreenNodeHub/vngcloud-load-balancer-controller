package utils

import (
	"crypto/sha256"
	"fmt"
)

func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TrimString(str string, length int) string {
	return str[:MinInt(len(str), length)]
}

// HashString hash a string to a string have 10 char
func HashString(str string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(str)))
}
