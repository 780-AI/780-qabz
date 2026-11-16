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
	// bill: case entity.TEL: TelIcon, AboutIconTel, FeeDescriptionTel (task 044).
	provider.BillTypeTel: {
		About:          "قبض تلفن ثابت-پایان دوره",
		Icon:           "https://public.780.ir/dl/Operator/mokhaberat.svg",
		FeeDescription: "تسویه آنی",
	},
	// bill: case entity.WATER: WaterIcon, AboutIconWater, FeeDescriptionWater (task 048).
	provider.BillTypeWater: {
		About:          "قبض آب",
		Icon:           "https://public.780.ir/dl/Operator/AbvaFazelab.svg",
		FeeDescription: "کارمزد",
	},
	// bill: case entity.ELECTRICITY: ElectricityIcon, AboutIconElectricity, FeeDescriptionElectricity
	// (task 052). bill's AboutIconElectricity starts with a space; Qabz keeps it (operator decision D7
	// (a)), so the token has the bytes of bill.
	provider.BillTypeElectricity: {
		About:          " قبض برق",
		Icon:           "https://public.780.ir/dl/Operator/Bargh.svg",
		FeeDescription: "کارمزد",
	},
	// bill: case entity.TELMIDTERM: TelIcon, AboutIconTelMidterm, FeeDescriptionTel (task 044).
	provider.BillTypeTelMidterm: {
		About:          "قبض تلفن ثابت-میان دوره",
		Icon:           "https://public.780.ir/dl/Operator/mokhaberat.svg",
		FeeDescription: "تسویه آنی",
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

// TelData is the data map of the order token of a landline bill (task 044; bill:
// controller/telephone NewOrderV2). phone is the phone of the request after the normalization. When
// the fee is above 0: feeAmount (the fee and the provider fee), feeDescription and hafhashtadFee
// (the fee); when the provider fee is above 0: providerFee. As in bill, feeAmount does not have the
// provider fee when the fee is 0.
func TelData(o Order, phone string) map[string]string {
	meta := MetadataOf(o.BillType)
	data := map[string]string{
		"type":         o.BillType,
		"phone":        phone,
		"billId":       o.BillID,
		"payId":        o.PayID,
		"payAmount":    strconv.FormatUint(uint64(o.Price), 10),
		"rawAmount":    strconv.FormatInt(o.BasePrice, 10),
		"about":        meta.About,
		"aboutIcon":    meta.Icon,
		"invoicePhone": replace98(phone),
		"receiptPhone": replace98(phone),
	}
	if o.Fee > 0 {
		data["feeAmount"] = strconv.FormatUint(uint64(o.Fee)+o.ProviderFee, 10)
		data["feeDescription"] = meta.FeeDescription
		data["hafhashtadFee"] = strconv.FormatInt(o.Fee, 10)
	}
	if o.ProviderFee > 0 {
		data["providerFee"] = strconv.FormatUint(o.ProviderFee, 10)
	}
	return data
}

// UtilityData is the data map of the order token of a water bill (task 048; bill: controller/water
// NewOrderV2) and of an electricity bill (task 052; bill: controller/electricity NewOrderV2): the two
// maps of bill have the same keys, and the texts come from MetadataOf(o.BillType). feeAmount (the fee)
// and feeDescription are there only when the fee is above 0.
func UtilityData(o Order) map[string]string {
	meta := MetadataOf(o.BillType)
	data := map[string]string{
		"payId":         o.PayID,
		"type":          o.BillType,
		"billId":        o.BillID,
		"payAmount":     strconv.FormatUint(uint64(o.Price), 10),
		"rawAmount":     strconv.FormatInt(o.BasePrice, 10),
		"about":         meta.About,
		"aboutIcon":     meta.Icon,
		"invoicePayID":  o.PayID,
		"invoiceBillID": o.BillID,
		"receiptPayID":  o.PayID,
		"receiptBillID": o.BillID,
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
