package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A second instance that started before setup still has constant.Setup
// false; it must treat the persisted setup record as completed setup and
// refuse to change the operation modes again.
func TestSetupOnAnotherInstanceIsRefusedOnceTheSetupRecordExists(t *testing.T) {
	previousDB, previousLogDB, previousSetup := model.DB, model.LOG_DB, constant.Setup
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Setup{}, &model.User{}, &model.Option{}))
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB, constant.Setup = previousDB, previousLogDB, previousSetup
		common.RedisEnabled = previousRedis
	})
	require.NoError(t, db.Create(&model.User{Username: "root", Password: "x", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Setup{Version: "v1", InitializedAt: 1}).Error)
	constant.Setup = false // this instance started before setup

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/setup", strings.NewReader(`{"DemoSiteEnabled":true,"SelfUseModeEnabled":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	PostSetup(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	var options int64
	require.NoError(t, db.Model(&model.Option{}).Where("key IN ?", []string{"DemoSiteEnabled", "SelfUseModeEnabled"}).Count(&options).Error)
	assert.Zero(t, options, "operation modes stay untouched")
	assert.True(t, constant.Setup, "the instance learns that setup is done")

	recorder = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/setup", nil)
	GetSetup(c)
	assert.Contains(t, recorder.Body.String(), `"status":true`)
}
