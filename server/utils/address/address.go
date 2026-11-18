package address

import "strings"

// IsValidEmailAddress checks whether the address is a valid email address
func IsValidEmailAddress(str string) bool {
	ars := strings.Split(str, "@")
	if len(ars) != 2 {
		return false
	}
	return strings.Contains(ars[1], ".")
}
