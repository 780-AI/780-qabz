package helpers

import "strconv"

// The rules here copy the old bill service (pkg/helper/bill_type.go), so clients see no change.

// ValidPayID reports whether id is a valid pay ID (شناسه پرداخت), as bill (IsValidPayId): at least
// 6 characters; every character before the last two is a digit; the character before the last is
// the mod 11 check digit of the digits before it (the weights 2 to 7 from right to left; 0 for a
// remainder of 0 or 1, else 11 minus the remainder). As in bill, the last character (the second
// check digit) is not checked.
func ValidPayID(id string) bool {
	if len(id) < 6 {
		return false
	}
	sum, weight := 0, 2
	for i := len(id) - 3; i >= 0; i-- {
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
	return id[len(id)-2] == byte('0'+check) // check is 0 to 9
}

// PayIDAmount reads the amount in rials from a pay ID, as bill (GetFromPayIdPrice): the characters
// before the last 5 are the amount in thousands of rials. ok is false for a pay ID that is shorter
// than 6, whose amount part is not a number of digits only, or whose amount does not fit uint64.
// bill reads the part with strconv.Atoi (a sign is accepted there); a valid pay ID (ValidPayID)
// has digits only, so the two agree for every valid pay ID.
func PayIDAmount(payID string) (amount uint64, ok bool) {
	if len(payID) < 6 {
		return 0, false
	}
	part := payID[:len(payID)-5]
	for i := 0; i < len(part); i++ {
		if part[i] < '0' || part[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(part, 10, 64)
	if err != nil || n > (1<<64-1)/1000 {
		return 0, false
	}
	return n * 1000, true
}

// Bill types that BillTypeOfBillID returns (bill: entity.BillType).
const (
	BillTypeWater                = "water"
	BillTypeElectricity          = "electricity"
	BillTypeGas                  = "gas"
	BillTypeTel                  = "tel"
	BillTypeMobile               = "mobile"
	BillTypeMobileMidTermRightel = "mobilemidtermRightel"
	BillTypeIrancell             = "irancell"
)

// BillTypeOfBillID reads the bill type from a bill ID, as bill (GetBillType): the character before
// the last one is the type (1 water, 2 electricity, 3 gas, 4 tel, 5 mobile). For a mobile bill the
// three characters before it name the operator: 920 to 930 Rightel, 935 Irancell, 301, 095, 091 and
// 080 to 090 MCI ("mobile"). ok is false for a bill ID shorter than 6, an unknown type character or
// an unknown mobile prefix. It does not check the check digit (ValidBillID does).
func BillTypeOfBillID(billID string) (billType string, ok bool) {
	n := len(billID)
	if n < 6 {
		return "", false
	}
	switch billID[n-2] {
	case '1':
		return BillTypeWater, true
	case '2':
		return BillTypeElectricity, true
	case '3':
		return BillTypeGas, true
	case '4':
		return BillTypeTel, true
	case '5':
	default:
		return "", false
	}
	prefix := billID[n-5 : n-2]
	switch {
	case prefix >= "920" && prefix <= "930" && digits(prefix):
		return BillTypeMobileMidTermRightel, true
	case prefix == "935":
		return BillTypeIrancell, true
	case prefix == "301", prefix == "095", prefix == "091", prefix >= "080" && prefix <= "090" && digits(prefix):
		return BillTypeMobile, true
	}
	return "", false
}

func digits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
