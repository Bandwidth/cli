package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const maxRedirectHops = 10

// RawResponse is a complete HTTP response. Unlike doRaw, it preserves status,
// headers, and the post-redirect URL — required for APIs that signal via 303
// and paginate through headers.
type RawResponse struct {
	StatusCode int
	Header     http.Header
	FinalURL   *url.URL
	Body       []byte
}

// sameOriginRedirect permits a redirect only when it stays on the same origin
// (scheme + host + effective port) as the request that triggered it. Go follows
// redirects automatically, so this must reject a hop *before* it is followed.
func sameOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > maxRedirectHops {
		return fmt.Errorf("too many redirects (>%d)", maxRedirectHops)
	}
	if len(via) == 0 {
		return nil
	}
	prev := via[len(via)-1].URL
	if !sameOrigin(prev, req.URL) {
		return fmt.Errorf("refusing cross-origin redirect from %s to %s", prev.Host, req.URL.Host)
	}
	return nil
}

func sameOrigin(a, b *url.URL) bool {
	return a.Scheme == b.Scheme && a.Hostname() == b.Hostname() && port(a) == port(b)
}

func port(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

// DoRawResponse executes a request and returns the full response, including
// non-2xx statuses (the caller decides how to interpret them). Transport
// failures still return an error.
func (c *Client) DoRawResponse(method, path string, body []byte) (*RawResponse, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := c.newRequest(method, path, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", c.contentTypeHeader())
	}
	req.Header.Set("Accept", c.contentTypeHeader())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	return &RawResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		FinalURL:   resp.Request.URL,
		Body:       data,
	}, nil
}

// contentTypeHeader maps the client's mode to a MIME type.
func (c *Client) contentTypeHeader() string {
	if c.contentType == "xml" {
		return "application/xml"
	}
	return "application/json"
}
