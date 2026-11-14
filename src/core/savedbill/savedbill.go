// Package savedbill is My Bills: the bills that a user saves to inquire and pay them later. The
// rows are in the bills table of the bill database. The rules copy the old bill service
// (controller/myBill, domain/repository/mybill), so clients see no change.
package savedbill

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"qabz/common/helpers"
)

// Bill types that a user can save (InsertBillRequest.TypeEnum).
const (
	TypeGas         = "gas"
	TypeWater       = "water"
	TypeTel         = "tel"
	TypeMobile      = "mobile"
	TypeElectricity = "electricity"
)

// StatusEnable is bills.status of a saved bill that the user can see (bill entity.ENABLE).
const StatusEnable = "enable"

// StatusDisable is bills.status of a bill that the user deleted (bill entity.DISABLE). The row
// stays in the table.
const StatusDisable = "disable"

// OperatorMCI is the operator of a mobile bill saved with no operator (bill entity.MCIOPERATOR).
const OperatorMCI = "mci"

// defaultNames are the names of a bill saved with no name (bill entity.billNames).
var defaultNames = map[string]string{
	TypeElectricity: "قبض برق",
	TypeGas:         "قبض گاز",
	TypeWater:       "قبض آب",
	TypeTel:         "قبض تلفن",
	TypeMobile:      "قبض همراه اول",
}

// ErrInvalid is the error of a request that breaks a rule. Test it with errors.Is.
var ErrInvalid = errors.New("savedbill: invalid request")

// ErrNotFound is the error of Repository.Get when no row has the id.
var ErrNotFound = errors.New("savedbill: bill not found")

// Bill is one row of the bills table. An empty string is a NULL column.
type Bill struct {
	ID     string // id, a UUID
	UserID string // user_id, a UUID
	Name   string
	Type   string
	BillID string // bill_id, as the client sent it
	// Data is the inquiry ID: the participate code or bill ID of a gas bill, the normalized phone
	// of a mobile or tel bill, the bill ID of the other types.
	Data string
	// Operator is json_data.Operator.
	Operator string
	// IsPinned is json_data.IsPinned. Only List reads it.
	IsPinned bool
	// LastOrderDate is last_order_date. Only List reads it.
	LastOrderDate string
	Status        string
}

// InsertRequest is a bill that a user saves.
type InsertRequest struct {
	Type            string
	BillID          string
	Name            string
	Operator        string
	Phone           string
	ParticipateCode string
}

// Repository reads and writes the bills table.
type Repository interface {
	// Insert saves b, unless the user (b.UserID) already has an enabled bill with the same bill_id
	// as b.BillID or the same data as b.Data, of any type. It returns the id of the new row
	// (created true) or of the existing row (created false).
	Insert(ctx context.Context, b Bill) (id string, created bool, err error)
	// Get returns the row with the id, or ErrNotFound.
	Get(ctx context.Context, id string) (Bill, error)
	// List returns the enabled bills of the user, newest first (bill: GetAll in
	// domain/repository/mybill). A user with no bills gives an empty list and no error.
	List(ctx context.Context, userID string) ([]Bill, error)
	// Disable sets status to StatusDisable on the bill with the id when the bill belongs to the user
	// (bill: Delete in domain/repository/mybill). The id and the user go to the database as sent.
	// It reports whether a bill of the user has the id (found; the bill may have been disabled
	// already) and that bill's type ("" for a NULL type or no bill); no such bill is not an error.
	Disable(ctx context.Context, userID, id string) (billType string, found bool, err error)
}

// Store applies the My Bills rules and saves through a Repository.
type Store struct{ repo Repository }

// NewStore returns a Store that saves through repo.
func NewStore(repo Repository) (*Store, error) {
	if repo == nil {
		return nil, errors.New("savedbill: repository is required")
	}
	return &Store{repo: repo}, nil
}

