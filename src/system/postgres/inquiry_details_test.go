package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fastjson"

	"qabz/provider"
)

func fullElectricity() *provider.ElectricityDetails {
	return &provider.ElectricityDetails{Phase: "تک فاز", CompanyName: "co \"q\"", TariffType: "ta", CustomerType: 7, SerialNumber: "se",
		ServiceAddress: "ad\nline", ServicePostCode: "po", CustomerName: "na", City: "ci", Region: "re"}
}

// TestInquiryExtraData: no details is "" (NULL); electricity details are one JSON object with
// bill's keys (city, region) and the fields the client sees; an empty value is not written.
func TestInquiryExtraData(t *testing.T) {
	for name, d := range map[string]*provider.Details{"nil": nil, "no bill type": {}} {
		s, err := inquiryExtraData(d)
		assert.NoError(t, err, name)
		assert.Equal(t, "", s, name)
	}
	s, err := inquiryExtraData(&provider.Details{Electricity: &provider.ElectricityDetails{}})
	assert.NoError(t, err)
	assert.Equal(t, "{}", s, "details with no value are an object, not NULL")

	s, err = inquiryExtraData(&provider.Details{Electricity: &provider.ElectricityDetails{City: "c", Region: "r"}})
	assert.NoError(t, err)
	assert.Equal(t, `{"city":"c","region":"r"}`, s, "what bill writes")

	s, err = inquiryExtraData(&provider.Details{Electricity: fullElectricity()})
	if assert.NoError(t, err) {
		for _, key := range []string{`"city"`, `"region"`, `"phase"`, `"company_name"`, `"tariff_type"`, `"customer_type":7`, `"serial_number"`, `"service_address"`, `"service_post_code"`, `"customer_name"`} {
			assert.Contains(t, s, key)
		}
	}
}

// TestInquiryDetailsRoundTrip: what inquiryExtraData writes, inquiryDetails reads back unchanged
// (the cache hit has the same details as the provider answer), for more than one shape.
func TestInquiryDetailsRoundTrip(t *testing.T) {
	var p fastjson.Parser
	for name, e := range map[string]*provider.ElectricityDetails{
		"full":              fullElectricity(),
		"empty":             {},
		"only bill's keys":  {City: "c", Region: "r"},
		"negative customer": {CustomerType: -2147483648, Phase: "x"},
		"max customer":      {CustomerType: 2147483647},
	} {
		s, err := inquiryExtraData(&provider.Details{Electricity: e})
		if !assert.NoError(t, err, name) {
			continue
		}
		got, err := inquiryDetails(&p, provider.BillTypeElectricity, s)
		if assert.NoError(t, err, name) && assert.NotNil(t, got, name) {
			assert.Equal(t, e, got.Electricity, name)
		}
	}
}

func TestInquiryDetailsReads(t *testing.T) {
	var p fastjson.Parser
	// No details.
	for name, c := range map[string][2]string{
		"NULL":                   {provider.BillTypeElectricity, ""},
		"JSON null":              {provider.BillTypeElectricity, "null"},
		"water":                  {provider.BillTypeWater, `{"city":"c"}`},
		"water, broken JSON":     {provider.BillTypeWater, `{`},
		"an unknown bill type":   {"gas", `{"city":"c"}`},
		"an empty bill type":     {"", `{"city":"c"}`},
		"electricity, uppercase": {"Electricity", `{"city":"c"}`},
	} {
		got, err := inquiryDetails(&p, c[0], c[1])
		assert.NoError(t, err, name)
		assert.Nil(t, got, name)
	}
	// A row that bill wrote: only city and region. Null values and other keys change nothing.
	got, err := inquiryDetails(&p, provider.BillTypeElectricity, `{"city": "c", "region": "r", "phase": null, "customer_type": null, "other": [1], "City": "x"}`)
	if assert.NoError(t, err) && assert.NotNil(t, got) {
		assert.Equal(t, &provider.ElectricityDetails{City: "c", Region: "r"}, got.Electricity)
	}
	got, err = inquiryDetails(&p, provider.BillTypeElectricity, `{}`)
	if assert.NoError(t, err) && assert.NotNil(t, got) {
		assert.Equal(t, &provider.ElectricityDetails{}, got.Electricity)
	}
	// The strings are copies: a second parse with the same parser does not change them.
	first, err := inquiryDetails(&p, provider.BillTypeElectricity, `{"city":"first-city"}`)
	if assert.NoError(t, err) && assert.NotNil(t, first) && assert.NotNil(t, first.Electricity) {
		_, err = inquiryDetails(&p, provider.BillTypeElectricity, `{"city":"other-text"}`)
		assert.NoError(t, err)
		assert.Equal(t, "first-city", first.Electricity.City)
	}
}

func TestInquiryDetailsErrors(t *testing.T) {
	var p fastjson.Parser
	for raw, want := range map[string]string{
		`{`:                               "extra_data:",
		`[]`:                              "extra_data is a JSON array, not an object",
		`"text"`:                          "extra_data is a JSON string, not an object",
		`7`:                               "extra_data is a JSON number, not an object",
		`{"city": 5}`:                     "extra_data city:",
		`{"customer_name": {}}`:           "extra_data customer_name:",
		`{"customer_type": "1"}`:          "extra_data customer_type: a JSON string, not a number",
		`{"customer_type": 1.5}`:          "extra_data customer_type: not a 32-bit integer",
		`{"customer_type": 2147483648}`:   "extra_data customer_type: not a 32-bit integer",
		`{"customer_type": -2147483649}`:  "extra_data customer_type: not a 32-bit integer",
		`{"customer_type": 1, "city": 1}`: "extra_data city:",
	} {
		got, err := inquiryDetails(&p, provider.BillTypeElectricity, raw)
		assert.Nil(t, got, raw)
		assert.ErrorContains(t, err, want, raw)
	}
	_, err := inquiryDetails(nil, provider.BillTypeElectricity, `{}`)
	assert.ErrorContains(t, err, "no parser")
}
