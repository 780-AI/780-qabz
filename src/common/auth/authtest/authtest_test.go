package authtest

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json/v2"
	"fmt"
	"hash"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// fatalTB records Fatalf instead of stopping the test, so the failure paths of the helpers can run.
type fatalTB struct {
	testing.TB
	fatal string
}

func (f *fatalTB) Helper() {}

func (f *fatalTB) Fatalf(format string, args ...any) { f.fatal = fmt.Sprintf(format, args...) }

func TestSignMakesAnHMACToken(t *testing.T) {
	cases := []struct {
		alg     string
		newHash func() hash.Hash
		size    int
	}{
		{alg: "HS256", newHash: sha256.New, size: 32},
		{alg: "HS384", newHash: sha512.New384, size: 48},
		{alg: "HS512", newHash: sha512.New, size: 64},
		{alg: "RS256", newHash: sha256.New, size: 32}, // a non-HMAC alg is signed with SHA-256
	}
	for _, tc := range cases {
		t.Run(tc.alg, func(t *testing.T) {
			token := Sign(t, Secret, Header(tc.alg), ValidClaims())
			parts := strings.Split(token, ".")
			if !assert.Len(t, parts, 3) {
				t.FailNow()
			}

			header, err := base64.RawURLEncoding.DecodeString(parts[0])
			if !assert.NoError(t, err) {
				t.FailNow()
			}
			var h map[string]any
			if !assert.NoError(t, json.Unmarshal(header, &h)) {
				t.FailNow()
			}
			assert.Equal(t, tc.alg, h["alg"])
			assert.Equal(t, "JWT", h["typ"])

			payload, err := base64.RawURLEncoding.DecodeString(parts[1])
			if !assert.NoError(t, err) {
				t.FailNow()
			}
			var c map[string]any
			if !assert.NoError(t, json.Unmarshal(payload, &c)) {
				t.FailNow()
			}
			assert.Equal(t, UserID, c["sub"])
			assert.Equal(t, Audience, c["aud"])

			sig, err := base64.RawURLEncoding.DecodeString(parts[2])
			if !assert.NoError(t, err) || !assert.Len(t, sig, tc.size) {
				t.FailNow()
			}
			mac := hmac.New(tc.newHash, []byte(Secret))
			mac.Write([]byte(parts[0] + "." + parts[1]))
			assert.True(t, hmac.Equal(mac.Sum(nil), sig))
		})
	}
}

func TestSignStopsWithoutSecret(t *testing.T) {
	f := &fatalTB{TB: t}
	assert.Equal(t, "", Sign(f, "", Header("HS256"), ValidClaims()))
	assert.Contains(t, f.fatal, "no secret")
}

func TestEncodeStopsOnBadValue(t *testing.T) {
	f := &fatalTB{TB: t}
	_ = encode(f, map[string]any{"bad": func() {}})
	assert.Contains(t, f.fatal, "authtest: marshal")
}

func TestCasesTable(t *testing.T) {
	cases := Cases(t)
	if !assert.NotEmpty(t, cases) {
		t.FailNow()
	}
	names := map[string]bool{}
	valid, noHeader := 0, 0
	for _, c := range cases {
		assert.NotEmpty(t, c.Name)
		assert.False(t, names[c.Name], "duplicate case name %q", c.Name)
		names[c.Name] = true
		if c.Valid {
			valid++
			assert.Empty(t, c.Reason, c.Name)
			assert.NotEmpty(t, c.Authorization, c.Name)
		} else {
			assert.NotEmpty(t, c.Reason, c.Name)
		}
		if c.NoHeader {
			noHeader++
			assert.Empty(t, c.Authorization, c.Name)
		}
	}
	assert.Equal(t, 6, valid)
	assert.Equal(t, 1, noHeader)
}

func TestCut3(t *testing.T) {
	cases := []struct {
		in      string
		a, b, c string
	}{
		{in: "a.b.c", a: "a", b: "b", c: "c"},
		{in: "a.b.c.d", a: "a", b: "b", c: "c.d"},
		{in: "a.b", a: "a", b: "b", c: ""},
		{in: "a", a: "a", b: "", c: ""},
		{in: "", a: "", b: "", c: ""},
	}
	for _, tc := range cases {
		a, b, c := cut3(tc.in)
		assert.Equal(t, []string{tc.a, tc.b, tc.c}, []string{a, b, c}, tc.in)
	}
}

// TestVerifierMatchesTable: the verifier of Verifier accepts exactly the valid inputs of the table.
func TestVerifierMatchesTable(t *testing.T) {
	v := Verifier(t)
	if !assert.NotNil(t, v) {
		t.FailNow()
	}
	for _, c := range Cases(t) {
		_, err := v.VerifyAuthorization(c.Authorization)
		assert.Equal(t, c.Valid, err == nil, c.Name)
	}
}
