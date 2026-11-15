package order

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"strconv"
	"strings"

	"qabz/provider"
)

// Metadata is the text of a bill type in the order token (bill: helper.GetBillMetaData).
type Metadata struct {
	About          string
	Icon           string
	FeeDescription string
}

// metadata is the Metadata of each bill type, the values of bill (pkg/helper/helper.go). A bill type
// that is not in the map has empty texts, as bill's default case.
var metadata = map[string]Metadata{
	// bill: case entity.Irancell: MtnIcon, AboutIconPhone, FeeDescriptionMobile.
	provider.BillTypeIrancell: {
		About:          "قبض موبایل",
		Icon:           "https://public.780.ir/dl/Operator/logOirancell2.svg",
		FeeDescription: "کارمزد",
	},
}

// MetadataOf gives the Metadata of the bill type.
func MetadataOf(billType string) Metadata { return metadata[billType] }

// Payload is the payload of the order token (bill: entity.InitialOrderEnc). The JSON keys and
// their order are those of bill.
type Payload struct {
	ServiceID uint32            `json:"serviceId"`
	OrderID   string            `json:"orderId"`
	Provider  string            `json:"provider"`
	Price     uint32            `json:"price"`
	UserID    string            `json:"userId"`
	Data      map[string]string `json:"data"`
}

// PhoneData is the data map of the order token of a mobile bill (bill: controller/phone NewOrderV2).
// phone is the phone of the request after the normalization, operator the operator text of the
// request. feeAmount and feeDescription are there only when the fee is above 0.
func PhoneData(o Order, phone, operator string) map[string]string {
	meta := MetadataOf(o.BillType)
	data := map[string]string{
		"type":          o.BillType,
		"phone":         phone,
		"service":       operator,
		"billId":        o.BillID,
		"payId":         o.PayID,
		"payAmount":     strconv.FormatUint(uint64(o.Price), 10),
		"rawAmount":     strconv.FormatInt(o.BasePrice, 10),
		"about":         meta.About,
		"aboutIcon":     meta.Icon,
		"receiptMobile": replace98(phone),
		"invoiceMobile": replace98(phone),
	}
	if o.Fee > 0 {
		data["feeAmount"] = strconv.FormatInt(o.Fee, 10)
		data["feeDescription"] = meta.FeeDescription
	}
	return data
}

// replace98 is bill's helper.Replace98: a text that starts with 98 gets 0 in place of the 98.
func replace98(s string) string {
	if rest, ok := strings.CutPrefix(s, "98"); ok {
		return "0" + rest
	}
	return s
}

// TokenPayload is the payload of the order token of o (bill: entity.InitialOrderEnc as NewOrderV2
// fills it).
func TokenPayload(o Order, serviceID uint32, data map[string]string) Payload {
	return Payload{ServiceID: serviceID, OrderID: o.OrderID, Provider: o.ProviderName, Price: o.Price, UserID: o.UserID, Data: data}
}

// MarshalPayload gives the JSON bytes of p as bill's json.Marshal (encoding/json v1) gives them: the
// struct fields in order, the map keys sorted, the escapes of HTML characters, U+2028 and U+2029,
// and an invalid UTF-8 byte as U+FFFD. Marshalling is not covered by the JSON rule of conventions
// § 1; encoding/json/v2 is used because v1 is forbidden.
func MarshalPayload(p Payload) ([]byte, error) {
	b, err := json.Marshal(p,
		json.Deterministic(true),
		jsontext.EscapeForHTML(true),
		jsontext.EscapeForJS(true),
		jsontext.AllowInvalidUTF8(true),
		json.FormatNilMapAsNull(true),
	)
	if err != nil {
		return nil, fmt.Errorf("order: marshal the token payload: %w", err)
	}
	return b, nil
}
