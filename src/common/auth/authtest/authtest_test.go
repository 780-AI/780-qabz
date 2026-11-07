package authtest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/json/v2"
	"encoding/pem"
	"fmt"
	"math/big"
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

func TestNewKeyIsP521WithMatchingPEM(t *testing.T) {
	key := NewKey(t)
	if !assert.NotNil(t, key.Private) {
		t.FailNow()
	}
	assert.Equal(t, elliptic.P521(), key.Private.Curve)
	block, _ := pem.Decode(key.PublicPEM)
	if !assert.NotNil(t, block) {
		t.FailNow()
	}
	assert.Equal(t, "PUBLIC KEY", block.Type)
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	ec, ok := pub.(*ecdsa.PublicKey)
	if !assert.True(t, ok) || !assert.NotNil(t, ec) {
		t.FailNow()
	}
	assert.True(t, ec.Equal(&key.Private.PublicKey))
}

func TestSignMakesAVerifiableToken(t *testing.T) {
	cases := []struct {
		name    string
		curve   elliptic.Curve
		alg     string
		sigSize int
	}{
		{name: "P-521 is ES512", curve: elliptic.P521(), alg: "ES512", sigSize: 132},
		{name: "P-256 is ES256", curve: elliptic.P256(), alg: "ES256", sigSize: 64},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key := newKey(t, tc.curve)
			if !assert.NotNil(t, key.Private) {
				t.FailNow()
			}
			token := Sign(t, key.Private, map[string]any{"alg": tc.alg}, ValidClaims())
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
			if !assert.NoError(t, err) || !assert.Len(t, sig, tc.sigSize) {
				t.FailNow()
			}
			input := []byte(parts[0] + "." + parts[1])
			var digest []byte
			if tc.sigSize == 132 {
				d := sha512.Sum512(input)
				digest = d[:]
			} else {
				d := sha256.Sum256(input)
				digest = d[:]
			}
			half := tc.sigSize / 2
			r := new(big.Int).SetBytes(sig[:half])
			s := new(big.Int).SetBytes(sig[half:])
			assert.True(t, ecdsa.Verify(&key.Private.PublicKey, digest, r, s))
		})
	}
}

func TestSignStopsWithoutKey(t *testing.T) {
	cases := []struct {
		name string
		key  *ecdsa.PrivateKey
	}{
		{name: "nil key", key: nil},
		{name: "key without curve", key: &ecdsa.PrivateKey{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fatalTB{TB: t}
			assert.Equal(t, "", Sign(f, tc.key, ES512Header(), ValidClaims()))
			assert.Contains(t, f.fatal, "no key")
		})
	}
}

func TestEncodeStopsOnBadValue(t *testing.T) {
	f := &fatalTB{TB: t}
	_ = encode(f, map[string]any{"bad": func() {}})
	assert.Contains(t, f.fatal, "authtest: marshal")
}

func TestCasesTable(t *testing.T) {
	cases := Cases(t, NewKey(t))
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
	assert.Positive(t, valid)
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
