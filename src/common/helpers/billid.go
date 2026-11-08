// Package helpers has small checks that more than one package needs: bill IDs and phone numbers.
// The rules copy the old bill service (pkg/helper and pkg/validator), so clients see no change.
package helpers

// ValidBillID reports whether id is a valid bill ID (شناسه قبض): 6 to 13 digits, and the last digit
// is the check digit of the others. The check digit is the bill (pkg/helper.IsValidBillId) mod 11
// rule: the digits before it, from right to left, are multiplied by 2, 3, ..., 7, 2, 3, ...; the
// sum mod 11 gives r; the check digit is 0 for r <= 1, else 11 - r.
func ValidBillID(id string) bool {
	if len(id) < 6 || len(id) > 13 {
		return false
	}
	sum, weight := 0, 2
	for i := len(id) - 2; i >= 0; i-- {
		d := id[i]
		if d < '0' || d > '9' {
			return false
		}
		sum += int(d-'0') * weight
		weight++
		if weight > 7 {
			weight = 2
		}
	}
	check := 0
	if r := sum % 11; r > 1 {
		check = 11 - r
	}
	return id[len(id)-1] == byte('0'+check) // check is 0 to 9: r 2..10 gives 9..1
}