// Insert checks req and saves it as a bill of userID. It returns the saved bill (with its id, the
// default name and the default operator applied) and whether a new row was made. A bill that the
// user already has is not saved again: Insert returns the existing id and created false, and the
// other fields come from req, as in bill. A request that breaks a rule gives an error that wraps
// ErrInvalid, and nothing is written.
func (s *Store) Insert(ctx context.Context, userID string, req InsertRequest) (Bill, bool, error) {
	if s == nil || s.repo == nil {
		return Bill{}, false, errors.New("savedbill: store not created with NewStore")
	}
	if userID == "" {
		return Bill{}, false, fmt.Errorf("%w: no user", ErrInvalid)
	}
	data, err := inquiryID(req)
	if err != nil {
		return Bill{}, false, err
	}
	b := Bill{
		UserID:   userID,
		Name:     req.Name,
		Type:     req.Type,
		BillID:   req.BillID,
		Data:     data,
		Operator: req.Operator,
		Status:   StatusEnable,
	}
	if b.Name == "" {
		b.Name = defaultNames[b.Type]
	}
	if b.Type == TypeMobile && b.Operator == "" {
		b.Operator = OperatorMCI
	}
	id, created, err := s.repo.Insert(ctx, b)
	if err != nil {
		return Bill{}, false, err
	}
	if id == "" {
		return Bill{}, false, errors.New("savedbill: repository returned no id")
	}
	b.ID = id
	return b, created, nil
}

// List returns the saved bills of userID that the user can see (status enable), newest first.
func (s *Store) List(ctx context.Context, userID string) ([]Bill, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("savedbill: store not created with NewStore")
	}
	if userID == "" {
		return nil, fmt.Errorf("%w: no user", ErrInvalid)
	}
	return s.repo.List(ctx, userID)
}

// Delete deletes the bill with the id when it belongs to userID: the row stays, with status
// disable, so List no longer returns it (bill: Delete in controller/myBill and service/myBill).
// The id is checked with uuid.Parse of github.com/google/uuid, the check and the version of bill,
// and goes to the repository as sent, not in the parsed form. An id that is not a bill of the
// user changes nothing and is not an error, as in bill; a bill that is already disabled stays
// disabled. found reports whether a bill of the user has the id, and billType is its type ("" for
// a NULL type or no bill). No user or an id that does not parse gives an error that wraps
// ErrInvalid, and nothing is written. The standard library uuid package is deliberately not used:
// its Parse differs from bill's github.com/google/uuid v1.6.0 (upper-case URN:UUID:, the braces).
func (s *Store) Delete(ctx context.Context, userID, id string) (billType string, found bool, err error) {
	if s == nil || s.repo == nil {
		return "", false, errors.New("savedbill: store not created with NewStore")
	}
	if userID == "" {
		return "", false, fmt.Errorf("%w: no user", ErrInvalid)
	}
	if _, err := uuid.Parse(id); err != nil {
		return "", false, fmt.Errorf("%w: bad id: %w", ErrInvalid, err)
	}
	return s.repo.Disable(ctx, userID, id)
}

// inquiryID checks req by its type and returns the value of the data column (bill
// insertRequestValidator).
func inquiryID(req InsertRequest) (string, error) {
	switch req.Type {
	case TypeGas:
		if req.ParticipateCode != "" {
			return req.ParticipateCode, nil // bill does not check the participate code
		}
		if !helpers.ValidBillID(req.BillID) {
			return "", fmt.Errorf("%w: bad bill id", ErrInvalid)
		}
		return req.BillID, nil
	case TypeMobile, TypeTel:
		phone := req.Phone
		if phone == "" {
			phone = req.BillID // bill: with no phone, the bill ID is the phone number
		}
		valid := helpers.ValidTel
		if req.Type == TypeMobile {
			valid = helpers.ValidMobile
			// Task 038: a mobile number is valid in every common form ("+98", "0098", separators,
			// Persian or Arabic digits). @todo bill has only NormalizePhoneNumber here. See
			// docs/this_vs_bill.md, item 35.
			if m := helpers.NormalizeMobile(phone); m != "" {
				return m, nil
			}
		}
		// bill's rule: the landline, and each mobile input that bill accepts and NormalizeMobile
		// does not (eleven bytes whose first byte is not "0").
		phone = helpers.NormalizePhone(phone)
		if !valid(phone) {
			return "", fmt.Errorf("%w: bad %s number", ErrInvalid, req.Type)
		}
		return phone, nil
	case TypeWater, TypeElectricity:
		if !helpers.ValidBillID(req.BillID) {
			return "", fmt.Errorf("%w: bad bill id", ErrInvalid)
		}
		return req.BillID, nil
	default:
		return "", fmt.Errorf("%w: bad type %q", ErrInvalid, req.Type)
	}
}
