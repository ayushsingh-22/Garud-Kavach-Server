package helpers

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
)

var emailRegexHelper = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// ValidateEmail returns true if s is a well-formed email address.
func ValidateEmail(s string) bool {
	return emailRegexHelper.MatchString(strings.TrimSpace(s))
}

// ValidatePhone strips all non-numeric characters from s and returns the
// cleaned digit-only string. Returns an empty string if no digits are found.
func ValidatePhone(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ValidateTrimLength trims whitespace from s, then returns an error if the
// result is empty or if its byte length exceeds max. Returns the trimmed
// string on success.
func ValidateTrimLength(s string, max int) (string, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return "", errors.New("field is required")
	}
	if len(t) > max {
		return "", errors.New("field exceeds maximum length")
	}
	return t, nil
}

// ValidateStatus returns true when s is a member of the allowed slice.
func ValidateStatus(s string, allowed []string) bool {
	for _, a := range allowed {
		if s == a {
			return true
		}
	}
	return false
}
