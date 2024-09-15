package controller

import (
	"fmt"
	"strings"
)

func genKey(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}
func revertKey(key string) (string, string) {
	split := strings.Split(key, "/")
	return split[0], split[1]
}
