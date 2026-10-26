package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

// SetTokenPublicRouter adds TOKEN's public, unauthenticated data endpoints
// (aggregates only), served on the API host for the website, and a friendly
// answer for the API base URL opened in a browser, and the GitHub secret
// scanning partner endpoint.
func SetTokenPublicRouter(router *gin.Engine) {
	router.GET("/public/metrics", middleware.GlobalAPIRateLimit(), controller.TokenPublicMetrics)
	router.GET("/v1", middleware.GlobalAPIRateLimit(), controller.TokenAPIRoot)
	// GitHub secret scanning reports leaked TOKEN keys here (signed by GitHub).
	router.POST("/public/github-secret-scanning", middleware.GlobalAPIRateLimit(), controller.TokenGitHubSecretScanning)
}
