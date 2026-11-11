package inquiry

import (
	"errors"
	"fmt"

	"qabz/common/helpers"
)

// ResultRaw is the Result of a raw inquiry, for Options.OnInquiry: answered from the bill ID and
// the pay ID of the request, with no provider and no saved inquiry.
const ResultRaw = "raw"

var (
	// ErrInvalidBillID is the error of a raw inquiry whose bill ID is not valid (bill:
	// raw.ErrInvalidBillID). It wraps ErrInvalid.
	ErrInvalidBillID = fmt.Errorf("%w: bill id", ErrInvalid)
	// ErrInvalidPayID is the error of a raw inquiry whose pay ID is not valid, or whose amount
	// cannot be read (bill: raw.ErrInvalidPayID). It wraps ErrInvalid.
	ErrInvalidPayID = fmt.Errorf("%w: pay id", ErrInvalid)
	// ErrUnknownBillType is the error of a raw inquiry whose bill ID names no known bill type
	// (bill: raw.ErrUnknownBillType). It wraps ErrInvalid.
	ErrUnknownBillType = fmt.Errorf("%w: bill type of the bill id", ErrInvalid)
)

// RawResult is the answer of RawInquiry.
type RawResult struct {
	BillID   string
	PayID    string
	Amount   uint64 // rials, read from the pay ID
	BillType string // read from the bill ID (helpers.BillTypeOfBillID); not the bill type of the endpoint
}

// RawInquiry answers an inquiry from the bill ID and the pay ID of the request, with no provider,
// no saved inquiry and no row (bill: raw.Service.Inquiry). endpointBillType is the bill type of the
// endpoint that got the request (for example provider.BillTypeGas); it is only the bill_type label
// of the metric. The steps and their order are those of bill:
//  1. The bill ID is valid (helpers.ValidBillID), else ErrInvalidBillID.
//  2. The pay ID is valid (helpers.ValidPayID), else ErrInvalidPayID.
//  3. The amount is read from the pay ID (helpers.PayIDAmount), else ErrInvalidPayID.
//  4. The bill type is read from the bill ID (helpers.BillTypeOfBillID), else ErrUnknownBillType.
//
// As in bill, the bill type of the bill ID does not have to be the one of the endpoint. A success
// calls Options.OnInquiry with endpointBillType, NoProviderLabel and ResultRaw; a request that
// breaks a rule is not counted (as in Inquire).
func (s *Service) RawInquiry(endpointBillType, billID, payID string) (RawResult, error) {
	if s == nil {
		return RawResult{}, errors.New("inquiry: service not created with NewService")
	}
	if !helpers.ValidBillID(billID) {
		return RawResult{}, ErrInvalidBillID
	}
	if !helpers.ValidPayID(payID) {
		return RawResult{}, ErrInvalidPayID
	}
	amount, ok := helpers.PayIDAmount(payID)
	if !ok {
		return RawResult{}, ErrInvalidPayID
	}
	billType, ok := helpers.BillTypeOfBillID(billID)
	if !ok {
		return RawResult{}, ErrUnknownBillType
	}
	s.countInquiry(endpointBillType, NoProviderLabel, ResultRaw)
	return RawResult{BillID: billID, PayID: payID, Amount: amount, BillType: billType}, nil
}
