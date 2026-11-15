// Package fee is the fee of an order (system.html, core/fee). It copies the old bill service
// (service/feeResolver/fee_resolver.go and pkg/helper/fee.go; task 039): the rules are the rows of
// the fees table, and the fee of an order is the rule of the user and the provider, else the rule
// of the user, else the rule of the provider, else the default fee.
//
// The package has no rule of a bill type or a provider: an order task of another bill type uses it
// as it is.
package fee

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/valyala/fastjson"
)

// DefaultFee is the fee of an order with no fees row (bill: feeResolver.DefaultFee).
const DefaultFee int64 = 9000

// The sources of a fee, for Options.OnResolve (a fixed set: metric labels).
const (
	SourceUserProvider = "user_provider" // the row of the user and the provider
	SourceUser         = "user"          // the row of the user (provider_id NULL)
	SourceProvider     = "provider"      // the row of the provider (user_id NULL)
	SourceDefault      = "default"       // no row: DefaultFee
	SourceLookupError  = "lookup_error"  // the read of a row failed: DefaultFee
	SourceCalcError    = "calc_error"    // the row has a config that gives no fee: 0
)

// Row is one row of the fees table that a lookup gives.
type Row struct {
	ID     int64
	Type   string // fees.type: ConfigFixed, ConfigPercent or ConfigStair (another text is a bad config)
	Config []byte // fees.config, the JSON text
}

// Repository reads the fees table. Each Find method gives the newest row (created_at) that is not
// deleted (deleted_at NULL), or nil and no error when there is none.
type Repository interface {
	// UserIDs gives the user_id of every row that has one (bill: GetUserIDs).
	UserIDs(ctx context.Context) ([]string, error)
	// FindUserProvider gives the row of the user and the provider (bill: GetByUserAndProvider).
	FindUserProvider(ctx context.Context, userID string, providerID int64) (*Row, error)
	// FindUser gives the row of the user with no provider (bill: GetByUser).
	FindUser(ctx context.Context, userID string) (*Row, error)
	// FindProvider gives the row of the provider with no user (bill: GetByProvider).
	FindProvider(ctx context.Context, providerID int64) (*Row, error)
}

// Logger writes the errors that Resolve handles itself (the App logger has it).
type Logger interface {
	Error(msg string, kv ...any)
}

// Options configure NewResolver.
type Options struct {
	// Repository is required.
	Repository Repository
	// Logger gets one error line for a failed read of the users with a fee (the lookup goes on with
	// the row of the provider, as bill). Nil: no line.
	Logger Logger
	// OnResolve is called once per Resolve call with one of the Source values. Nil: no call.
	OnResolve func(source string)
}

// Result is the fee of an order: the columns fee, fee_type, fee_config and fee_id of orders.
type Result struct {
	Fee    int64
	Type   string // the applied fee type (TypeFixed, ...), "" for the default fee
	Config []byte // the applied config, nil for the default fee and for a failed calculation
	ID     *int64 // the id of the fees row, nil when no row was used
	Source string // one of the Source values
}

// Resolver gives the fee of an order. It is safe for concurrent use.
type Resolver struct {
	repo      Repository
	log       Logger
	onResolve func(source string)
	parsers   fastjson.ParserPool
}

// NewResolver returns the resolver.
func NewResolver(o Options) (*Resolver, error) {
	if o.Repository == nil {
		return nil, errors.New("fee: repository is required")
	}
	return &Resolver{repo: o.Repository, log: o.Logger, onResolve: o.OnResolve}, nil
}

// Resolve gives the fee of an order of the user on the provider for the amount (bill:
// FeeResolver.Resolve, the same steps):
//  1. A provider ID of 0 gives the default fee.
//  2. When the user has a row (UserIDs; a failed read is logged and counts as no row), the row of
//     the user and the provider, else the row of the user.
//  3. Else the row of the provider.
//  4. No row gives the default fee: DefaultFee, no type, no config, no ID.
//
// The fee of a row is Calculate. Resolve never stops an order: a failed read of a row gives the
// default fee and a failed calculation gives 0 (TypeFixed, the ID of the row), each with an error
// that the caller logs.
// @todo bill caches the row of the fee and the list of users with a fee in Redis
// (caching.provider); Qabz reads the fees table on each order. See docs/this_vs_bill.md, item 39.
func (r *Resolver) Resolve(ctx context.Context, userID string, providerID int64, amount int64) (Result, error) {
	def := Result{Fee: DefaultFee, Source: SourceDefault}
	if r == nil || r.repo == nil {
		return def, errors.New("fee: resolver not created with NewResolver")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if providerID == 0 {
		r.count(SourceDefault)
		return def, nil
	}
	row, source, err := r.find(ctx, userID, providerID)
	if err != nil {
		def.Source = SourceLookupError
		r.count(SourceLookupError)
		return def, fmt.Errorf("fee: find the row of the order: %w", err)
	}
	if row == nil {
		r.count(SourceDefault)
		return def, nil
	}
	id := row.ID
	parser := r.parsers.Get()
	fee, feeType, config, err := Calculate(parser, row.Type, row.Config, amount)
	r.parsers.Put(parser)
	if err != nil {
		r.count(SourceCalcError)
		return Result{Fee: 0, Type: TypeFixed, ID: &id, Source: SourceCalcError}, fmt.Errorf("fee: row %d: %w", id, err)
	}
	r.count(source)
	return Result{Fee: fee, Type: feeType, Config: config, ID: &id, Source: source}, nil
}

// find is the lookup of steps 2 and 3 of Resolve (bill: findFee and userHasFee).
func (r *Resolver) find(ctx context.Context, userID string, providerID int64) (*Row, string, error) {
	if userID != "" && r.userHasFee(ctx, userID) {
		row, err := r.repo.FindUserProvider(ctx, userID, providerID)
		if err != nil {
			return nil, "", err
		}
		if row != nil {
			return row, SourceUserProvider, nil
		}
		row, err = r.repo.FindUser(ctx, userID)
		if err != nil {
			return nil, "", err
		}
		if row != nil {
			return row, SourceUser, nil
		}
	}
	row, err := r.repo.FindProvider(ctx, providerID)
	if err != nil {
		return nil, "", err
	}
	return row, SourceProvider, nil
}

// userHasFee is true when the user has a fees row. A failed read is false, as bill; Qabz logs it
// (bill writes nothing).
func (r *Resolver) userHasFee(ctx context.Context, userID string) bool {
	ids, err := r.repo.UserIDs(ctx)
	if err != nil {
		if ctx.Err() == nil && r.log != nil {
			r.log.Error("fee: read the users with a fee failed", "error", err)
		}
		return false
	}
	return slices.Contains(ids, userID)
}

func (r *Resolver) count(source string) {
	if r.onResolve != nil {
		r.onResolve(source)
	}
}
