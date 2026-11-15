package fee

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/valyala/fastjson"
)

// The config types of a fees row (fees.type; bill: entity.FeeConfigType).
const (
	ConfigFixed   = "fixed"
	ConfigPercent = "percent"
	ConfigStair   = "stair"
)

// The applied fee types of an order (orders.fee_type; bill: entity.FeeType).
const (
	TypeFixed        = "fixed"
	TypePercent      = "percent"
	TypeStairFixed   = "stair_fixed"
	TypeStairPercent = "stair_percent"
)

// stairStep is one step of a stair config (bill: helper.StairFee).
type stairStep struct {
	Min, Max        int64
	Type            string // the rule of the step: TypeFixed or TypePercent
	Amount, Percent int64
}

// feeConfig is the config of a fees row after the parse (bill: helper.BillFeeConfig). Only the
// part of the config type is set.
type feeConfig struct {
	stair   []stairStep
	fixed   *int64 // the amount
	percent *int64 // the percent
}

// Calculate gives the fee of a fees row for the amount (bill: helper.GetFee, the same steps and
// errors):
//  1. The config is parsed for the config type (parseConfig). A bad config is an error.
//  2. fixed: the amount of the config. percent: amount × percent / 100 (integer division, as Go
//     int64, so it rounds toward zero). stair: the first step, in the order of the list, with
//     min ≤ amount ≤ max; its own type decides fixed or percent. No step for the amount is an
//     error.
//
// It returns the fee, the applied fee type (TypeFixed, TypePercent, TypeStairFixed,
// TypeStairPercent) and the applied config: the JSON of the part that was used ({"amount":N},
// {"percent":P}, or the one step of a stair). On an error it returns 0, TypeFixed and a nil config,
// as bill.
func Calculate(p *fastjson.Parser, configType string, config []byte, amount int64) (int64, string, []byte, error) {
	cfg, err := parseConfig(p, configType, config)
	if err != nil {
		return 0, TypeFixed, nil, err
	}
	switch configType {
	case ConfigFixed:
		return *cfg.fixed, TypeFixed, appliedAmount(*cfg.fixed), nil
	case ConfigPercent:
		return amount * *cfg.percent / 100, TypePercent, appliedPercent(*cfg.percent), nil
	default: // ConfigStair: parseConfig accepts no other type
		for _, s := range cfg.stair {
			if amount < s.Min || amount > s.Max {
				continue
			}
			if s.Type == TypeFixed {
				return s.Amount, TypeStairFixed, appliedStep(s), nil
			}
			return amount * s.Percent / 100, TypeStairPercent, appliedStep(s), nil
		}
		return 0, TypeFixed, nil, fmt.Errorf("fee: no stair step for the amount %d", amount)
	}
}

var (
	errEmptyConfig   = errors.New("fee: config is empty")
	errInvalidJSON   = errors.New("fee: invalid config json")
	errNoConfigType  = errors.New("fee: config type is missing")
	errNoStairConfig = errors.New("fee: stair config is missing")
)

// parseConfig reads the config of a fees row as bill does (helper.BillFeeConfigFromFee and
// unmarshalBillFeeConfig, with the json.Decoder of encoding/json v1 and DisallowUnknownFields):
//   - An empty config ("", "{}", "[]", "null", after the white space is removed) is an error.
//   - The config must be valid JSON, and the config type must not be empty.
//   - fixed: an object whose only key is amount (an integer, not null). percent: the same with the
//     key percent. stair: an array of objects with the keys min, max, type, amount and percent; a
//     key that is not there, a null value and a null step keep the zero value.
//   - A key matches without case (as encoding/json v1); a key that matches no field is an error.
//     When a key is there twice, the last one wins; a null value sets a pointer field back to nil.
//   - An integer is the JSON number text read with strconv.ParseInt (as encoding/json v1 for
//     int64): 1.0, 1e3 and a number out of the int64 range are errors.
//   - A stair step has min ≤ max and the type fixed or percent; an empty stair is an error.
//   - Any other config type is an error.
//
// It is on the hot path of an order (conventions § 1): it uses fastjson with the parser p.
func parseConfig(p *fastjson.Parser, configType string, config []byte) (feeConfig, error) {
	trimmed := bytes.TrimSpace(config)
	switch string(trimmed) {
	case "", "{}", "[]", "null":
		return feeConfig{}, errEmptyConfig
	}
	if fastjson.ValidateBytes(trimmed) != nil {
		return feeConfig{}, errInvalidJSON
	}
	if configType == "" {
		return feeConfig{}, errNoConfigType
	}
	if p == nil {
		return feeConfig{}, errors.New("fee: no json parser")
	}
	v, err := p.ParseBytes(trimmed)
	if err != nil || v == nil {
		return feeConfig{}, errInvalidJSON
	}
	var cfg feeConfig
	switch configType {
	case ConfigFixed:
		if cfg.fixed, err = oneInt(v, "amount"); err != nil {
			return feeConfig{}, err
		}
		if cfg.fixed == nil {
			return feeConfig{}, errInvalidJSON
		}
	case ConfigPercent:
		if cfg.percent, err = oneInt(v, "percent"); err != nil {
			return feeConfig{}, err
		}
		if cfg.percent == nil {
			return feeConfig{}, errInvalidJSON
		}
	case ConfigStair:
		if cfg.stair, err = stairSteps(v); err != nil {
			return feeConfig{}, err
		}
		if len(cfg.stair) == 0 {
			return feeConfig{}, errNoStairConfig
		}
		for i, s := range cfg.stair {
			switch {
			case s.Min > s.Max:
				return feeConfig{}, fmt.Errorf("fee: invalid stair range at index %d", i)
			case s.Type == "":
				return feeConfig{}, fmt.Errorf("fee: stair type is missing at index %d", i)
			case s.Type != TypeFixed && s.Type != TypePercent:
				return feeConfig{}, fmt.Errorf("fee: unknown stair type at index %d: %s", i, s.Type)
			}
		}
	default:
		return feeConfig{}, fmt.Errorf("fee: unknown config type: %s", configType)
	}
	return cfg, nil
}

