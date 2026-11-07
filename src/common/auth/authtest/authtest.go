// Package authtest makes test keys, test tokens and the shared table of JWT inputs for the auth
// tests. It is for tests only: no production code imports it, and Qabz itself never signs a token.
package authtest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/json/v2"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

// Audience is the aud value the test verifiers expect.
const Audience = "qabz"

// Now is the fixed clock of the tests, so the exp and nbf cases do not depend on the time of the run.
var Now = time.Unix(1_800_000_000, 0)

// UserID is the sub of the valid test token.
const UserID = "6f1c2b9e-1d2a-4c3b-9a8e-0f1e2d3c4b5a"

// Key is a test key pair.
type Key struct {
	Private   *ecdsa.PrivateKey
	PublicPEM []byte
}

// NewKey makes a P-521 key pair. It stops the test if a step fails.
func NewKey(t testing.TB) Key {
	t.Helper()
	return newKey(t, elliptic.P521())
}

func newKey(t testing.TB, curve elliptic.Curve) Key {
	t.Helper()
	priv, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil || priv == nil {
		t.Fatalf("authtest: generate key: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("authtest: marshal public key: %v", err)
	}
	return Key{Private: priv, PublicPEM: pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})}
}

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

// Sign returns header.payload.signature, signed with key (ES512 for P-521, ES256 for P-256).
// header must name the algorithm; Sign does not check it, so a test can lie about it.
func Sign(t testing.TB, key *ecdsa.PrivateKey, header, claims map[string]any) string {
	t.Helper()
	if key == nil || key.Curve == nil {
		t.Fatalf("authtest: sign: no key")
		return ""
	}
	input := encode(t, header) + "." + encode(t, claims)
	size := (key.Curve.Params().BitSize + 7) / 8
	var digest []byte
	if size == 66 {
		d := sha512.Sum512([]byte(input))
		digest = d[:]
	} else {
		d := sha256.Sum256([]byte(input))
		digest = d[:]
	}
	r, s, err := ecdsa.Sign(rand.Reader, key, digest)
	if err != nil {
		t.Fatalf("authtest: sign: %v", err)
	}
	sig := make([]byte, 2*size)
	r.FillBytes(sig[:size])
	s.FillBytes(sig[size:])
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// ES512Header is the header of a good token.
func ES512Header() map[string]any { return map[string]any{"alg": "ES512", "typ": "JWT"} }

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

// Cases returns every JWT input of the tests, built for key. Add a new input here: every test
// that uses the table picks it up.
func Cases(t testing.TB, key Key) []Case {
	t.Helper()
	b := func(token string) string { return "Bearer " + token }
	with := func(edit func(map[string]any)) map[string]any {
		c := ValidClaims()
		edit(c)
		return c
	}
	good := Sign(t, key.Private, ES512Header(), ValidClaims())
	other := NewKey(t)
	p256 := newKey(t, elliptic.P256())

	// The payload changed after signing: the signature no longer matches.
	tampered := func() string {
		h, _, sig := cut3(good)
		return h + "." + encode(t, with(func(c map[string]any) { c["sub"] = "00000000-0000-0000-0000-000000000000" })) + "." + sig
	}()
	// HS256 with the public key as the HMAC secret (the algorithm-confusion attack).
	hs256 := func() string {
		input := encode(t, map[string]any{"alg": "HS256", "typ": "JWT"}) + "." + encode(t, ValidClaims())
		m := hmac.New(sha256.New, key.PublicPEM)
		m.Write([]byte(input))
		return input + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
	}()
	// alg none with no signature.
	none := encode(t, map[string]any{"alg": "none"}) + "." + encode(t, ValidClaims()) + "."
	// A valid ES512 signature over a header with a crit extension.
	crit := Sign(t, key.Private, map[string]any{"alg": "ES512", "crit": []string{"exp"}, "exp": 1}, ValidClaims())
	// A signature of the right size that is all zero (r = s = 0 is never valid).
	zeroSig := func() string {
		h, p, _ := cut3(good)
		return h + "." + p + "." + base64.RawURLEncoding.EncodeToString(make([]byte, 132))
	}()
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
		{Name: "expired", Authorization: b(Sign(t, key.Private, ES512Header(), with(func(c map[string]any) { c["exp"] = Now.Unix() - 1 }))), Reason: "expired"},
		{Name: "exp equals now", Authorization: b(Sign(t, key.Private, ES512Header(), with(func(c map[string]any) { c["exp"] = Now.Unix() }))), Reason: "expired"},
		{Name: "no exp", Authorization: b(Sign(t, key.Private, ES512Header(), with(func(c map[string]any) { delete(c, "exp") }))), Reason: "expired"},
		{Name: "exp is a string", Authorization: b(Sign(t, key.Private, ES512Header(), with(func(c map[string]any) { c["exp"] = "9999999999" }))), Reason: "expired"},
		{Name: "nbf in the future", Authorization: b(Sign(t, key.Private, ES512Header(), with(func(c map[string]any) { c["nbf"] = Now.Unix() + 60 }))), Reason: "not_yet_valid"},
		{Name: "signed with another key", Authorization: b(Sign(t, other.Private, ES512Header(), ValidClaims())), Reason: "signature"},
		{Name: "payload changed after signing", Authorization: b(tampered), Reason: "signature"},
		{Name: "zero signature", Authorization: b(zeroSig), Reason: "signature"},
		{Name: "alg none", Authorization: b(none), Reason: "algorithm"},
		{Name: "HS256 with the public key", Authorization: b(hs256), Reason: "algorithm"},
		{Name: "ES256", Authorization: b(Sign(t, p256.Private, map[string]any{"alg": "ES256"}, ValidClaims())), Reason: "algorithm"},
		{Name: "crit header", Authorization: b(crit), Reason: "algorithm"},
		{Name: "sub not a uuid", Authorization: b(Sign(t, key.Private, ES512Header(), with(func(c map[string]any) { c["sub"] = "12345" }))), Reason: "claims"},
		{Name: "no sub", Authorization: b(Sign(t, key.Private, ES512Header(), with(func(c map[string]any) { delete(c, "sub") }))), Reason: "claims"},
		{Name: "no mobile", Authorization: b(Sign(t, key.Private, ES512Header(), with(func(c map[string]any) { delete(c, "mobile") }))), Reason: "claims"},
		{Name: "no non_otp", Authorization: b(Sign(t, key.Private, ES512Header(), with(func(c map[string]any) { delete(c, "non_otp") }))), Reason: "claims"},
		{Name: "wrong aud", Authorization: b(Sign(t, key.Private, ES512Header(), with(func(c map[string]any) { c["aud"] = "other" }))), Reason: "audience"},
		{Name: "no aud", Authorization: b(Sign(t, key.Private, ES512Header(), with(func(c map[string]any) { delete(c, "aud") }))), Reason: "audience"},
		{Name: "guest", Authorization: b(Sign(t, key.Private, ES512Header(), with(func(c map[string]any) { c["non_otp"] = "guest" }))), Reason: "guest"},
		{Name: "valid", Authorization: b(good), Valid: true},
		{Name: "valid lower-case scheme", Authorization: "bearer " + good, Valid: true},
		{Name: "valid aud array", Authorization: b(Sign(t, key.Private, ES512Header(), with(func(c map[string]any) { c["aud"] = []string{"other", Audience} }))), Valid: true},
		{Name: "valid nbf in the past", Authorization: b(Sign(t, key.Private, ES512Header(), with(func(c map[string]any) { c["nbf"] = Now.Unix() - 60 }))), Valid: true},
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
