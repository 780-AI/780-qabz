// Package crypto signs the order token (system.html, common/crypto; task 039): a JWT that the
// payment flow reads. It copies the old bill service (pkg/crypt/jwtSign.go, go-jose v2): RS256 with
// the general private key that bill and other services share, the header kid (the service name)
// and the type JWT. Qabz makes no key of its own.
//
// The package knows nothing of an order: it signs the payload bytes it gets.
package crypto

import (
	stdcrypto "crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

// Signer signs order tokens. It is safe for concurrent use.
type Signer struct {
	key *rsa.PrivateKey
	// header is the base64url text of the protected header, made once.
	header string
}

// NewSigner returns a signer with the private key in keyPEM and the header kid (bill:
// crypt.NewJWTMarker): the first PEM block of keyPEM must be a PKCS #1 RSA private key. No PEM
// block is the error "nil private key", as bill.
func NewSigner(keyPEM []byte, kid string) (*Signer, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, errors.New("crypto: nil private key")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("crypto: parse private key: %w", err)
	}
	header, err := headerJSON(kid)
	if err != nil {
		return nil, err
	}
	return &Signer{key: key, header: base64.RawURLEncoding.EncodeToString(header)}, nil
}

// LoadSigner reads the private key file at path (bill: app reads paymentServer.privateKeyPath at
// start) and returns NewSigner. A file that cannot be read is an error that names the path.
func LoadSigner(path, kid string) (*Signer, error) {
	if path == "" {
		return nil, errors.New("crypto: no private key path")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("crypto: read private key %s: %w", path, err)
	}
	s, err := NewSigner(b, kid)
	if err != nil {
		return nil, fmt.Errorf("%w (file %s)", err, path)
	}
	return s, nil
}

// headerJSON is the protected header that go-jose v2 writes for bill's signer: the keys in sorted
// order, the kid as a JSON string with the escapes of encoding/json v1 (HTML characters, U+2028 and
// U+2029).
func headerJSON(kid string) ([]byte, error) {
	k, err := json.Marshal(kid, jsontext.EscapeForHTML(true), jsontext.EscapeForJS(true), jsontext.AllowInvalidUTF8(true))
	if err != nil {
		return nil, fmt.Errorf("crypto: header: %w", err)
	}
	return []byte(`{"alg":"RS256","kid":` + string(k) + `,"typ":"JWT"}`), nil
}

// Sign returns the compact JWS of the payload: header.payload.signature, each part base64url with
// no padding, the signature RSASSA-PKCS1-v1_5 with SHA-256 (RS256) over "header.payload".
func (s *Signer) Sign(payload []byte) (string, error) {
	if s == nil || s.key == nil {
		return "", errors.New("crypto: signer not created with NewSigner")
	}
	input := s.header + "." + base64.RawURLEncoding.EncodeToString(payload)
	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, stdcrypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("crypto: sign: %w", err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// PublicKey is the public key of the signer, for a check of a token.
func (s *Signer) PublicKey() *rsa.PublicKey {
	if s == nil || s.key == nil {
		return nil
	}
	return &s.key.PublicKey
}
