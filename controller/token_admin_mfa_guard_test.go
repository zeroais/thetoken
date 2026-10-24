package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefuseRemovingLastAdminFactor(t *testing.T) {
	customer, _ := setupStripeReversalDB(t) // fresh SQLite DB with a root and a common user
	require.NoError(t, model.DB.AutoMigrate(&model.TwoFA{}, &model.PasskeyCredential{}))
	gin.SetMode(gin.TestMode)
	security := operation_setting.GetTokenSecuritySetting()
	saved := *security
	t.Cleanup(func() { *security = saved })
	security.RequireAdminMFA = true

	var root model.User
	require.NoError(t, model.DB.Where("role = ?", common.RoleRootUser).First(&root).Error)
	require.NoError(t, model.DB.Create(&model.TwoFA{UserId: root.Id, Secret: "s", IsEnabled: true}).Error)

	refused := func(userID int, removing string) (bool, int) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		return refuseRemovingLastAdminFactor(c, userID, removing), w.Code
	}

	got, code := refused(root.Id, "totp")
	assert.True(t, got, "root's only factor cannot be removed")
	assert.Equal(t, http.StatusConflict, code)
	got, _ = refused(customer.Id, "totp")
	assert.False(t, got, "common users may remove their factor")

	require.NoError(t, model.DB.Create(&model.PasskeyCredential{UserID: root.Id, CredentialID: "cred", PublicKey: "pk"}).Error)
	got, _ = refused(root.Id, "totp")
	assert.False(t, got, "TOTP can go once a passkey remains")

	security.RequireAdminMFA = false
	require.NoError(t, model.DB.Where("user_id = ?", root.Id).Delete(&model.PasskeyCredential{}).Error)
	got, _ = refused(root.Id, "totp")
	assert.False(t, got, "setting off: upstream behavior")
}
