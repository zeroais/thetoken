package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// RotateTokenKey replaces a key with a new one and returns it once (TOKEN:
// keys are stored hashed, so a lost key is rotated, never revealed).
func RotateTokenKey(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	userId := c.GetInt("id")
	params := tokenAuditParams(c)
	params["id"] = id
	key, err := model.RotateTokenKey(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.SetContextKey(c, constant.ContextKeyTokenAuditSucceeded, true)
	common.ApiSuccess(c, gin.H{"id": id, "key": key}) // the key carries its own prefix
}
