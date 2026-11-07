package auth_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/common/auth"
	"qabz/common/auth/authtest"
)

func newVerifier(t *testing.T, key authtest.Key) *auth.Verifier {
	t.Helper()
	pub, err := auth.ParsePublicKey(key.PublicPEM)
	if !assert.NoError(t, err) || !assert.NotNil(t, pub) {
		t.FailNow()
	}
	v, err := auth.NewVerifier(pub, authtest.Audience, func() time.Time { return authtest.Now })
	if !assert.NoError(t, err) || !assert.NotNil(t, v) {
		t.FailNow()
	}
	return v
}

// TestVerifyTable runs every JWT input of the shared table through the verifier.
func TestVerifyTable(t *testing.T) {
	key := authtest.NewKey(t)
	v := newVerifier(t, key)
	for _, tc := range authtest.Cases(t, key) {
		t.Run(tc.Name, func(t *testing.T) {
			claims, err := v.VerifyAuthorization(tc.Authorization)
			if tc.Valid {
				if assert.NoError(t, err) {
					assert.Equal(t, authtest.UserID, claims.UserID)
					assert.Equal(t, "09120000000", claims.Mobile)
					assert.Equal(t, "normal", claims.NonOTP)
				}
				return
			}
			if assert.Error(t, err) {
				assert.Equal(t, auth.Reason(tc.Reason), auth.ReasonOf(err))
			}
			assert.Equal(t, auth.Claims{}, claims)
		})
	}
}

func TestBearerToken(t *testing.T) {
	tok, err := auth.BearerToken("BEARER abc")
	assert.NoError(t, err)
	assert.Equal(t, "abc", tok)

	_, err = auth.BearerToken("Bearer\tabc")
	assert.Equal(t, auth.ReasonScheme, auth.ReasonOf(err))
}

func TestNewVerifierRejectsBadInput(t *testing.T) {
	key := authtest.NewKey(t)
	pub, err := auth.ParsePublicKey(key.PublicPEM)
	if !assert.NoError(t, err) {
		return
	}
	_, err = auth.NewVerifier(nil, authtest.Audience, nil)
	assert.Error(t, err)
	_, err = auth.NewVerifier(pub, "", nil)
	assert.Error(t, err)

	p256, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if assert.NoError(t, err) && assert.NotNil(t, p256) {
		_, err = auth.NewVerifier(&p256.PublicKey, authtest.Audience, nil)
		assert.Error(t, err)
	}

	v, err := auth.NewVerifier(pub, authtest.Audience, nil) // nil clock: the real clock
	assert.NoError(t, err)
	assert.NotNil(t, v)
}

func TestParsePublicKeyRejectsBadKeys(t *testing.T) {
	_, err := auth.ParsePublicKey([]byte("not pem"))
	assert.Error(t, err)

	_, err = auth.ParsePublicKey(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte{1}}))
	assert.Error(t, err)

	_, err = auth.ParsePublicKey(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte{1, 2, 3}}))
	assert.Error(t, err)

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if assert.NoError(t, err) && assert.NotNil(t, rsaKey) {
		der, err := x509.MarshalPKIXPublicKey(&rsaKey.PublicKey)
		if assert.NoError(t, err) {
			_, err = auth.ParsePublicKey(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
			assert.Error(t, err)
		}
	}

	p256, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if assert.NoError(t, err) && assert.NotNil(t, p256) {
		der, err := x509.MarshalPKIXPublicKey(&p256.PublicKey)
		if assert.NoError(t, err) {
			_, err = auth.ParsePublicKey(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
			assert.Error(t, err)
		}
	}
}

func TestLoadVerifier(t *testing.T) {
	key := authtest.NewKey(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "jwt.pub.pem")
	if !assert.NoError(t, os.WriteFile(path, key.PublicPEM, 0o600)) {
		return
	}
	v, err := auth.LoadVerifier(path, authtest.Audience)
	assert.NoError(t, err)
	assert.NotNil(t, v)

	_, err = auth.LoadVerifier("", authtest.Audience)
	assert.Error(t, err)
	_, err = auth.LoadVerifier(filepath.Join(dir, "missing.pem"), authtest.Audience)
	assert.Error(t, err)

	bad := filepath.Join(dir, "bad.pem")
	if assert.NoError(t, os.WriteFile(bad, []byte("x"), 0o600)) {
		_, err = auth.LoadVerifier(bad, authtest.Audience)
		assert.Error(t, err)
	}
}

func TestVerifyOnZeroVerifier(t *testing.T) {
	var v *auth.Verifier
	_, err := v.Verify("a.b.c")
	assert.Error(t, err)
	_, err = (&auth.Verifier{}).Verify("a.b.c")
	assert.Error(t, err)
}

func TestErrorAndReasonOf(t *testing.T) {
	assert.Equal(t, auth.Reason(""), auth.ReasonOf(errors.New("other")))
	assert.Equal(t, auth.Reason(""), auth.ReasonOf(nil))
	var e *auth.Error
	assert.Equal(t, "auth: rejected", e.Error())
	assert.Equal(t, "auth: rejected (guest)", (&auth.Error{Reason: auth.ReasonGuest}).Error())
}

func TestClaimsContext(t *testing.T) {
	_, ok := auth.ClaimsFrom(context.Background())
	assert.False(t, ok)
	//nolint:staticcheck // a nil context is the case under test
	_, ok = auth.ClaimsFrom(nil)
	assert.False(t, ok)

	want := auth.Claims{UserID: authtest.UserID, Mobile: "0912", NonOTP: "normal"}
	got, ok := auth.ClaimsFrom(auth.WithClaims(context.Background(), want))
	assert.True(t, ok)
	assert.Equal(t, want, got)

	//nolint:staticcheck // a nil context is the case under test
	got, ok = auth.ClaimsFrom(auth.WithClaims(nil, want))
	assert.True(t, ok)
	assert.Equal(t, want, got)
}
