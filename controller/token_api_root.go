package controller

import (
	"html"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// TOKEN: GET /v1 is the API base URL people paste into a browser. Instead of
// "Invalid URL (GET /v1)", it says what the URL is for and links the docs
// (System Settings docs_link, else TOKEN's docs). Browsers get a short page,
// API clients the same text as JSON.
const tokenDocsURL = "https://thetoken.dev/docs"

func TokenAPIRoot(c *gin.Context) {
	docs := operation_setting.GetGeneralSetting().DocsLink
	if docs == "" {
		docs = tokenDocsURL
	}
	message := "This is the TOKEN API base URL. Send OpenAI-compatible requests to /v1/chat/completions with your API key. Documentation: " + docs
	c.Header("Cache-Control", "public, max-age=300")
	if !strings.Contains(c.GetHeader("Accept"), "text/html") {
		c.JSON(http.StatusOK, gin.H{"message": message, "docs": docs})
		return
	}
	link := html.EscapeString(docs)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`<!doctype html><html lang="en"><head><meta charset="utf-8">`+
		`<meta name="viewport" content="width=device-width, initial-scale=1"><title>TOKEN API</title>`+
		`<style>body{font:16px/1.5 system-ui,sans-serif;max-width:40rem;margin:4rem auto;padding:0 1rem;color:#111;background:#fff}`+
		`code{background:#f2f2f2;padding:.1rem .3rem;border-radius:4px}a{color:#0b5cd5}`+
		`@media (prefers-color-scheme:dark){body{color:#eee;background:#111}code{background:#222}a{color:#7ab0ff}}</style></head>`+
		`<body><h1>TOKEN API</h1><p>This is the API base URL, <code>https://api.thetoken.dev/v1</code>. `+
		`It answers API requests made with your API key, for example <code>POST /v1/chat/completions</code>; there is nothing to see in a browser.</p>`+
		`<p>Read the documentation at <a href="`+link+`">`+link+`</a>.</p></body></html>`))
}
