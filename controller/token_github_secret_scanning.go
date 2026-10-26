package controller

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// TOKEN: GitHub secret scanning partner endpoint
// (POST https://api.thetoken.dev/public/github-secret-scanning). When GitHub
// finds a TOKEN API key ("thetoken_…", common/token_api_key.go) in a public
// repository, gist or issue, it posts the matches here. Each request is
// signed by GitHub (ECDSA P-256, SHA-256) with a key published at
// tokenGitHubKeysURL; unsigned or wrongly signed requests are refused. A
// match that is a live key is treated as compromised: the key is disabled at
// once, the event is logged on the account and the account's owner and the
// operator are notified. The answer labels each match true or false
// positive, as the program's feedback format asks.
var tokenGitHubKeysURL = "https://api.github.com/meta/public_keys/secret_scanning"

type githubSecretMatch struct {
	Token  string `json:"token"`
	Type   string `json:"type"`
	URL    string `json:"url"`
	Source string `json:"source"`
}

type githubSecretLabel struct {
	TokenRaw  string `json:"token_raw"`
	TokenType string `json:"token_type"`
	Label     string `json:"label"`
}

var tokenGitHubKeys = struct {
	sync.Mutex
	keys    map[string]*ecdsa.PublicKey
	fetched time.Time
}{}

func TokenGitHubSecretScanning(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if err := verifyGitHubSignature(c.GetHeader("Github-Public-Key-Identifier"), c.GetHeader("Github-Public-Key-Signature"), body); err != nil {
		logger.LogWarn(c.Request.Context(), "GitHub secret scanning: refused: "+err.Error())
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var matches []githubSecretMatch
	if err := common.Unmarshal(body, &matches); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	labels := make([]githubSecretLabel, 0, len(matches))
	for _, m := range matches {
		label := "false_positive"
		if common.IsTokenAPIKey(m.Token) && revokeLeakedTokenKey(c, m) {
			label = "true_positive"
		}
		labels = append(labels, githubSecretLabel{TokenRaw: m.Token, TokenType: m.Type, Label: label})
	}
	c.JSON(http.StatusOK, labels)
}

// revokeLeakedTokenKey disables the key a match names, if it exists, and
// tells its owner and the operator. It reports whether the key was live.
func revokeLeakedTokenKey(c *gin.Context, m githubSecretMatch) bool {
	token, err := model.GetTokenByKey(model.HashTokenKey(m.Token), true)
	if err != nil || token == nil {
		return false
	}
	if token.Status == common.TokenStatusEnabled {
		if err := model.DB.Model(&model.Token{}).Where("id = ?", token.Id).Update("status", common.TokenStatusDisabled).Error; err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("GitHub secret scanning: key #%d not disabled: %v", token.Id, err))
		}
		_ = model.InvalidateUserTokensCache(token.UserId)
	}
	summary := fmt.Sprintf("API key #%d (%s) was found in public on GitHub (%s, %s) and has been disabled. Create a new key and remove the old one from that place.",
		token.Id, token.Name, m.Source, m.URL)
	model.RecordLog(token.UserId, model.LogTypeManage, summary)
	logger.LogWarn(c.Request.Context(), "GitHub secret scanning: "+summary)
	if email, err := model.GetUserEmail(token.UserId); err == nil && email != "" {
		if err := common.SendEmail("Your TOKEN API key was found in public and disabled", email,
			"<p>"+summary+"</p><p>Anyone who found the key could have used it. Check your usage logs for requests you did not make.</p>"); err != nil {
			logger.LogError(c.Request.Context(), "GitHub secret scanning: owner not emailed: "+err.Error())
		}
	}
	service.NotifyRootUser(tokenReversalNotifyType, "TOKEN API key leaked on GitHub", summary)
	return true
}

func verifyGitHubSignature(keyID, signature string, body []byte) error {
	if keyID == "" || signature == "" {
		return errors.New("missing signature headers")
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return errors.New("signature is not base64")
	}
	pub, err := githubPublicKey(keyID)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(body)
	if !ecdsa.VerifyASN1(pub, digest[:], sig) {
		return errors.New("bad signature")
	}
	return nil
}

// githubPublicKey returns GitHub's secret scanning key by identifier, from
// a copy refreshed hourly (or at once for an identifier not seen yet).
func githubPublicKey(keyID string) (*ecdsa.PublicKey, error) {
	tokenGitHubKeys.Lock()
	defer tokenGitHubKeys.Unlock()
	if key := tokenGitHubKeys.keys[keyID]; key != nil && time.Since(tokenGitHubKeys.fetched) < time.Hour {
		return key, nil
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(tokenGitHubKeysURL)
	if err != nil {
		return nil, fmt.Errorf("GitHub keys: %w", err)
	}
	defer resp.Body.Close()
	var list struct {
		PublicKeys []struct {
			KeyIdentifier string `json:"key_identifier"`
			Key           string `json:"key"`
		} `json:"public_keys"`
	}
	if err := common.DecodeJson(resp.Body, &list); err != nil {
		return nil, fmt.Errorf("GitHub keys: %w", err)
	}
	keys := map[string]*ecdsa.PublicKey{}
	for _, k := range list.PublicKeys {
		block, _ := pem.Decode([]byte(k.Key))
		if block == nil {
			continue
		}
		parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			continue
		}
		if ecKey, ok := parsed.(*ecdsa.PublicKey); ok {
			keys[k.KeyIdentifier] = ecKey
		}
	}
	tokenGitHubKeys.keys, tokenGitHubKeys.fetched = keys, time.Now()
	if keys[keyID] == nil {
		return nil, errors.New("unknown GitHub key identifier")
	}
	return keys[keyID], nil
}
