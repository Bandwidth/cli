package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Bandwidth/cli/internal/auth"
)

// Version is set at startup from the build-time version injected by GoReleaser.
// Falls back to "dev" for local builds without ldflags.
var Version = "dev"

func userAgent() string {
	return "band-cli/" + Version
}

// APIError represents a non-2xx HTTP response.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	body := strings.TrimSpace(e.Body)
	if body == "" {
		if status := http.StatusText(e.StatusCode); status != "" {
			body = status
		} else {
			body = "(empty response body)"
		}
	}
	return fmt.Sprintf("API error %d: %s", e.StatusCode, body)
}

// Requester is the interface satisfied by Client. Commands accept this so
// tests can substitute a mock without hitting real Bandwidth APIs.
type Requester interface {
	Get(path string, result interface{}) error
	Post(path string, body, result interface{}) error
	Put(path string, body, result interface{}) error
	Patch(path string, body, result interface{}) error
	Delete(path string, result interface{}) error
	GetRaw(path string) ([]byte, error)
	PutRaw(path string, data []byte, contentType string) error
}

// Client is an authenticated HTTP client for Bandwidth APIs.
type Client struct {
	BaseURL       string
	httpClient    *http.Client
	tm            *auth.TokenManager
	contentType   string // "json" (default) or "xml"
	basicUser     string // if set, use Basic Auth instead of Bearer
	basicPassword string
}

// NewClient creates a Client that obtains Bearer tokens via the given TokenManager.
// Defaults to JSON mode.
func NewClient(baseURL string, tm *auth.TokenManager) *Client {
	return &Client{
		BaseURL:     baseURL,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		tm:          tm,
		contentType: "json",
	}
}

// NewXMLClient creates a Client configured for XML serialization (Dashboard API).
func NewXMLClient(baseURL string, tm *auth.TokenManager) *Client {
	return &Client{
		BaseURL:     baseURL,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		tm:          tm,
		contentType: "xml",
	}
}

// NewBasicAuthClient creates a Client that uses HTTP Basic Authentication.
func NewBasicAuthClient(baseURL, username, password string) *Client {
	return &Client{
		BaseURL:       baseURL,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		contentType:   "json",
		basicUser:     username,
		basicPassword: password,
	}
}

// NewClientNoAuth creates a Client without authentication (for Build registration).
func NewClientNoAuth(baseURL string) *Client {
	return &Client{
		BaseURL:     baseURL,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		contentType: "json",
	}
}

// newRequest creates an authenticated HTTP request with standard headers.
func (c *Client) newRequest(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	if c.basicUser != "" {
		req.SetBasicAuth(c.basicUser, c.basicPassword)
	} else if c.tm != nil {
		token, err := c.tm.GetToken()
		if err != nil {
			return nil, fmt.Errorf("obtaining auth token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("User-Agent", userAgent())
	return req, nil
}

// doRaw executes a request and returns the raw response bytes, checking for non-2xx status.
func (c *Client) doRaw(req *http.Request) ([]byte, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(data)}
	}
	return data, nil
}

// do executes an HTTP request and unmarshals the response into result.
// result may be nil (e.g., for 204 No Content responses).
func (c *Client) do(method, path string, reqBody, result interface{}) error {
	var bodyReader io.Reader
	var contentTypeHeader string

	if reqBody != nil {
		if c.contentType == "xml" {
			xmlb, ok := reqBody.(XMLBody)
			if !ok {
				return fmt.Errorf("XML client requires XMLBody for request body, got %T", reqBody)
			}
			data, err := MapToXML(xmlb.RootElement, xmlb.Data)
			if err != nil {
				return fmt.Errorf("marshaling XML request body: %w", err)
			}
			bodyReader = bytes.NewReader(data)
			contentTypeHeader = "application/xml"
		} else {
			data, err := json.Marshal(reqBody)
			if err != nil {
				return fmt.Errorf("marshaling request body: %w", err)
			}
			bodyReader = bytes.NewReader(data)
			contentTypeHeader = "application/json"
		}
	}

	req, err := c.newRequest(method, path, bodyReader)
	if err != nil {
		return err
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", contentTypeHeader)
	}

	respBody, err := c.doRaw(req)
	if err != nil {
		return err
	}

	if result != nil && len(respBody) > 0 {
		if c.contentType == "xml" {
			m, err := XMLToMap(respBody)
			if err != nil {
				return fmt.Errorf("unmarshaling XML response: %w", err)
			}
			switch r := result.(type) {
			case *interface{}:
				*r = m
			case *map[string]interface{}:
				*r = m
			default:
				if err := json.Unmarshal(respBody, result); err != nil {
					return fmt.Errorf("unmarshaling response: %w", err)
				}
			}
		} else {
			if err := json.Unmarshal(respBody, result); err != nil {
				return fmt.Errorf("unmarshaling response: %w", err)
			}
		}
	}

	return nil
}

// Get performs a GET request and unmarshals the response into result.
func (c *Client) Get(path string, result interface{}) error {
	return c.do(http.MethodGet, path, nil, result)
}

// Post performs a POST request with body and unmarshals the response into result.
func (c *Client) Post(path string, body, result interface{}) error {
	return c.do(http.MethodPost, path, body, result)
}

// Put performs a PUT request with body and unmarshals the response into result.
func (c *Client) Put(path string, body, result interface{}) error {
	return c.do(http.MethodPut, path, body, result)
}

// Patch performs a PATCH request with body and unmarshals the response into result.
func (c *Client) Patch(path string, body, result interface{}) error {
	return c.do(http.MethodPatch, path, body, result)
}

// Delete performs a DELETE request and unmarshals the response into result.
func (c *Client) Delete(path string, result interface{}) error {
	return c.do(http.MethodDelete, path, nil, result)
}

// GetRaw performs a GET request and returns the raw response bytes.
// Useful for file downloads like recordings.
func (c *Client) GetRaw(path string) ([]byte, error) {
	req, err := c.newRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	return c.doRaw(req)
}

// PutRaw performs a PUT request with raw binary data and a custom content type.
// Useful for uploading files like MMS media.
func (c *Client) PutRaw(path string, data []byte, contentType string) error {
	req, err := c.newRequest(http.MethodPut, path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	_, err = c.doRaw(req)
	return err
}
