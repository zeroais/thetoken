package controller

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TOKEN: a leak report signed by GitHub disables the live key it names and
// labels it a true positive; a well-formed key that is not ours is a false
// positive; an unsigned or wrongly signed report changes nothing.
func TestGitHubSecretScanningRevokesLeakedKeys(t *testing.T) {
	_, user := setupMoyasar(t, "") // a database with users, keys and logs

	githubKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&githubKey.PublicKey)
	require.NoError(t, err)
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	keys := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		data, _ := common.Marshal(map[string]any{"public_keys": []map[string]any{{"key_identifier": "gh-1", "key": pemKey, "is_current": true}}})
		_, _ = w.Write(data)
	}))
	t.Cleanup(keys.Close)
	savedURL := tokenGitHubKeysURL
	tokenGitHubKeysURL = keys.URL
	t.Cleanup(func() { tokenGitHubKeysURL = savedURL })

	leaked, err := common.GenerateTokenAPIKey()
	require.NoError(t, err)
	stranger, err := common.GenerateTokenAPIKey()
	require.NoError(t, err)
	key := model.Token{UserId: user.Id, Name: "laptop", Key: leaked, Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, model.DB.Create(&key).Error)

	body, err := common.Marshal([]githubSecretMatch{
		{Token: leaked, Type: "tkn_api_key", URL: "https://github.com/someone/repo/blob/main/.env", Source: "content"},
		{Token: stranger, Type: "tkn_api_key", URL: "https://gist.github.com/x", Source: "gist"},
	})
	require.NoError(t, err)
	post := func(keyID string, sign *ecdsa.PrivateKey) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/public/github-secret-scanning", bytes.NewReader(body))
		digest := sha256.Sum256(body)
		sig, err := ecdsa.SignASN1(rand.Reader, sign, digest[:])
		require.NoError(t, err)
		c.Request.Header.Set("Github-Public-Key-Identifier", keyID)
		c.Request.Header.Set("Github-Public-Key-Signature", base64.StdEncoding.EncodeToString(sig))
		TokenGitHubSecretScanning(c)
		return rec
	}

	forger, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, post("gh-1", forger).Code, "not signed by GitHub")
	assert.Equal(t, http.StatusUnauthorized, post("gh-unknown", githubKey).Code, "unknown key identifier")
	var still model.Token
	require.NoError(t, model.DB.First(&still, key.Id).Error)
	assert.Equal(t, common.TokenStatusEnabled, still.Status)

	rec := post("gh-1", githubKey)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var labels []githubSecretLabel
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &labels))
	assert.Equal(t, []githubSecretLabel{
		{TokenRaw: leaked, TokenType: "tkn_api_key", Label: "true_positive"},
		{TokenRaw: stranger, TokenType: "tkn_api_key", Label: "false_positive"},
	}, labels)
	var revoked model.Token
	require.NoError(t, model.DB.First(&revoked, key.Id).Error)
	assert.Equal(t, common.TokenStatusDisabled, revoked.Status, "a leaked key is disabled at once")
	var logged int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND content LIKE ?", user.Id, "%found in public on GitHub%").Count(&logged).Error)
	assert.EqualValues(t, 1, logged)
}
