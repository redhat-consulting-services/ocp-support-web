package k8s

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client provides low-level HTTP access to the Kubernetes API.
type Client struct {
	APIURL     string
	Token      string
	HTTPClient *http.Client
}

// NewClient creates a Kubernetes API client using Bearer token authentication.
func NewClient(apiURL, token string, insecureSkipTLS bool) *Client {
	return &Client{
		APIURL: apiURL,
		Token:  token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipTLS},
			},
		},
	}
}

func (c *Client) newRequest(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, c.APIURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// GetAsUser fetches a JSON resource using Kubernetes user impersonation.
// Groups should include the user's group memberships (e.g. from X-Forwarded-Groups)
// so that RBAC bindings via groups are respected.
func (c *Client) GetAsUser(path, username string, groups []string) (map[string]interface{}, error) {
	req, err := c.newRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Impersonate-User", username)
	for _, g := range groups {
		req.Header.Add("Impersonate-Group", g)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &StatusError{Code: resp.StatusCode, Body: string(body)}
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// Get fetches a JSON resource and returns the parsed result.
func (c *Client) Get(path string) (map[string]interface{}, error) {
	req, err := c.newRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &StatusError{Code: resp.StatusCode, Body: string(body)}
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetRaw fetches a resource and returns the raw bytes.
func (c *Client) GetRaw(path string) ([]byte, error) {
	req, err := c.newRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &StatusError{Code: resp.StatusCode, Body: string(body)}
	}

	return io.ReadAll(resp.Body)
}

// GetStream fetches a resource and returns a streaming reader. Caller must close.
func (c *Client) GetStream(path string) (io.ReadCloser, error) {
	req, err := c.newRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &StatusError{Code: resp.StatusCode, Body: string(body)}
	}

	return resp.Body, nil
}

// GetList fetches a list resource with automatic pagination.
func (c *Client) GetList(path string) ([]map[string]interface{}, error) {
	var allItems []map[string]interface{}
	continueToken := ""

	for {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		pagePath := path + sep + "limit=500"
		if continueToken != "" {
			pagePath += "&continue=" + continueToken
		}

		result, err := c.Get(pagePath)
		if err != nil {
			return nil, err
		}

		items := JsonArray(result, "items")
		for _, item := range items {
			if m, ok := item.(map[string]interface{}); ok {
				allItems = append(allItems, m)
			}
		}

		ct := JsonPath(result, "metadata", "continue")
		if ct == "" {
			break
		}
		continueToken = ct
	}

	return allItems, nil
}

// Post sends a JSON body and returns the parsed response.
func (c *Client) Post(path string, body interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := c.newRequest("POST", path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &StatusError{Code: resp.StatusCode, Body: string(respBody)}
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Patch sends a PATCH request with merge-patch content type.
func (c *Client) Patch(path string, body interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := c.newRequest("PATCH", path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/merge-patch+json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &StatusError{Code: resp.StatusCode, Body: string(respBody)}
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Put sends a PUT request (update/replace).
func (c *Client) Put(path string, body interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := c.newRequest("PUT", path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &StatusError{Code: resp.StatusCode, Body: string(respBody)}
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Delete sends a DELETE request.
func (c *Client) Delete(path string) error {
	req, err := c.newRequest("DELETE", path, nil)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &StatusError{Code: resp.StatusCode, Body: string(body)}
	}

	return nil
}

// StatusError represents an HTTP error response from the API.
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.Code, e.Body)
}

// IsNotFound returns true if the error is a 404.
func IsNotFound(err error) bool {
	if se, ok := err.(*StatusError); ok {
		return se.Code == 404
	}
	return false
}

// IsForbidden returns true if the error is a 403.
func IsForbidden(err error) bool {
	if se, ok := err.(*StatusError); ok {
		return se.Code == 403
	}
	return false
}

// JsonPath navigates nested maps to extract a string value.
func JsonPath(data map[string]interface{}, keys ...string) string {
	current := data
	for i, key := range keys {
		if i == len(keys)-1 {
			if v, ok := current[key].(string); ok {
				return v
			}
			return ""
		}
		if next, ok := current[key].(map[string]interface{}); ok {
			current = next
		} else {
			return ""
		}
	}
	return ""
}

// JsonArray navigates nested maps to extract a slice.
func JsonArray(data map[string]interface{}, keys ...string) []interface{} {
	current := data
	for i, key := range keys {
		if i == len(keys)-1 {
			if v, ok := current[key].([]interface{}); ok {
				return v
			}
			return nil
		}
		if next, ok := current[key].(map[string]interface{}); ok {
			current = next
		} else {
			return nil
		}
	}
	return nil
}

// JsonMap navigates nested maps to extract a child map.
func JsonMap(data map[string]interface{}, keys ...string) map[string]interface{} {
	current := data
	for _, key := range keys {
		if next, ok := current[key].(map[string]interface{}); ok {
			current = next
		} else {
			return nil
		}
	}
	return current
}

// StringOrEmpty safely extracts a string from a map.
func StringOrEmpty(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

