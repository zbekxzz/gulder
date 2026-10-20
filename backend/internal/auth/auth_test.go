package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPasswordHashing(t *testing.T) {
	h, err := HashPassword("correct horse")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(h, "$argon2id$v=19$"))

	ok, err := VerifyPassword("correct horse", h)
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = VerifyPassword("wrong", h)
	require.NoError(t, err)
	assert.False(t, ok)

	h2, _ := HashPassword("correct horse")
	assert.NotEqual(t, h, h2, "соль случайная")

	_, err = VerifyPassword("x", "garbage")
	assert.ErrorIs(t, err, ErrInvalidHash)
}

func TestTokens(t *testing.T) {
	iss := NewIssuer([]byte(strings.Repeat("s", 32)), time.Minute, time.Hour)
	shop := int64(3)
	toks, err := iss.Issue(Principal{UserID: 7, Role: RoleFlorist, ShopID: &shop})
	require.NoError(t, err)

	p, err := iss.Parse(toks.AccessToken, TokenAccess)
	require.NoError(t, err)
	assert.Equal(t, int64(7), p.UserID)
	assert.Equal(t, RoleFlorist, p.Role)
	assert.Equal(t, int64(3), *p.ShopID)

	_, err = iss.Parse(toks.RefreshToken, TokenAccess)
	assert.ErrorIs(t, err, ErrInvalidToken, "refresh нельзя использовать как access")

	other := NewIssuer([]byte(strings.Repeat("x", 32)), time.Minute, time.Hour)
	_, err = other.Parse(toks.AccessToken, TokenAccess)
	assert.ErrorIs(t, err, ErrInvalidToken, "чужая подпись")

	iss.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	_, err = iss.Parse(toks.AccessToken, TokenAccess)
	assert.ErrorIs(t, err, ErrInvalidToken, "истёкший токен")
}
