package postgres

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"math"

	"github.com/valyala/fastjson"

	"qabz/provider"
)

// electricityExtra is the extra_data of an electricity inquiry (task 022). bill writes only city and
// region (gateway/inquiry ElectricityDetails: its other fields have json:"-"); Qabz adds the fields
// that the client sees, so saved inquiries answer with the same details as the provider. An empty
// value is not written, as in bill (omitempty), so details with no value are the object {}.
// @todo bill saves only city and region. See docs/this_vs_bill.md, item 23.
type electricityExtra struct {
	City            string `json:"city,omitzero"`
	Region          string `json:"region,omitzero"`
	Phase           string `json:"phase,omitzero"`
	CompanyName     string `json:"company_name,omitzero"`
	TariffType      string `json:"tariff_type,omitzero"`
	CustomerType    int32  `json:"customer_type,omitzero"`
	SerialNumber    string `json:"serial_number,omitzero"`
	ServiceAddress  string `json:"service_address,omitzero"`
	ServicePostCode string `json:"service_post_code,omitzero"`
	CustomerName    string `json:"customer_name,omitzero"`
}

// inquiryExtraData returns the JSON text of the extra_data column for the details of an inquiry,
// or "" (NULL) when there are none. Writing JSON is not covered by conventions § 1.
func inquiryExtraData(d *provider.Details) (string, error) {
	if d == nil || d.Electricity == nil {
		return "", nil
	}
	e := d.Electricity
	b, err := json.Marshal(electricityExtra{
		City: e.City, Region: e.Region, Phase: e.Phase, CompanyName: e.CompanyName, TariffType: e.TariffType,
		CustomerType: e.CustomerType, SerialNumber: e.SerialNumber, ServiceAddress: e.ServiceAddress,
		ServicePostCode: e.ServicePostCode, CustomerName: e.CustomerName,
	})
	if err != nil {
		return "", fmt.Errorf("extra_data: %w", err)
	}
	return string(b), nil
}

// inquiryDetails reads the extra_data of a saved inquiry (raw, its JSON text; "" for NULL) into
// the details of its bill type. It is a hot path (each inquiry that is answered from the database),
// so it reads with fastjson (conventions § 1). A bill type with no details, a NULL and a JSON null
// give nil. For electricity an object gives details: a missing key and a null value keep the zero
// value (a row that bill wrote has only city and region), other keys are ignored. A value of the
// wrong JSON type, or an extra_data that is not an object or null, is an error.
//
// The returned strings are copies: nothing refers to the parser after it goes back to the pool.
func inquiryDetails(p *fastjson.Parser, billType, raw string) (*provider.Details, error) {
	if billType != provider.BillTypeElectricity || raw == "" {
		return nil, nil
	}
	if p == nil {
		return nil, errors.New("extra_data: no parser")
	}
	v, err := p.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("extra_data: %w", err)
	}
	if v == nil {
		return nil, errors.New("extra_data: no value")
	}
	switch v.Type() {
	case fastjson.TypeNull:
		return nil, nil
	case fastjson.TypeObject:
	default:
		return nil, fmt.Errorf("extra_data is a JSON %s, not an object", v.Type())
	}
	o, err := v.Object()
	if err != nil {
		return nil, fmt.Errorf("extra_data: %w", err)
	}
	if o == nil {
		return nil, errors.New("extra_data: no object")
	}
	e := &provider.ElectricityDetails{}
	o.Visit(func(key []byte, val *fastjson.Value) {
		if err != nil || val == nil {
			return
		}
		var dst *string
		switch string(key) {
		case "city":
			dst = &e.City
		case "region":
			dst = &e.Region
		case "phase":
			dst = &e.Phase
		case "company_name":
			dst = &e.CompanyName
		case "tariff_type":
			dst = &e.TariffType
		case "serial_number":
			dst = &e.SerialNumber
		case "service_address":
			dst = &e.ServiceAddress
		case "service_post_code":
			dst = &e.ServicePostCode
		case "customer_name":
			dst = &e.CustomerName
		case "customer_type":
			switch val.Type() {
			case fastjson.TypeNull:
			case fastjson.TypeNumber:
				n, e2 := val.Int64()
				if e2 != nil || n < math.MinInt32 || n > math.MaxInt32 {
					err = fmt.Errorf("extra_data %s: not a 32-bit integer", key)
					return
				}
				e.CustomerType = int32(n)
			default:
				err = fmt.Errorf("extra_data %s: a JSON %s, not a number", key, val.Type())
			}
			return
		default:
			return
		}
		if s, ok, e2 := jsonString(val); e2 != nil {
			err = fmt.Errorf("extra_data %s: %w", key, e2)
		} else if ok {
			*dst = s
		}
	})
	if err != nil {
		return nil, err
	}
	return &provider.Details{Electricity: e}, nil
}
