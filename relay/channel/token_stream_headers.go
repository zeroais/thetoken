package channel

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	common2 "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/common"
)

// TOKEN: a provider that accepts a streaming request and then sends nothing
// (seen with tool-call requests: no headers after 200 s, while the same
// request answered in 1 s before and after) held the
// customer for RELAY_RESPONSE_HEADER_TIMEOUT (300 s). Streaming responses send
// their headers within seconds, so a streaming request that has none after
// TOKEN_STREAM_HEADER_TIMEOUT seconds (default 60; 0 turns it off) fails as
// do_request_failed (500) and the relay retries it. Once the headers arrive
// the stream itself is not limited. Non-streaming requests keep the global
// header timeout: their headers only come after the whole answer.
var tokenStreamHeaderTimeout = time.Duration(common2.GetEnvOrDefault("TOKEN_STREAM_HEADER_TIMEOUT", 60)) * time.Second

// tokenBoundStreamHeaders returns the request to send and a function to pass
// its result through.
func tokenBoundStreamHeaders(req *http.Request, info *common.RelayInfo) (*http.Request, func(*http.Response, error) (*http.Response, error)) {
	if !info.IsStream || tokenStreamHeaderTimeout <= 0 {
		return req, func(resp *http.Response, err error) (*http.Response, error) { return resp, err }
	}
	ctx, cancel := context.WithCancel(req.Context())
	timer := time.AfterFunc(tokenStreamHeaderTimeout, cancel)
	return req.WithContext(ctx), func(resp *http.Response, err error) (*http.Response, error) {
		fired := !timer.Stop()
		if err != nil {
			cancel()
			if fired {
				err = fmt.Errorf("no response headers within %s: %w", tokenStreamHeaderTimeout, err)
			}
			return resp, err
		}
		// The request context must live as long as the stream is read.
		resp.Body = cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
		return resp, nil
	}
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b cancelOnClose) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}
