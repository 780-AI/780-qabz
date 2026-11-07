// Package authtest makes test secrets, test tokens and the shared table of JWT inputs for the auth
// tests. It is for tests only: no production code imports it.
package authtest

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json/v2"
	"hash"
	"strings"
	"testing"
	"time"

	"qabz/common/auth"
)

// Audience is the aud value the test verifiers expect.
const Audience = "qabz"

// Now is the fixed clock of the tests, so the exp and nbf cases do not depend on the time of the run.
var Now = time.Unix(1_800_000_000, 0)

// UserID is the sub of the valid test token.
const UserID = "6f1c2b9e-1d2a-4c3b-9a8e-0f1e2d3c4b5a"

// Secret is the HMAC secret of the test verifiers. It is a test value, never a real secret.
const Secret = "qabz-test-hmac-secret-not-for-production"

// otherSecret signs the "signed with another secret" case.
const otherSecret = "another-test-hmac-secret"

// ValidClaims returns the claims of a valid token at Now.
func ValidClaims() map[string]any {
	return map[string]any{
		"sub":     UserID,
		"mobile":  "09120000000",
		"non_otp": "normal",
		"aud":     Audience,
		"iat":     Now.Unix() - 60,
		"exp":     Now.Unix() + 3600,
	}
}

// Verifier returns a verifier with Secret, Audience and the clock Now. It stops the test if that fails.
func Verifier(t testing.TB) *auth.Verifier {
	t.Helper()
	v, err := auth.NewVerifier([]byte(Secret), Audience, func() time.Time { return Now })
	if err != nil || v == nil {
		t.Fatalf("authtest: new verifier: %v", err)
		return nil
	}
	return v
}

// Header returns a JWT header with alg.
func Header(alg string) map[string]any { return map[string]any{"alg": alg, "typ": "JWT"} }

// macHash returns the hash of an HMAC alg. Any other alg gets SHA-256, so a test can sign a token
// whose header names another algorithm (RS256, ES512, none) and check that the alg is refused.
func macHash(alg any) func() hash.Hash {
	switch alg {
	case "HS384":
		return sha512.New384
	case "HS512":
		return sha512.New
	}
	return sha256.New
}

