package helpers

import (
	"regexp"
	"strings"
)

var (
	// mobileRe is an Iranian mobile number in the 98 form (bill pkg/validator.PhoneNumber).
	mobileRe = regexp.MustCompile(`^989\d{9}$`)
	// telRe is an Iranian landline number in the 98 form (bill pkg/validator.TelephoneNumber).
	telRe = regexp.MustCompile(`^98\d{10}$`)
)

// NormalizePhone converts a phone number to the 98 form, as bill does
// (pkg/helper.NormalizePhoneNumber): a value that starts with "98" or is shorter than 10 bytes is
// not changed; 11 bytes ("021xxxxxxxx") lose the first byte and get "98"; 10 bytes ("21xxxxxxxx")
// get "98". Any other value is not changed. It does not check the digits: use ValidMobile or
// ValidTel on the result.
func NormalizePhone(s string) string {
	switch {
	case strings.HasPrefix(s, "98") || len(s) < 10:
		return s
	case len(s) == 11:
		return "98" + s[1:]
	case len(s) == 10:
		return "98" + s
	default:
		return s
	}
}

// ValidMobile reports whether s is a mobile number in the 98 form (989 and 9 more digits).
func ValidMobile(s string) bool { return mobileRe.MatchString(s) }

// ValidTel reports whether s is a landline number in the 98 form (98 and 10 more digits).
func ValidTel(s string) bool { return telRe.MatchString(s) }
