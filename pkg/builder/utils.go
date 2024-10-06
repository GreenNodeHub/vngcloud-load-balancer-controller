package builder

import (
	"crypto/sha256"
	"fmt"
	"strings"
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

func StringListToString(s []string) string {
	return strings.Join(s, ",")
}

func PointerOf[T any](t T) *T {
	return &t
}
