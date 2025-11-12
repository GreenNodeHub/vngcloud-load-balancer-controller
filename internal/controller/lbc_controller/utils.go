package lbc_controller

// Generic function to remove the first occurrence of a matching value from a slice
func removeFisrt[T comparable](slice []T, value T) []T {
	for i, v := range slice {
		if v == value {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}
