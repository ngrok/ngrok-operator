package util

import maps0 "maps"

// MergeMaps merges multiple maps into a single map giving precedence to the last map in the list
func MergeMaps[K comparable, V any](maps ...map[K]V) map[K]V {
	result := make(map[K]V)
	for _, m := range maps {
		maps0.Copy(result, m)
	}
	return result
}
