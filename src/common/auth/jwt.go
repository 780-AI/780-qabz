// Package auth verifies the user JWT that the auth service issues. Qabz only verifies tokens, with
// the issuer's public key; it never signs one.
//
// The check is stateless: signature, algorithm, time claims and the required claims. It runs for
// every request, so it is a hot path: the JSON parts are read with fastjson (conventions § 1).
package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"
	"uuid"

	"github.com/valyala/fastjson"
)

// Algorithm is the only JWT algorithm Qabz accepts (ECDSA P-521 with SHA-512). A token with any
// other "alg" is rejected, so a token cannot choose a weaker check.
const Algorithm = "ES512"

// es512KeySize is the size of each of r and s in an ES512 signature (RFC 7518 § 3.4).
const es512KeySize = 66

// Reason tells why a token was rejected. It is a metric label, never sent to the client.
type Reason string

// Rejection reasons.
const (
	ReasonMissing     Reason = "missing"       // no Authorization value
	ReasonScheme      Reason = "scheme"        // not "Bearer <token>"
	ReasonMalformed   Reason = "malformed"     // not three base64url parts of valid JSON
	ReasonAlgorithm   Reason = "algorithm"     // alg is not Algorithm, or crit is present
	ReasonSignature   Reason = "signature"     // the signature does not match the key
	ReasonExpired     Reason = "expired"       // exp is missing or not in the future
	ReasonNotYetValid Reason = "not_yet_valid" // nbf is in the future
	ReasonClaims      Reason = "claims"        // sub, mobile or non_otp missing or bad
	ReasonAudience    Reason = "audience"      // aud does not contain the expected audience
	ReasonGuest       Reason = "guest"         // a guest (non_otp == "guest") token
)

// Error is a rejected token.
type Error struct{ Reason Reason }

func (e *Error) Error() string {
	if e == nil {
		return "auth: rejected"
	}
	return "auth: rejected (" + string(e.Reason) + ")"
}

func reject(r Reason) error { return &Error{Reason: r} }

// ReasonOf returns the Reason of err, or "" when err is not an *Error.
func ReasonOf(err error) Reason {
	var e *Error
	if errors.As(err, &e) && e != nil {
		return e.Reason
	}
	return ""
}

// Claims are the token values that the handlers can use.
type Claims struct {
	UserID string // sub, a UUID
	Mobile string // mobile
	NonOTP string // non_otp, the login type (never "guest" here)
}

// Verifier checks tokens with one public key and one expected audience.
type Verifier struct {
	key      *ecdsa.PublicKey
	audience string
	now      func() time.Time
	parsers  fastjson.ParserPool
}

// NewVerifier returns a Verifier for key and audience. now is the clock; nil means time.Now.
func NewVerifier(key *ecdsa.PublicKey, audience string, now func() time.Time) (*Verifier, error) {
	if key == nil {
		return nil, errors.New("auth: public key is required")
	}
	if key.Curve != elliptic.P521() {
		return nil, fmt.Errorf("auth: %s needs a P-521 public key", Algorithm)
	}
	if audience == "" {
		return nil, errors.New("auth: audience is required")
	}
	if now == nil {
		now = time.Now
	}
	return &Verifier{key: key, audience: audience, now: now}, nil
}

// LoadVerifier reads a PEM public key file and returns a Verifier that uses the real clock.
func LoadVerifier(path, audience string) (*Verifier, error) {
	if path == "" {
		return nil, errors.New("auth: public key path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("auth: public key file: %w", err)
	}
	key, err := ParsePublicKey(data)
	if err != nil {
		return nil, err
	}
	return NewVerifier(key, audience, nil)
}

// ParsePublicKey reads a PEM "PUBLIC KEY" (PKIX) block that holds an ECDSA P-521 key.
func ParsePublicKey(data []byte) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, errors.New("auth: no PEM \"PUBLIC KEY\" block")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("auth: public key: %w", err)
	}
	key, ok := pub.(*ecdsa.PublicKey)
	if !ok || key == nil {
		return nil, errors.New("auth: the public key is not an ECDSA key")
	}
	if key.Curve != elliptic.P521() {
		return nil, fmt.Errorf("auth: %s needs a P-521 public key", Algorithm)
	}
	return key, nil
}

// BearerToken returns the token of an Authorization value "Bearer <token>". The scheme is not
// case-sensitive (RFC 9110 § 11.1); exactly one space separates it from the token.
func BearerToken(authorization string) (string, error) {
	if authorization == "" {
		return "", reject(ReasonMissing)
	}
	scheme, token, ok := strings.Cut(authorization, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", reject(ReasonScheme)
	}
	if token == "" || strings.ContainsAny(token, " \t") {
		return "", reject(ReasonScheme)
	}
	return token, nil
}

// VerifyAuthorization checks an Authorization value ("Bearer <token>").
func (v *Verifier) VerifyAuthorization(authorization string) (Claims, error) {
	token, err := BearerToken(authorization)
	if err != nil {
		return Claims{}, err
	}
	return v.Verify(token)
}