// oneInt reads an object with one integer field (the key name, without case). The result is nil
// when the key is not there or its last value is null.
func oneInt(v *fastjson.Value, name string) (*int64, error) {
	o, err := v.Object()
	if err != nil || o == nil {
		return nil, errInvalidJSON
	}
	var out *int64
	var bad error
	o.Visit(func(key []byte, x *fastjson.Value) {
		if bad != nil {
			return
		}
		if !strings.EqualFold(string(key), name) {
			bad = fmt.Errorf("%w: unknown field %q", errInvalidJSON, key)
			return
		}
		if x.Type() == fastjson.TypeNull {
			out = nil
			return
		}
		n, err := jsonInt(x)
		if err != nil {
			bad = err
			return
		}
		out = &n
	})
	if bad != nil {
		return nil, bad
	}
	return out, nil
}

// stairSteps reads the array of a stair config.
func stairSteps(v *fastjson.Value) ([]stairStep, error) {
	items, err := v.Array()
	if err != nil {
		return nil, errInvalidJSON
	}
	steps := make([]stairStep, len(items))
	for i, it := range items {
		if it == nil || it.Type() == fastjson.TypeNull {
			continue // encoding/json v1 leaves the zero value
		}
		o, err := it.Object()
		if err != nil || o == nil {
			return nil, errInvalidJSON
		}
		var bad error
		s := &steps[i]
		o.Visit(func(key []byte, x *fastjson.Value) {
			if bad != nil {
				return
			}
			bad = setStepField(s, string(key), x)
		})
		if bad != nil {
			return nil, bad
		}
	}
	return steps, nil
}

// setStepField sets the field of the key (without case). A null value keeps the field.
func setStepField(s *stairStep, key string, x *fastjson.Value) error {
	var field *int64
	switch {
	case strings.EqualFold(key, "type"):
		if x.Type() == fastjson.TypeNull {
			return nil
		}
		b, err := x.StringBytes()
		if err != nil {
			return errInvalidJSON
		}
		s.Type = string(b)
		return nil
	case strings.EqualFold(key, "min"):
		field = &s.Min
	case strings.EqualFold(key, "max"):
		field = &s.Max
	case strings.EqualFold(key, "amount"):
		field = &s.Amount
	case strings.EqualFold(key, "percent"):
		field = &s.Percent
	default:
		return fmt.Errorf("%w: unknown field %q", errInvalidJSON, key)
	}
	if x.Type() == fastjson.TypeNull {
		return nil
	}
	n, err := jsonInt(x)
	if err != nil {
		return err
	}
	*field = n
	return nil
}

// jsonInt reads a JSON number as an int64 the way encoding/json v1 does: the number text with
// strconv.ParseInt (no fraction, no exponent, in range).
func jsonInt(x *fastjson.Value) (int64, error) {
	if x.Type() != fastjson.TypeNumber {
		return 0, errInvalidJSON
	}
	n, err := strconv.ParseInt(string(x.MarshalTo(nil)), 10, 64)
	if err != nil {
		return 0, errInvalidJSON
	}
	return n, nil
}

// appliedAmount is the JSON of bill's helper.FixedFee: {"amount":N}.
func appliedAmount(n int64) []byte {
	return []byte(`{"amount":` + strconv.FormatInt(n, 10) + `}`)
}

// appliedPercent is the JSON of bill's helper.PercentFee: {"percent":P}.
func appliedPercent(n int64) []byte {
	return []byte(`{"percent":` + strconv.FormatInt(n, 10) + `}`)
}

// appliedStep is the JSON of bill's helper.StairFee: min and max always; type, amount and percent
// only when they are not the zero value (omitempty). The type is fixed or percent (parseConfig
// checks it), so it needs no escape.
func appliedStep(s stairStep) []byte {
	b := []byte(`{"min":` + strconv.FormatInt(s.Min, 10) + `,"max":` + strconv.FormatInt(s.Max, 10))
	if s.Type != "" {
		b = append(b, `,"type":"`+s.Type+`"`...)
	}
	if s.Amount != 0 {
		b = append(b, `,"amount":`+strconv.FormatInt(s.Amount, 10)...)
	}
	if s.Percent != 0 {
		b = append(b, `,"percent":`+strconv.FormatInt(s.Percent, 10)...)
	}
	return append(b, '}')
}
