package monitoring

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/redhat-consulting-services/ocp-support-web/internal/k8s"
)

type Client struct {
	thanosURL string
	k8s       *k8s.Client
}

func NewClient(clusterDomain string, k8sClient *k8s.Client) *Client {
	return &Client{
		thanosURL: fmt.Sprintf("https://thanos-querier-openshift-monitoring.%s", clusterDomain),
		k8s:       k8sClient,
	}
}

func (c *Client) Query(query string) (json.RawMessage, error) {
	params := url.Values{"query": {query}}
	u := c.thanosURL + "/api/v1/query?" + params.Encode()
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.k8s.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.k8s.HTTPClient.Do(req)
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