// Verify checks a compact JWS token and returns its claims. The signature is checked before the
// claims are read.
func (v *Verifier) Verify(token string) (Claims, error) {
	if v == nil || v.key == nil {
		return Claims{}, errors.New("auth: verifier not created with NewVerifier")
	}
	headerB64, rest, ok := strings.Cut(token, ".")
	if !ok {
		return Claims{}, reject(ReasonMalformed)
	}
	payloadB64, sigB64, ok := strings.Cut(rest, ".")
	if !ok || strings.Contains(sigB64, ".") || headerB64 == "" || payloadB64 == "" {
		return Claims{}, reject(ReasonMalformed)
	}

	header, err := base64.RawURLEncoding.DecodeString(headerB64)
	if err != nil {
		return Claims{}, reject(ReasonMalformed)
	}
	if err := v.checkHeader(header); err != nil {
		return Claims{}, err
	}

	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil || len(sig) != 2*es512KeySize {
		return Claims{}, reject(ReasonSignature)
	}
	digest := sha512.Sum512([]byte(token[:len(headerB64)+1+len(payloadB64)]))
	r := new(big.Int).SetBytes(sig[:es512KeySize])
	s := new(big.Int).SetBytes(sig[es512KeySize:])
	if !ecdsa.Verify(v.key, digest[:], r, s) {
		return Claims{}, reject(ReasonSignature)
	}

	payload, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return Claims{}, reject(ReasonMalformed)
	}
	return v.readClaims(payload)
}

// checkHeader accepts only {"alg":"ES512"} (plus typ and kid) and no "crit" extension.
func (v *Verifier) checkHeader(header []byte) error {
	p := v.parsers.Get()
	defer v.parsers.Put(p)
	h, err := p.ParseBytes(header)
	if err != nil || h == nil || h.Type() != fastjson.TypeObject {
		return reject(ReasonMalformed)
	}
	if h.Exists("crit") {
		return reject(ReasonAlgorithm)
	}
	alg := h.Get("alg")
	if alg == nil || alg.Type() != fastjson.TypeString || string(alg.GetStringBytes()) != Algorithm {
		return reject(ReasonAlgorithm)
	}
	return nil
}

// readClaims checks the time claims, the audience and the required claims. Every string is
// copied out before the parser goes back to the pool.
func (v *Verifier) readClaims(payload []byte) (Claims, error) {
	p := v.parsers.Get()
	defer v.parsers.Put(p)
	c, err := p.ParseBytes(payload)
	if err != nil || c == nil || c.Type() != fastjson.TypeObject {
		return Claims{}, reject(ReasonMalformed)
	}

	// Compare as float64: exp and nbf are JSON numbers (NumericDate, RFC 7519 § 2), and a float
	// compare cannot overflow the way a conversion of a huge value to int64 can.
	now := float64(v.now().Unix())
	exp := c.Get("exp")
	if exp == nil || exp.Type() != fastjson.TypeNumber {
		return Claims{}, reject(ReasonExpired)
	}
	if expAt, err := exp.Float64(); err != nil || expAt <= now {
		return Claims{}, reject(ReasonExpired)
	}
	if nbf := c.Get("nbf"); nbf != nil {
		if nbf.Type() != fastjson.TypeNumber {
			return Claims{}, reject(ReasonMalformed)
		}
		if nbfAt, err := nbf.Float64(); err != nil || nbfAt > now {
			return Claims{}, reject(ReasonNotYetValid)
		}
	}

	if !hasAudience(c.Get("aud"), v.audience) {
		return Claims{}, reject(ReasonAudience)
	}

	sub, ok := stringClaim(c, "sub")
	if !ok || len(sub) != 36 {
		return Claims{}, reject(ReasonClaims)
	}
	if _, err := uuid.Parse(sub); err != nil {
		return Claims{}, reject(ReasonClaims)
	}
	mobile, ok := stringClaim(c, "mobile")
	if !ok {
		return Claims{}, reject(ReasonClaims)
	}
	nonOTP, ok := stringClaim(c, "non_otp")
	if !ok {
		return Claims{}, reject(ReasonClaims)
	}
	if nonOTP == "guest" {
		return Claims{}, reject(ReasonGuest)
	}
	return Claims{UserID: sub, Mobile: mobile, NonOTP: nonOTP}, nil
}

// stringClaim returns a copy of the non-empty string claim name.
func stringClaim(c *fastjson.Value, name string) (string, bool) {
	v := c.Get(name)
	if v == nil || v.Type() != fastjson.TypeString {
		return "", false
	}
	s := string(v.GetStringBytes()) // string() copies the bytes out of the parser
	return s, s != ""
}

// hasAudience reports whether aud (a string or an array of strings, RFC 7519 § 4.1.3) contains want.
func hasAudience(aud *fastjson.Value, want string) bool {
	if aud == nil {
		return false
	}
	switch aud.Type() {
	case fastjson.TypeString:
		return string(aud.GetStringBytes()) == want
	case fastjson.TypeArray:
		items, err := aud.Array()
		if err != nil {
			return false
		}
		for _, item := range items {
			if item != nil && item.Type() == fastjson.TypeString && string(item.GetStringBytes()) == want {
				return true
			}
		}
	}
	return false
}
