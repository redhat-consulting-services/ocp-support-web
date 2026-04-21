package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/redhat-consulting-services/ocp-support-web/internal/k8s"
	"go.yaml.in/yaml/v2"
)

type apiResourceGroup struct {
	Name      string        `json:"name"`
	Version   string        `json:"version"`
	Resources []apiResource `json:"resources"`
}

type apiResource struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Namespaced bool   `json:"namespaced"`
}

var (
	apiResourcesCacheMu   sync.Mutex
	apiResourcesCache     []apiResourceGroup
	apiResourcesCacheTime time.Time

	validAPIGroup     = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*[a-z0-9]$`)
	validAPIVersion   = regexp.MustCompile(`^v[0-9]+(?:(?:alpha|beta)[0-9]+)?$`)
	validResourceType = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	validObjectName   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]*$`)
)

func (h *Handler) handleResourcesPage(w http.ResponseWriter, r *http.Request) {
	if err := h.tmpl.ExecuteTemplate(w, "resources.html", h.getPageVars(r)); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal Server Error", 500)
	}
}

func (h *Handler) handleAPIResources(w http.ResponseWriter, r *http.Request) {
	groups, err := h.discoverAPIResources()
	if err != nil {
		log.Printf("API resources discovery error: %v", err)
		jsonError(w, "failed to discover API resources", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(groups)
}

func (h *Handler) discoverAPIResources() ([]apiResourceGroup, error) {
	apiResourcesCacheMu.Lock()
	if time.Since(apiResourcesCacheTime) < 5*time.Minute && apiResourcesCache != nil {
		cached := apiResourcesCache
		apiResourcesCacheMu.Unlock()
		return cached, nil
	}
	apiResourcesCacheMu.Unlock()

	var groups []apiResourceGroup

	if coreData, err := h.k8s.Get("/api/v1"); err == nil {
		cg := apiResourceGroup{Name: "", Version: "v1"}
		for _, r := range k8s.JsonArray(coreData, "resources") {
			res, ok := r.(map[string]interface{})
			if !ok {
				continue
			}
			name := k8s.StringOrEmpty(res, "name")
			if strings.Contains(name, "/") {
				continue
			}
			namespaced, _ := res["namespaced"].(bool)
			cg.Resources = append(cg.Resources, apiResource{
				Name:       name,
				Kind:       k8s.StringOrEmpty(res, "kind"),
				Namespaced: namespaced,
			})
		}
		sort.Slice(cg.Resources, func(i, j int) bool {
			return cg.Resources[i].Name < cg.Resources[j].Name
		})
		groups = append(groups, cg)
	}

	if apisData, err := h.k8s.Get("/apis"); err == nil {
		for _, g := range k8s.JsonArray(apisData, "groups") {
			grp, ok := g.(map[string]interface{})
			if !ok {
				continue
			}
			groupName := k8s.StringOrEmpty(grp, "name")
			prefVersion := k8s.JsonPath(grp, "preferredVersion", "version")
			if prefVersion == "" {
				continue
			}

			resData, err := h.k8s.Get("/apis/" + groupName + "/" + prefVersion)
			if err != nil {
				continue
			}

			rg := apiResourceGroup{Name: groupName, Version: prefVersion}
			for _, r := range k8s.JsonArray(resData, "resources") {
				res, ok := r.(map[string]interface{})
				if !ok {
					continue
				}
				name := k8s.StringOrEmpty(res, "name")
				if strings.Contains(name, "/") {
					continue
				}
				namespaced, _ := res["namespaced"].(bool)
				rg.Resources = append(rg.Resources, apiResource{
					Name:       name,
					Kind:       k8s.StringOrEmpty(res, "kind"),
					Namespaced: namespaced,
				})
			}
			sort.Slice(rg.Resources, func(i, j int) bool {
				return rg.Resources[i].Name < rg.Resources[j].Name
			})
			if len(rg.Resources) > 0 {
				groups = append(groups, rg)
			}
		}
	}

	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Name == "" {
			return true
		}
		if groups[j].Name == "" {
			return false
		}
		return groups[i].Name < groups[j].Name
	})

	apiResourcesCacheMu.Lock()
	apiResourcesCache = groups
	apiResourcesCacheTime = time.Now()
	apiResourcesCacheMu.Unlock()

	return groups, nil
}

type nsResourceItem struct {
	Name    string `json:"name"`
	Created string `json:"created"`
}

type nsResourceResult struct {
	Group    string           `json:"group"`
	Version  string           `json:"version"`
	Resource string           `json:"resource"`
	Kind     string           `json:"kind"`
	Items    []nsResourceItem `json:"items"`
}

