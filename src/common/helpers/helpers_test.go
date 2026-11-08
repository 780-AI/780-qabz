package helpers

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidBillID(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"123455", true},          // 6 digits (the shortest)
		{"1234567890125", true},   // 13 digits (the longest)
		{"987654", true},          // check digit 4 (computed with bill's code)
		{"500003", true},          // check digit 3
		{"100030", true},          // remainder 1: check 0
		{"100080", true},          // remainder 0: check 0
		{"000000", true},          // all zero
		{"123456", false},         // wrong check digit
		{"100031", false},         // remainder 1 with check 1
		{"12345", false},          // too short
		{"12345678901250", false}, // too long
		{"12a455", false},         // a letter
		{"1234+5", false},         // a sign
		{"12345a", false},         // a letter as the check digit
		{"۱۲۳۴۵۵", false},         // Persian digits are not accepted (bill: the same)
		{"", false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, ValidBillID(c.id), c.id)
	}
}

// TestValidBillIDMatchesBill compares ValidBillID with a direct copy of bill's
// pkg/helper.IsValidBillId for every check digit of many bases and lengths.
func TestValidBillIDMatchesBill(t *testing.T) {
	bases := []string{"12345", "00000", "99999", "10003", "10008", "123456789012", "5555555", "8642097"}
	for n := 10000; n < 10400; n++ {
		bases = append(bases, strconv.Itoa(n))
	}
	valid := 0
	for _, b := range bases {
		for d := 0; d <= 9; d++ {
			id := b + strconv.Itoa(d)
			want := billIsValidBillID(id)
			assert.Equal(t, want, ValidBillID(id), id)
			if want {
				valid++
			}
		}
	}
	assert.Equal(t, len(bases), valid, "each base has exactly one check digit")
}

// billIsValidBillID is bill's pkg/helper.IsValidBillId, copied as it is (the reference).
func billIsValidBillID(id string) bool {
	if len(id) < 6 || len(id) > 13 {
		return false
	}
	var digit int
	var sum int64
	digit = 2
	for i := 0; i < len(id)-1; i++ {
		item, err := strconv.Atoi(string(id[len(id)-1-i-1]))
		if err != nil {
			return false
		}
		if digit > 7 {
			digit = 2
		}
		sum += int64(item * digit)
		digit++
	}
	r := sum % 11
	checkSum := 0
	if r > 1 {
		checkSum = 11 - int(r)
	}
	return string(id[len(id)-1]) == strconv.Itoa(checkSum)
}

func TestNormalizePhone(t *testing.T) {
	cases := []struct{ in, want string }{
		{"09121234567", "989121234567"},  // 11 bytes: the 0 goes
		{"9121234567", "989121234567"},   // 10 bytes: 98 is added
		{"989121234567", "989121234567"}, // already 98
		{"02112345678", "982112345678"},  // landline, 11 bytes
		{"2112345678", "982112345678"},   // landline, 10 bytes
		{"98", "98"},
		{"123", "123"}, // shorter than 10: not changed
		{"", ""},
		{"+989121234567", "+989121234567"}, // 13 bytes: not changed
		{"00989121234567", "00989121234567"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, NormalizePhone(c.in), c.in)
	}
}

func TestValidMobile(t *testing.T) {
	assert.True(t, ValidMobile("989121234567"))
	assert.True(t, ValidMobile("989000000000"))
	for _, s := range []string{"", "09121234567", "98912123456", "9891212345678", "988121234567", "98912123456a", "+989121234567", "۹۸۹۱۲۱۲۳۴۵۶۷"} {
		assert.False(t, ValidMobile(s), s)
	}
}

func TestValidTel(t *testing.T) {
	assert.True(t, ValidTel("982112345678"))
	assert.True(t, ValidTel("989121234567")) // a mobile number is also 98 and 10 digits (bill: the same)
	for _, s := range []string{"", "02112345678", "98211234567", "9821123456789", "972112345678", "98211234567a"} {
		assert.False(t, ValidTel(s), s)
	}
}
