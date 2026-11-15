package karashop

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
	_ "time/tzdata" // the Asia/Tehran zone also where the system has no zone files (a small image)

	"github.com/valyala/fastjson"

	"qabz/provider"
)

// serviceSendPaymentInfo is the Karashop service of a settlement (bill: karashop.SendPaymentInfo).
const serviceSendPaymentInfo = "tci-SendPaymentInfo-1"

// channelName is the channel of every settlement (bill: "IPG_ghabzino").
const channelName = "IPG_ghabzino"

// banks gives the bank name of the provider that was paid (bill: app/inquiryGateway.go
// initKarashopProvider). A provider that is not in the map gives "", as in bill.
var banks = map[string]string{
	"parsian": "parsian",
	"eways":   "dey",
	"eways2":  "dey",
}

// settleBillTypes are the bill types that are settled with Karashop (bill: confirmPayment
// conformable: tel and telmidterm).
var settleBillTypes = map[string]bool{provider.BillTypeTel: true, provider.BillTypeTelMidterm: true}

// SupportsSettle reports whether Settle reports a payment of the bill type: tel and telmidterm.
func SupportsSettle(billType string) bool { return settleBillTypes[billType] }

// providerDownText is bill's text of a provider that does not answer (ProviderInquiryIsDownErrorType).
const providerDownText = "استعلام از سوی تامین کننده موقتا غیر فعال می باشد"

// settleAnswer is one entry of the code tables of a settlement answer (bill:
// confirm_payment_gateway BaseResponse).
type settleAnswer struct {
	code    int32
	status  string
	message string
}

// resultCodes is the table of result.status.code (bill: resultStatusCodeMap). Every entry is failed.
var resultCodes = map[int]settleAnswer{
	1: {http.StatusBadRequest, provider.StatusFailed, "نام متود اشتباه است"},
	2: {http.StatusBadRequest, provider.StatusFailed, "تعاریف متد نامعتبر است"},
	3: {http.StatusBadRequest, provider.StatusFailed, "کد استان در header وارد نشده است"},
	4: {http.StatusBadRequest, provider.StatusFailed, "Province is not allowed"},
	5: {http.StatusBadRequest, provider.StatusFailed, providerDownText},
	6: {http.StatusBadRequest, provider.StatusFailed, "اطلاعات ارسالی تکراریست"},
}

// statusCodes is the table of status.code (bill: statusCodeMap).
var statusCodes = map[int]settleAnswer{
	200: {http.StatusOK, provider.StatusSuccess, "عملیات با موفقیت انجام شد"},
	202: {http.StatusOK, provider.StatusSuccess, "درخواست شما دریافت شد"},
	400: {http.StatusBadRequest, provider.StatusFailed, "خطا در داده‌های ورودی وجود دارد"},
	401: {http.StatusBadRequest, provider.StatusFailed, "نام کاربری یا کلمه عبور نادرست است"},
	403: {http.StatusBadRequest, provider.StatusFailed, "آی‌پی شما قبلاً اعلام شده و مجاز به اتصال نیست"},
	404: {http.StatusBadRequest, provider.StatusFailed, "پاسخی برای درخواست شما یافت نشد"},
	410: {http.StatusBadRequest, provider.StatusFailed, "برای دسترسی به سرویس از HTTPS استفاده نمایید"},
	422: {http.StatusBadRequest, provider.StatusFailed, "خطا در پردازش اطلاعات"},
	429: {http.StatusBadRequest, provider.StatusFailed, "درخواست بیش از حد مجاز است / مجوز استفاده از سرویس صادر نشده است"},
	499: {http.StatusInternalServerError, provider.StatusPending, "در هنگام پردازش، کاربر ارتباط را قطع کرده است"},
	500: {http.StatusInternalServerError, provider.StatusPending, "بروز خطا در انجام عملیات توسط سرور"},
	502: {http.StatusInternalServerError, provider.StatusPending, "خطا در دریافت پاسخ از سرور"},
	503: {http.StatusInternalServerError, provider.StatusPending, "بروز خطا در انجام عملیات توسط فراهم‌کننده سرویس"},
	504: {http.StatusBadRequest, provider.StatusFailed, "سرویس‌دهنده پاسخگو نیست"},
	520: {http.StatusBadRequest, provider.StatusFailed, "خطای ناشناخته از سرویس‌دهنده"},
}

// tehran is the zone of the payment date and time (task 044, design D12: bill uses the zone of its
// process). Iran has had no daylight saving time since 2022; the fixed zone is the fallback only.
var tehran = func() *time.Location {
	if loc, err := time.LoadLocation("Asia/Tehran"); err == nil {
		return loc
	}
	return time.FixedZone("IRST", 3*3600+1800)
}()

