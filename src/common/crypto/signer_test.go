package crypto

import (
	stdcrypto "crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	keyOnce sync.Once
	testKey *rsa.PrivateKey
)

// key returns one generated RSA key for the tests of the package (no key file is in git).
func key(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	keyOnce.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err == nil {
			testKey = k
		}
	})
	if testKey == nil {
		t.Fatal("generate key")
	}
	return testKey
}

func pkcs1PEM(k *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
}

// TestSignVerifies: the token has three parts, the header of go-jose v2 for bill's signer, the
// payload bytes as given, and an RS256 signature that the public key accepts.
func TestSignVerifies(t *testing.T) {
	k := key(t)
	s, err := NewSigner(pkcs1PEM(k), "qabz")
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		return
	}
	payload := []byte(`{"serviceId":105,"orderId":"x","data":{"a":"<&>"}}`)
	token, err := s.Sign(payload)
	if !assert.NoError(t, err) {
		return
	}
	parts := strings.Split(token, ".")
	if !assert.Len(t, parts, 3) {
		return
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	assert.NoError(t, err)
	assert.Equal(t, `{"alg":"RS256","kid":"qabz","typ":"JWT"}`, string(header))
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	assert.NoError(t, err)
	assert.Equal(t, payload, body)
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	assert.NoError(t, err)
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	assert.NoError(t, rsa.VerifyPKCS1v15(s.PublicKey(), stdcrypto.SHA256, sum[:], sig))
	assert.Equal(t, &k.PublicKey, s.PublicKey())

	// A changed payload does not pass the check (positive control of the verification).
	other := sha256.Sum256([]byte(parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte("{}"))))
	assert.Error(t, rsa.VerifyPKCS1v15(s.PublicKey(), stdcrypto.SHA256, other[:], sig))
}

// TestHeaderKidEscapes: the kid is a JSON string with the escapes of encoding/json v1.
func TestHeaderKidEscapes(t *testing.T) {
	h, err := headerJSON("a\"<b>&\u2028")
	assert.NoError(t, err)
	assert.Equal(t, "{\"alg\":\"RS256\",\"kid\":\"a\\\"\\u003cb\\u003e\\u0026\\u2028\",\"typ\":\"JWT\"}", string(h))
}

func TestNewSignerBadKeys(t *testing.T) {
	_, err := NewSigner([]byte("not a pem"), "qabz")
	assert.EqualError(t, err, "crypto: nil private key")

	_, err = NewSigner(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte{1, 2, 3}}), "qabz")
	assert.ErrorContains(t, err, "crypto: parse private key")

	// A PKCS #8 key is not read (bill reads PKCS #1 only).
	der, err := x509.MarshalPKCS8PrivateKey(key(t))
	if assert.NoError(t, err) {
		_, err = NewSigner(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), "qabz")
		assert.ErrorContains(t, err, "crypto: parse private key")
	}
}

func TestLoadSigner(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "key.pem")
	bad := filepath.Join(dir, "bad.pem")
	if !assert.NoError(t, os.WriteFile(good, pkcs1PEM(key(t)), 0o600)) || !assert.NoError(t, os.WriteFile(bad, []byte("x"), 0o600)) {
		return
	}
	s, err := LoadSigner(good, "qabz")
	assert.NoError(t, err)
	assert.NotNil(t, s)

	_, err = LoadSigner("", "qabz")
	assert.EqualError(t, err, "crypto: no private key path")
	_, err = LoadSigner(filepath.Join(dir, "missing.pem"), "qabz")
	assert.ErrorContains(t, err, "crypto: read private key "+filepath.Join(dir, "missing.pem"))
	_, err = LoadSigner(bad, "qabz")
	assert.EqualError(t, err, "crypto: nil private key (file "+bad+")")
}

func TestZeroSigner(t *testing.T) {
	var s *Signer
	_, err := s.Sign([]byte("{}"))
	assert.Error(t, err)
	assert.Nil(t, s.PublicKey())
	_, err = (&Signer{}).Sign([]byte("{}"))
	assert.Error(t, err)
}
