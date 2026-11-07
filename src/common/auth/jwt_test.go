package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/common/auth"
	"qabz/common/auth/authtest"
)

// TestVerifyTable runs every JWT input of the shared table through the verifier.
func TestVerifyTable(t *testing.T) {
	v := authtest.Verifier(t)
	for _, tc := range authtest.Cases(t) {
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
	_, err := auth.NewVerifier(nil, authtest.Audience, nil)
	assert.Error(t, err)
	_, err = auth.NewVerifier([]byte{}, authtest.Audience, nil)
	assert.Error(t, err)
	_, err = auth.NewVerifier([]byte(authtest.Secret), "", nil)
	assert.Error(t, err)

	v, err := auth.NewVerifier([]byte(authtest.Secret), authtest.Audience, nil) // nil clock: the real clock
	assert.NoError(t, err)
	assert.NotNil(t, v)
}

// TestNewVerifierCopiesSecret: a change to the caller's slice after NewVerifier does not change the
// secret the verifier uses.
func TestNewVerifierCopiesSecret(t *testing.T) {
	secret := []byte(authtest.Secret)
	v, err := auth.NewVerifier(secret, authtest.Audience, func() time.Time { return authtest.Now })
	if !assert.NoError(t, err) || !assert.NotNil(t, v) {
		t.FailNow()
	}
	for i := range secret {
		secret[i] = 'x'
	}
	token := authtest.Sign(t, authtest.Secret, authtest.Header("HS256"), authtest.ValidClaims())
	_, err = v.Verify(token)
	assert.NoError(t, err)
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
