package auth

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type UserInfo struct {
	Name   string
	Groups []string
}

type Middleware struct {
	apiURL        string
	httpClient    *http.Client
	allowedGroups map[string]bool
	cache         sync.Map // token → *cacheEntry
}

type cacheEntry struct {
	user    *UserInfo
	allowed bool
	expires time.Time
}

const cacheTTL = 60 * time.Second

func NewMiddleware(apiURL string, insecureSkipTLS bool, allowedGroups []string) *Middleware {
	groups := make(map[string]bool, len(allowedGroups))
	for _, g := range allowedGroups {
		groups[g] = true
	}
	return &Middleware{
		apiURL: apiURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipTLS},
			},
		},
		allowedGroups: groups,
	}
}

func (m *Middleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if token == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		user, allowed, err := m.resolveAndAuthorize(token)
		if err != nil {
			log.Printf("auth: failed to authenticate: %v", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if !allowed {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		r.Header.Set("X-Forwarded-User", user.Name)
		next.ServeHTTP(w, r)
	})
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

func (m *Middleware) resolveAndAuthorize(token string) (*UserInfo, bool, error) {
	if entry, ok := m.cache.Load(token); ok {
		ce := entry.(*cacheEntry)
		if time.Now().Before(ce.expires) {
			return ce.user, ce.allowed, nil
		}
		m.cache.Delete(token)
	}

	user, err := m.fetchUser(token)
	if err != nil {
		return nil, false, err
	}

	var allowed bool
	if len(m.allowedGroups) > 0 {
		allowed = m.isAllowed(user)
	} else {
		allowed, err = m.checkAccess(token)
		if err != nil {
			return nil, false, fmt.Errorf("access review failed: %w", err)
		}
	}

	m.cache.Store(token, &cacheEntry{
		user:    user,
		allowed: allowed,
		expires: time.Now().Add(cacheTTL),
	})
	return user, allowed, nil
}

func (m *Middleware) fetchUser(token string) (*UserInfo, error) {
	req, err := http.NewRequest("GET", m.apiURL+"/apis/user.openshift.io/v1/users/~", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("user API returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Groups []string `json:"groups"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}

	return &UserInfo{
		Name:   result.Metadata.Name,
		Groups: result.Groups,
	}, nil
}

// checkAccess performs a SelfSubjectAccessReview using the user's token to verify
// they have RBAC access to OCPSupportWeb resources.
func (m *Middleware) checkAccess(token string) (bool, error) {
	sar := map[string]interface{}{
		"apiVersion": "authorization.k8s.io/v1",
		"kind":       "SelfSubjectAccessReview",
		"spec": map[string]interface{}{
			"resourceAttributes": map[string]interface{}{
				"group":    "support.openshift.io",
				"resource": "ocpsupportwebs",
				"verb":     "get",
			},
		},
	}

	body, err := json.Marshal(sar)
	if err != nil {
		return false, err
	}

	req, err := http.NewRequest("POST", m.apiURL+"/apis/authorization.k8s.io/v1/selfsubjectaccessreviews", bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return false, fmt.Errorf("SAR returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Status struct {
			Allowed bool `json:"allowed"`
		} `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("failed to decode SAR response: %w", err)
	}

	return result.Status.Allowed, nil
}

func (m *Middleware) isAllowed(user *UserInfo) bool {
	for _, g := range user.Groups {
		if m.allowedGroups[g] {
			return true
		}
	}
	return false
}
