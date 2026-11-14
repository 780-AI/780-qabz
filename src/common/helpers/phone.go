package helpers

import (
	"regexp"
	"strings"
	"unicode"
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

// maxMobileInput is the longest input, in bytes, that NormalizeMobile reads. The longest form of the
// task 038 table ("+98 937 396 9041" in Persian digits with separators) is far below it.
const maxMobileInput = 64

// NormalizeMobile converts a mobile number in any common form to the 98 form ("989" and nine
// digits), the form that ValidMobile checks (task 038). It returns "" when the input is not a
// mobile number, so ValidMobile(NormalizeMobile(s)) is the check of an incoming mobile number.
//
// The rule, in this order:
//  1. Each Persian digit (U+06F0 to U+06F9) and Arabic digit (U+0660 to U+0669) becomes its ASCII
//     digit.
//  2. Separators are dropped: white space (unicode.IsSpace), "-", "(" and ")".
//  3. One "+" is accepted, only before the first digit ("+98..."). Any other character makes the
//     input invalid.
//  4. The prefix, by the number of digits: "+98" or "0098" and ten digits; "98" and ten digits
//     (twelve digits, no "+"); "0" and ten digits; ten digits. The ten digits must start with "9".
//
// It is not bill's rule: bill's NormalizePhoneNumber is NormalizePhone, which also changes a
// landline. NormalizeMobile never returns a landline.
func NormalizeMobile(s string) string {
	if s == "" || len(s) > maxMobileInput {
		return ""
	}
	var buf [14]byte // the longest valid digit string: "0098" and ten digits
	n := 0
	plus := false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= '\u06f0' && r <= '\u06f9':
			r = r - '\u06f0' + '0'
		case r >= '\u0660' && r <= '\u0669':
			r = r - '\u0660' + '0'
		case r == '-' || r == '(' || r == ')' || unicode.IsSpace(r):
			continue
		case r == '+' && n == 0 && !plus:
			plus = true
			continue
		default:
			return ""
		}
		if n == len(buf) {
			return ""
		}
		buf[n] = byte(r)
		n++
	}
	d := string(buf[:n])
	switch {
	case plus:
		if n != 12 || !strings.HasPrefix(d, "98") {
			return ""
		}
		d = d[2:]
	case n == 14 && strings.HasPrefix(d, "0098"):
		d = d[4:]
	case n == 12 && strings.HasPrefix(d, "98"):
		d = d[2:]
	case n == 11 && d[0] == '0':
		d = d[1:]
	case n == 10:
	default:
		return ""
	}
	if d[0] != '9' {
		return ""
	}
	return "98" + d
}

// ValidMobile reports whether s is a mobile number in the 98 form (989 and 9 more digits).
func ValidMobile(s string) bool { return mobileRe.MatchString(s) }

// ValidTel reports whether s is a landline number in the 98 form (98 and 10 more digits).
func ValidTel(s string) bool { return telRe.MatchString(s) }
