package monitoring

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	thanosURL  string
	token      string
	httpClient *http.Client
}

func NewClient(clusterDomain, token string, insecureSkipTLS bool) *Client {
	tlsConfig := &tls.Config{}
	if insecureSkipTLS {
		tlsConfig.InsecureSkipVerify = true
	}

	return &Client{
		thanosURL: fmt.Sprintf("https://thanos-querier-openshift-monitoring.%s", clusterDomain),
		token:     token,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{TLSClientConfig: tlsConfig},
		},
	}
}

func (c *Client) Query(query string) (json.RawMessage, error) {
	params := url.Values{"query": {query}}
	u := c.thanosURL + "/api/v1/query?" + params.Encode()
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("API error %d: %s", resp.StatusCode, string(body))
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return nil, fmt.Errorf("authentication error: token is invalid or lacks permissions (HTTP %d)", resp.StatusCode)
		}
		return nil, fmt.Errorf("API error (HTTP %d)", resp.StatusCode)
	}

	var promResp struct {
		Status string          `json:"status"`
		Data   json.RawMessage `json:"data"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(body, &promResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if promResp.Status != "success" {
		return nil, fmt.Errorf("query failed: %s", promResp.Error)
	}
	return promResp.Data, nil
}