func (h *Handler) handleNamespaceResources(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("ns")
	if !validNamespace.MatchString(ns) {
		jsonError(w, "invalid namespace", 400)
		return
	}

	groups, err := h.discoverAPIResources()
	if err != nil {
		jsonError(w, "failed to discover API resources", 500)
		return
	}

	type resType struct {
		group, version, resource, kind string
	}
	var types []resType
	for _, g := range groups {
		if g.Name == "packages.operators.coreos.com" {
			continue
		}
		for _, res := range g.Resources {
			if res.Namespaced {
				types = append(types, resType{g.Name, g.Version, res.Name, res.Kind})
			}
		}
	}

	results := make([]nsResourceResult, len(types))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 20)

	for i, t := range types {
		wg.Add(1)
		go func(idx int, rt resType) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var path string
			if rt.group == "" {
				path = "/api/" + rt.version + "/namespaces/" + ns + "/" + rt.resource + "?limit=100"
			} else {
				path = "/apis/" + rt.group + "/" + rt.version + "/namespaces/" + ns + "/" + rt.resource + "?limit=100"
			}

			data, err := h.k8s.Get(path)
			if err != nil {
				return
			}

			var items []nsResourceItem
			for _, item := range k8s.JsonArray(data, "items") {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				items = append(items, nsResourceItem{
					Name:    k8s.JsonPath(m, "metadata", "name"),
					Created: k8s.JsonPath(m, "metadata", "creationTimestamp"),
				})
			}

			if len(items) > 0 {
				results[idx] = nsResourceResult{
					Group:    rt.group,
					Version:  rt.version,
					Resource: rt.resource,
					Kind:     rt.kind,
					Items:    items,
				}
			}
		}(i, t)
	}

	wg.Wait()

	var filtered []nsResourceResult
	for _, res := range results {
		if len(res.Items) > 0 {
			filtered = append(filtered, res)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		gi, gj := filtered[i].Group, filtered[j].Group
		if gi != gj {
			if gi == "" {
				return true
			}
			if gj == "" {
				return false
			}
			return gi < gj
		}
		return filtered[i].Resource < filtered[j].Resource
	})

	if filtered == nil {
		filtered = []nsResourceResult{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(filtered)
}

func (h *Handler) handleListNamespacedResources(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("ns")
	group := r.URL.Query().Get("group")
	version := r.URL.Query().Get("version")
	resource := r.URL.Query().Get("resource")

	if !validNamespace.MatchString(ns) {
		jsonError(w, "invalid namespace", 400)
		return
	}
	if group != "" && !validAPIGroup.MatchString(group) {
		jsonError(w, "invalid group", 400)
		return
	}
	if !validAPIVersion.MatchString(version) {
		jsonError(w, "invalid version", 400)
		return
	}
	if !validResourceType.MatchString(resource) {
		jsonError(w, "invalid resource", 400)
		return
	}

	var path string
	if group == "" {
		path = "/api/" + version + "/namespaces/" + ns + "/" + resource
	} else {
		path = "/apis/" + group + "/" + version + "/namespaces/" + ns + "/" + resource
	}

	data, err := h.k8s.Get(path)
	if err != nil {
		if k8s.IsNotFound(err) || k8s.IsForbidden(err) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]interface{}{})
			return
		}
		log.Printf("list resources error (%s): %v", path, err)
		jsonError(w, "failed to list resources", 500)
		return
	}

	type item struct {
		Name    string `json:"name"`
		Created string `json:"created"`
	}
	var items []item
	for _, i := range k8s.JsonArray(data, "items") {
		m, ok := i.(map[string]interface{})
		if !ok {
			continue
		}
		items = append(items, item{
			Name:    k8s.JsonPath(m, "metadata", "name"),
			Created: k8s.JsonPath(m, "metadata", "creationTimestamp"),
		})
	}
	if items == nil {
		items = []item{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func (h *Handler) handleGetResource(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("ns")
	group := r.URL.Query().Get("group")
	version := r.URL.Query().Get("version")
	resource := r.URL.Query().Get("resource")
	name := r.URL.Query().Get("name")

	if !validNamespace.MatchString(ns) {
		jsonError(w, "invalid namespace", 400)
		return
	}
	if group != "" && !validAPIGroup.MatchString(group) {
		jsonError(w, "invalid group", 400)
		return
	}
	if !validAPIVersion.MatchString(version) {
		jsonError(w, "invalid version", 400)
		return
	}
	if !validResourceType.MatchString(resource) {
		jsonError(w, "invalid resource", 400)
		return
	}
	if !validObjectName.MatchString(name) {
		jsonError(w, "invalid name", 400)
		return
	}

	var path string
	if group == "" {
		path = "/api/" + version + "/namespaces/" + ns + "/" + resource + "/" + name
	} else {
		path = "/apis/" + group + "/" + version + "/namespaces/" + ns + "/" + resource + "/" + name
	}

	data, err := h.k8s.Get(path)
	if err != nil {
		if k8s.IsNotFound(err) {
			jsonError(w, "not found", 404)
			return
		}
		log.Printf("get resource error (%s): %v", path, err)
		jsonError(w, "failed to get resource", 500)
		return
	}

	if metadata, ok := data["metadata"].(map[string]interface{}); ok {
		delete(metadata, "managedFields")
	}

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		jsonError(w, "failed to marshal YAML", 500)
		return
	}

	w.Header().Set("Content-Type", "text/yaml")
	w.Write(yamlBytes)
}