// Settle reports a paid landline bill to Karashop (bill: KarashopTelephone.Execute of
// gateway/confirmPayment and karashop.SendPaymentInfo). The body is the service
// tci-SendPaymentInfo-1 with the number in the local form, the amount, the bill ID, the pay ID, the
// bank of the paid provider (banks), the RRN as referenceCode, the channel IPG_ghabzino, and the
// Shamsi date (YYYYMMDD) and the time (HHMMSS) of req.PaidAt in Asia/Tehran.
//
// The answer (bill: HandleKarashopResponse): a result.status.code of resultCodes is that failed
// answer; else a status.code of statusCodes is that answer (200 and 202 are success); else pending
// with bill's "provider down" text. As in bill, the HTTP status is not checked: the body decides.
// A call that fails, or a body that cannot be read, is an error that wraps provider.ErrProviderDown
// (bill: a failed settlement, "connectionError"). The trace ID is meta.transactionId.
//
// The switch (enabled), the credential and the URL are checked first: with one missing, nothing is
// sent. @todo bill uses the time zone of its process for the date and the time. See
// docs/this_vs_bill.md, item 41.
func (a *Adapter) Settle(ctx context.Context, req provider.SettleRequest) (*provider.SettleResponse, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("karashop: adapter not created with New")
	}
	if !SupportsSettle(req.BillType) {
		return nil, fmt.Errorf("karashop: settle of %q: %w", req.BillType, provider.ErrNotSupported)
	}
	if !a.enabled {
		return nil, down("providers.external.karashop.enabled is false")
	}
	if a.code == "" || a.password == "" || a.paymentInfoURL == "" {
		return nil, down("providers.external.karashop.code, password and bill_payment_info_url are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	paid := req.PaidAt.In(tehran)
	body, err := json.Marshal(request{
		Credential: credential{Code: a.code, Password: a.password},
		Parameters: []parameter{
			{Name: "telNumber", Value: localForm(req.TelephoneNumber)},
			{Name: "amount", Value: strconv.FormatUint(req.Amount, 10)},
			{Name: "billId", Value: req.BillID},
			{Name: "paymentId", Value: req.PayID},
			{Name: "bankName", Value: banks[req.PaymentProvider]},
			{Name: "referenceCode", Value: req.RRN},
			{Name: "channelName", Value: channelName},
			{Name: "paymentDate", Value: shamsiDate(paid)},
			{Name: "paymentTime", Value: paid.Format("150405")},
		},
		Service: serviceSendPaymentInfo,
	})
	if err != nil {
		return nil, downCause("encode request", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.paymentInfoURL, bytes.NewReader(body))
	if err != nil {
		return nil, downCause("build request", err) // the message has the URL, never the body
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, downCause("call", err)
	}
	if resp == nil || resp.Body == nil {
		return nil, down("empty answer")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxAnswerBytes))
	if err != nil {
		return nil, downCause("read answer", err)
	}
	return readSettleAnswer(raw)
}

// readSettleAnswer reads the body of a settlement answer (bill: CreateResponsePayment and
// HandleKarashopResponse).
func readSettleAnswer(raw []byte) (*provider.SettleResponse, error) {
	outerParser := parsers.Get()
	defer parsers.Put(outerParser)
	outer, err := outerParser.ParseBytes(raw)
	if err != nil {
		return nil, downCause("decode answer", err)
	}
	text := outer.Get("responseText")
	if text == nil || text.Type() != fastjson.TypeString {
		return nil, down("responseText is not a string")
	}
	innerParser := parsers.Get()
	defer parsers.Put(innerParser)
	ans, err := innerParser.ParseBytes(text.GetStringBytes())
	if err != nil {
		return nil, downCause("decode responseText", err)
	}
	trace := string(ans.GetStringBytes("meta", "transactionId"))
	answer := func(s settleAnswer) *provider.SettleResponse {
		return &provider.SettleResponse{Status: s.status, Code: s.code, Message: s.message, ProviderTraceID: trace}
	}
	// bill: a code that is not in the table gives an empty entry, and the status code decides.
	if c := ans.Get("result", "status", "code"); c != nil && c.Type() == fastjson.TypeNumber {
		if n, err := c.Int(); err == nil {
			if s, ok := resultCodes[n]; ok {
				return answer(s), nil
			}
		}
	}
	if s, ok := statusCodes[ans.GetInt("status", "code")]; ok {
		return answer(s), nil
	}
	return &provider.SettleResponse{Status: provider.StatusPending, Code: http.StatusInternalServerError, Message: providerDownText, ProviderTraceID: trace}, nil
}

// shamsiDate returns the Shamsi (Jalali) date of t as YYYYMMDD (bill: toShamsi). It uses the date
// of t in its own location.
func shamsiDate(t time.Time) string {
	y, m, d := toJalali(t.Year(), int(t.Month()), t.Day())
	return fmt.Sprintf("%04d%02d%02d", y, m, d)
}

// toJalali converts a Gregorian date to the Jalali calendar (the arithmetic of the 33-year cycle,
// valid for the Gregorian years 1600 to 3000).
func toJalali(gy, gm, gd int) (int, int, int) {
	gdm := [12]int{0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334}
	gy2 := gy
	if gm > 2 {
		gy2 = gy + 1
	}
	days := 355666 + 365*gy + (gy2+3)/4 - (gy2+99)/100 + (gy2+399)/400 + gd + gdm[gm-1]
	jy := -1595 + 33*(days/12053)
	days %= 12053
	jy += 4 * (days / 1461)
	days %= 1461
	if days > 365 {
		jy += (days - 1) / 365
		days = (days - 1) % 365
	}
	if days < 186 {
		return jy, 1 + days/31, 1 + days%31
	}
	return jy, 7 + (days-186)/30, 1 + (days-186)%30
}

// Settle fixtures of the mock (task 044): the success answer of Karashop, with a fixed trace ID.
const (
	mockSettleTrace = "mock-karashop-settle-0001"
)

// mockSettle is the success answer of the Karashop mock to a settlement.
func mockSettle(provider.SettleRequest) *provider.SettleResponse {
	s := statusCodes[http.StatusOK]
	return &provider.SettleResponse{Status: s.status, Code: s.code, Message: s.message, ProviderTraceID: mockSettleTrace}
}
