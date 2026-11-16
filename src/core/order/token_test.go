package order

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func tokenOrder(fee int64) Order {
	return Order{OrderID: "11111111-2222-3333-4444-555555555555", UserID: testUser, BillType: "irancell", PayID: "irancell",
		Price: uint32(450000 + fee), BasePrice: 450000, Fee: fee, ProviderName: "irancell"}
}

// TestPhoneData: the data map of bill's NewOrderV2, with and without a fee.
func TestPhoneData(t *testing.T) {
	base := map[string]string{
		"type": "irancell", "phone": testPhone, "service": "irancell", "billId": "", "payId": "irancell",
		"payAmount": "459000", "rawAmount": "450000", "about": "قبض موبایل",
		"aboutIcon":     "https://public.780.ir/dl/Operator/logOirancell2.svg",
		"receiptMobile": "09350000391", "invoiceMobile": "09350000391",
		"feeAmount": "9000", "feeDescription": "کارمزد",
	}
	assert.Equal(t, base, PhoneData(tokenOrder(9000), testPhone, "irancell"))

	noFee := PhoneData(tokenOrder(0), testPhone, "irancell")
	assert.NotContains(t, noFee, "feeAmount")
	assert.NotContains(t, noFee, "feeDescription")
	assert.Equal(t, "450000", noFee["payAmount"])
	assert.Len(t, noFee, 11)
}

func TestReplace98AndMetadata(t *testing.T) {
	for in, want := range map[string]string{"989350000391": "09350000391", "09350000391": "09350000391", "98": "0", "": "", "1989": "1989"} {
		assert.Equal(t, want, replace98(in), in)
	}
	assert.Equal(t, Metadata{}, MetadataOf("gas"), "bill's default case: empty texts")
}

// TestMarshalPayload: the bytes of bill's json.Marshal of InitialOrderEnc: the fields in order,
// the map keys sorted, the HTML characters escaped.
func TestMarshalPayload(t *testing.T) {
	p := TokenPayload(tokenOrder(9000), 105, map[string]string{"z": "<&>", "a": "x y", "m": "قبض"})
	b, err := MarshalPayload(p)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "{\"serviceId\":105,\"orderId\":\"11111111-2222-3333-4444-555555555555\",\"provider\":\"irancell\",\"price\":459000,"+
		"\"userId\":\""+testUser+"\",\"data\":{\"a\":\"x\\u2028y\",\"m\":\"قبض\",\"z\":\"\\u003c\\u0026\\u003e\"}}", string(b))

	b, err = MarshalPayload(Payload{})
	assert.NoError(t, err)
	assert.Equal(t, `{"serviceId":0,"orderId":"","provider":"","price":0,"userId":"","data":null}`, string(b), "a nil map is null, as v1")
}
