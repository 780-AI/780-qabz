package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The expected values here were checked against bill's own functions (pkg/helper IsValidPayId,
// GetFromPayIdPrice, GetBillType) with 400 000 random inputs on 2026-11-11 (task 023).

func TestValidPayID(t *testing.T) {
	for id, want := range map[string]bool{
		"3200012345":   true,  // the first check digit (4) is right
		"3200012347":   true,  // the last character is not checked, as in bill
		"320001234a":   true,  // also when it is not a digit, as in bill
		"000000":       true,  // 6 characters (the shortest)
		"4500147":      true,  // 7 characters
		"429496712377": true,  // a long one
		"2500012345":   false, // a wrong check digit
		"3200012355":   false,
		"12345":        false, // too short
		"":             false,
		"32000a2345":   false, // a letter before the check digits
		"+200012345":   false, // a sign
		"32000123a5":   false, // a letter as the check digit
		"۳۲۰۰۰۱۲۳۴۵":   false, // Persian digits are not accepted
	} {
		assert.Equal(t, want, ValidPayID(id), id)
	}
}

func TestPayIDAmount(t *testing.T) {
	cases := []struct {
		id     string
		amount uint64
		ok     bool
	}{
		{"3200012345", 32_000_000, true},
		{"4500147", 45_000, true},
		{"000000", 0, true},
		{"123456", 1_000, true}, // not a valid pay ID; the amount part is still read
		{"429496712377", 4_294_967_000, true},
		{"1844674407370955112345", 18_446_744_073_709_551_000, true}, // the largest amount that fits
		{"1844674407370955212345", 0, false},                         // n * 1000 overflows
		{"99999999999999999999912345", 0, false},                     // the amount part overflows
		{"12345", 0, false},                                          // too short
		{"", 0, false},
		{"a200012345", 0, false},
		{"+200012345", 0, false}, // a sign is not a digit
		{"-200012345", 0, false},
	}
	for _, c := range cases {
		amount, ok := PayIDAmount(c.id)
		assert.Equal(t, c.ok, ok, c.id)
		assert.Equal(t, c.amount, amount, c.id)
	}
}

func TestBillTypeOfBillID(t *testing.T) {
	cases := []struct {
		id   string
		want string
		ok   bool
	}{
		{"123456789018", BillTypeWater, true},
		{"123456789026", BillTypeElectricity, true},
		{"1234567890125", BillTypeElectricity, true},
		{"123456789034", BillTypeGas, true},
		{"123456789042", BillTypeTel, true},
		{"12345692051", BillTypeMobileMidTermRightel, true}, // 920
		{"12345693051", BillTypeMobileMidTermRightel, true}, // 930
		{"12345693554", BillTypeIrancell, true},             // 935
		{"12345630153", BillTypeMobile, true},               // 301
		{"12345609553", BillTypeMobile, true},               // 095
		{"12345609153", BillTypeMobile, true},               // 091
		{"12345609053", BillTypeMobile, true},               // 090
		{"12345608053", BillTypeMobile, true},               // 080
		{"12345608557", BillTypeMobile, true},               // 085
		{"12345693151", "", false},                          // 931: not a known prefix
		{"12345691951", "", false},                          // 919
		{"12345607953", "", false},                          // 079
		{"12345609253", "", false},                          // 092
		{"12345612350", "", false},                          // 123
		{"1234a09053", BillTypeMobile, true},                // as bill: only the type and the prefix are read; ValidBillID rejects the letter
		{"12345a9053", "", false},                           // "a90" is not a prefix
		{"12345089a53", "", false},                          // "89a" is not a prefix
		{"123456789069", "", false},                         // type 6
		{"123456789000", "", false},                         // type 0
		{"1234a6", "", false},                               // a letter as the type
		{"12318", "", false},                                // shorter than 6
		{"", "", false},
		{"000015", BillTypeWater, true}, // 6 characters; the check digit is not checked here
		{"092051", BillTypeMobileMidTermRightel, true},
	}
	for _, c := range cases {
		got, ok := BillTypeOfBillID(c.id)
		assert.Equal(t, c.ok, ok, c.id)
		assert.Equal(t, c.want, got, c.id)
	}
}