// Sign returns header.payload.signature, an HMAC of header.payload with secret. The hash comes from
// header["alg"] (see macHash); Sign does not check the alg, so a test can lie about it. It stops the
// test when secret is empty.
func Sign(t testing.TB, secret string, header, claims map[string]any) string {
	t.Helper()
	if secret == "" {
		t.Fatalf("authtest: sign: no secret")
		return ""
	}
	input := encode(t, header) + "." + encode(t, claims)
	mac := hmac.New(macHash(header["alg"]), []byte(secret))
	mac.Write([]byte(input))
	return input + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func encode(t testing.TB, v map[string]any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("authtest: marshal: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// Case is one JWT input of the shared table.
type Case struct {
	Name string
	// NoHeader means the call has no Authorization value at all.
	NoHeader bool
	// Authorization is the full header value.
	Authorization string
	// Valid means the call must reach the handler while the check is on.
	Valid bool
	// Reason is the expected rejection reason (common/auth) when Valid is false.
	Reason string
}

// Cases returns every JWT input of the tests, for a verifier with Secret and Audience. Add a new
// input here: every test that uses the table picks it up.
func Cases(t testing.TB) []Case {
	t.Helper()
	b := func(token string) string { return "Bearer " + token }
	with := func(edit func(map[string]any)) map[string]any {
		c := ValidClaims()
		edit(c)
		return c
	}
	hs256 := Header("HS256")
	sign := func(claims map[string]any) string { return Sign(t, Secret, hs256, claims) }
	good := sign(ValidClaims())

	// The payload changed after signing: the signature no longer matches.
	tampered := func() string {
		h, _, sig := cut3(good)
		return h + "." + encode(t, with(func(c map[string]any) { c["sub"] = "00000000-0000-0000-0000-000000000000" })) + "." + sig
	}()
	// A signature of the right size that is all zero.
	zeroSig := func() string {
		h, p, _ := cut3(good)
		return h + "." + p + "." + base64.RawURLEncoding.EncodeToString(make([]byte, sha256.Size))
	}()
	// The first half of the right signature.
	shortSig := func() string {
		h, p, sig := cut3(good)
		raw, err := base64.RawURLEncoding.DecodeString(sig)
		if err != nil {
			t.Fatalf("authtest: decode signature: %v", err)
		}
		return h + "." + p + "." + base64.RawURLEncoding.EncodeToString(raw[:len(raw)/2])
	}()
	// An HS256 signature under a header that says HS512.
	hs512Header := func() string {
		_, p, sig := cut3(good)
		return encode(t, Header("HS512")) + "." + p + "." + sig
	}()
	// alg none with no signature.
	none := encode(t, map[string]any{"alg": "none"}) + "." + encode(t, ValidClaims()) + "."
	notJSON := base64.RawURLEncoding.EncodeToString([]byte("not json")) + "." + encode(t, ValidClaims()) + ".AA"

	return []Case{
		{Name: "no header", NoHeader: true, Reason: "missing"},
		{Name: "empty header", Authorization: "", Reason: "missing"},
		{Name: "bearer without token", Authorization: "Bearer", Reason: "scheme"},
		{Name: "bearer with empty token", Authorization: "Bearer ", Reason: "scheme"},
		{Name: "wrong scheme", Authorization: "Basic dXNlcjpwYXNz", Reason: "scheme"},
		{Name: "token with a space", Authorization: "Bearer " + good + " x", Reason: "scheme"},
		{Name: "two parts", Authorization: b("aGVhZGVy.cGF5bG9hZA"), Reason: "malformed"},
		{Name: "four parts", Authorization: b(good + ".x"), Reason: "malformed"},
		{Name: "bad base64", Authorization: b("!!!.@@@.###"), Reason: "malformed"},
		{Name: "header not json", Authorization: b(notJSON), Reason: "malformed"},
		{Name: "expired", Authorization: b(sign(with(func(c map[string]any) { c["exp"] = Now.Unix() - 1 }))), Reason: "expired"},
		{Name: "exp equals now", Authorization: b(sign(with(func(c map[string]any) { c["exp"] = Now.Unix() }))), Reason: "expired"},
		{Name: "no exp", Authorization: b(sign(with(func(c map[string]any) { delete(c, "exp") }))), Reason: "expired"},
		{Name: "exp is a string", Authorization: b(sign(with(func(c map[string]any) { c["exp"] = "9999999999" }))), Reason: "expired"},
		{Name: "nbf in the future", Authorization: b(sign(with(func(c map[string]any) { c["nbf"] = Now.Unix() + 60 }))), Reason: "not_yet_valid"},
		{Name: "nbf is a string", Authorization: b(sign(with(func(c map[string]any) { c["nbf"] = "0" }))), Reason: "malformed"},
		{Name: "signed with another secret", Authorization: b(Sign(t, otherSecret, hs256, ValidClaims())), Reason: "signature"},
		{Name: "payload changed after signing", Authorization: b(tampered), Reason: "signature"},
		{Name: "zero signature", Authorization: b(zeroSig), Reason: "signature"},
		{Name: "short signature", Authorization: b(shortSig), Reason: "signature"},
		{Name: "HS256 signature under an HS512 header", Authorization: b(hs512Header), Reason: "signature"},
		{Name: "alg none", Authorization: b(none), Reason: "algorithm"},
		{Name: "no alg", Authorization: b(Sign(t, Secret, map[string]any{"typ": "JWT"}, ValidClaims())), Reason: "algorithm"},
		{Name: "lower-case hs256", Authorization: b(Sign(t, Secret, Header("hs256"), ValidClaims())), Reason: "algorithm"},
		{Name: "RS256", Authorization: b(Sign(t, Secret, Header("RS256"), ValidClaims())), Reason: "algorithm"},
		{Name: "ES512", Authorization: b(Sign(t, Secret, Header("ES512"), ValidClaims())), Reason: "algorithm"},
		{Name: "PS256", Authorization: b(Sign(t, Secret, Header("PS256"), ValidClaims())), Reason: "algorithm"},
		{Name: "crit header", Authorization: b(Sign(t, Secret, map[string]any{"alg": "HS256", "crit": []string{"exp"}, "exp": 1}, ValidClaims())), Reason: "algorithm"},
		{Name: "sub not a uuid", Authorization: b(sign(with(func(c map[string]any) { c["sub"] = "12345" }))), Reason: "claims"},
		{Name: "no sub", Authorization: b(sign(with(func(c map[string]any) { delete(c, "sub") }))), Reason: "claims"},
		{Name: "no mobile", Authorization: b(sign(with(func(c map[string]any) { delete(c, "mobile") }))), Reason: "claims"},
		{Name: "no non_otp", Authorization: b(sign(with(func(c map[string]any) { delete(c, "non_otp") }))), Reason: "claims"},
		{Name: "wrong aud", Authorization: b(sign(with(func(c map[string]any) { c["aud"] = "other" }))), Reason: "audience"},
		{Name: "no aud", Authorization: b(sign(with(func(c map[string]any) { delete(c, "aud") }))), Reason: "audience"},
		{Name: "guest", Authorization: b(sign(with(func(c map[string]any) { c["non_otp"] = "guest" }))), Reason: "guest"},
		{Name: "valid HS256", Authorization: b(good), Valid: true},
		{Name: "valid HS384", Authorization: b(Sign(t, Secret, Header("HS384"), ValidClaims())), Valid: true},
		{Name: "valid HS512", Authorization: b(Sign(t, Secret, Header("HS512"), ValidClaims())), Valid: true},
		{Name: "valid lower-case scheme", Authorization: "bearer " + good, Valid: true},
		{Name: "valid aud array", Authorization: b(sign(with(func(c map[string]any) { c["aud"] = []string{"other", Audience} }))), Valid: true},
		{Name: "valid nbf in the past", Authorization: b(sign(with(func(c map[string]any) { c["nbf"] = Now.Unix() - 60 }))), Valid: true},
	}
}

// cut3 splits a token into its three parts (test tokens only).
func cut3(token string) (string, string, string) {
	parts := strings.SplitN(token, ".", 3)
	for len(parts) < 3 {
		parts = append(parts, "")
	}
	return parts[0], parts[1], parts[2]
}
