package util

import (
	"fmt"
	"strings"
)

// ParseHelmDictionary parses a string in the format of a Helm dictionary and returns a map
// - Keys and Values must be strings
// - Format must be key=val,key=val
func ParseHelmDictionary(dict string) (map[string]string, error) {
	final := make(map[string]string)

	if len(dict) == 0 {
		return final, nil
	}

	dict = strings.TrimSpace(dict)
	dict = strings.TrimSuffix(dict, ",")

	pairs := strings.Split(dict, ",")

	if len(pairs) == 0 {
		return nil, fmt.Errorf("invalid metadata dictionary: %q", dict)
	}

	for _, pair := range pairs {
		kv := strings.Split(pair, "=")
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid metadata pair: %q", pair)
		}

		final[kv[0]] = kv[1]
	}

	return final, nil
}
