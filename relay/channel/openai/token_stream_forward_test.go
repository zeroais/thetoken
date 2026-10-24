package openai

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lockedWriter is a ResponseWriter the test can read while the handler writes.
type lockedWriter struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	header http.Header
}

func (w *lockedWriter) Header() http.Header { return w.header }
func (w *lockedWriter) WriteHeader(int)     {}
func (w *lockedWriter) Flush()              {}
func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}
func (w *lockedWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// TOKEN: OpenAI-format stream events are forwarded as they arrive (not one
// event later), and the final usage event is still handled exactly once.
func TestOaiStreamForwardsEventsImmediately(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if constant.StreamingTimeout <= 0 {
		prev := constant.StreamingTimeout
		constant.StreamingTimeout = 60
		t.Cleanup(func() { constant.StreamingTimeout = prev })
	}
	for _, includeUsage := range []bool{false, true} {
		w := &lockedWriter{header: http.Header{}}
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		info := &relaycommon.RelayInfo{
			OriginModelName: "m", RelayFormat: types.RelayFormatOpenAI, RelayMode: relayconstant.RelayModeChatCompletions,
			IsStream: true, DisablePing: true, StartTime: time.Now(), ShouldIncludeUsage: includeUsage,
			ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "m"},
		}
		pr, pw := io.Pipe()
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, apiErr := OaiStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr})
			assert.Nil(t, apiErr)
		}()

		event := func(s string) { _, _ = io.WriteString(pw, "data: "+s+"\n\n") }
		event(`{"id":"x","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"content":"FIRST"}}]}`)
		require.Eventually(t, func() bool { return strings.Contains(w.String(), "FIRST") }, time.Second, 5*time.Millisecond,
			"first token forwarded before the next event arrives")
		event(`{"id":"x","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"content":"SECOND"},"finish_reason":"stop"}]}`)
		event(`{"id":"x","object":"chat.completion.chunk","model":"m","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`)
		event(`[DONE]`)
		_ = pw.Close()
		<-done

		out := w.String()
		assert.Equal(t, 1, strings.Count(out, "FIRST"), "includeUsage=%v", includeUsage)
		assert.Equal(t, 1, strings.Count(out, "SECOND"), "includeUsage=%v", includeUsage)
		usageEvents := strings.Count(out, `"prompt_tokens":3`)
		if includeUsage {
			assert.Equal(t, 1, usageEvents, "usage event sent once when requested")
		} else {
			assert.Zero(t, usageEvents, "usage-only event dropped when not requested")
		}
		assert.Equal(t, 1, strings.Count(out, "[DONE]"))
	}
}

// TOKEN: an Anthropic Messages client (Claude Code) always gets a closed
// stream, even when the upstream ends without its finish chunk (seen in
// production): the blocks are stopped and message_delta carries
// the stop reason and the counted output tokens before message_stop.
func TestClaudeStreamIsClosedWhenUpstreamEndsWithoutFinishChunk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if constant.StreamingTimeout <= 0 {
		prev := constant.StreamingTimeout
		constant.StreamingTimeout = 60
		t.Cleanup(func() { constant.StreamingTimeout = prev })
	}
	for name, ending := range map[string]string{
		"finish chunk with usage": `data: {"id":"x","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":90,"completion_tokens":13,"total_tokens":103}}` + "\n\n",
		"no finish chunk":         "",
	} {
		t.Run(name, func(t *testing.T) {
			w := &lockedWriter{header: http.Header{}}
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest(http.MethodPost, "/v1/messages", nil)
			info := &relaycommon.RelayInfo{
				OriginModelName: "m", RelayFormat: types.RelayFormatClaude, RelayMode: relayconstant.RelayModeChatCompletions,
				IsStream: true, DisablePing: true, StartTime: time.Now(),
				ChannelMeta:       &relaycommon.ChannelMeta{UpstreamModelName: "m"},
				ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone},
			}
			upstream := `data: {"id":"x","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"reasoning_content":"Thinking"}}]}` + "\n\n" +
				`data: {"id":"x","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"content":"OK"}}]}` + "\n\n" +
				ending + "data: [DONE]\n\n"
			usage, apiErr := OaiStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(upstream))})
			require.Nil(t, apiErr)
			require.NotNil(t, usage)

			out := w.String()
			var events []string
			for line := range strings.SplitSeq(out, "\n") {
				if event, ok := strings.CutPrefix(line, "event: "); ok && (len(events) == 0 || events[len(events)-1] != event) {
					events = append(events, event)
				}
			}
			assert.Equal(t, []string{"message_start", "content_block_start", "content_block_delta", "content_block_stop",
				"content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"}, events)
			assert.Contains(t, out, `"stop_reason":"end_turn"`)
			assert.Contains(t, out, fmt.Sprintf(`"output_tokens":%d`, usage.CompletionTokens))
			assert.Positive(t, usage.CompletionTokens)
			assert.Equal(t, 1, strings.Count(out, "event: message_start"))
			assert.Equal(t, 1, strings.Count(out, "event: message_stop"))
		})
	}
}
