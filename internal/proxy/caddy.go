package proxy

import (
	"encoding/json"
	"fmt"
	"govard/internal/conventions"
	"os/exec"
	"reflect"
	"strings"
	"time"
)

const caddyExecRetryAttempts = 8
const caddyExecRetryDelay = 350 * time.Millisecond
const caddyAdminAPI = "http://localhost:2019"

var initCaddyCommandRunner = func(container string, initJSON string) error {
	_, err := runCaddyExec(container, "curl", "-s", "-X", "POST",
		fmt.Sprintf("%s/load", caddyAdminAPI),
		"-H", "Content-Type: application/json",
		"-d", initJSON)
	if err != nil {
		return fmt.Errorf("initialize caddy admin config: %w", err)
	}
	return nil
}

func runCaddyExec(container string, args ...string) ([]byte, error) {
	dockerArgs := []string{"exec", "-i", container}
	dockerArgs = append(dockerArgs, args...)

	var lastErr error
	var lastOutput string

	for attempt := 1; attempt <= caddyExecRetryAttempts; attempt++ {
		cmd := exec.Command("docker", dockerArgs...)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return output, nil
		}

		lastErr = err
		lastOutput = strings.TrimSpace(string(output))

		if !isTransientCaddyExecError(err, lastOutput) || attempt == caddyExecRetryAttempts {
			break
		}
		time.Sleep(caddyExecRetryDelay)
	}

	if lastOutput != "" {
		return nil, fmt.Errorf("docker exec failed: %w (%s)", lastErr, lastOutput)
	}
	return nil, fmt.Errorf("docker exec failed: %w", lastErr)
}

func isTransientCaddyExecError(err error, output string) bool {
	if err == nil {
		return false
	}
	combined := strings.ToLower(strings.TrimSpace(err.Error() + " " + output))
	if combined == "" {
		return false
	}

	transientMarkers := []string{
		"oci runtime exec failed",
		"is restarting",
		"is not running",
		"no such container",
		"cannot exec in a stopped state",
		"connection refused",
		"context deadline exceeded",
		"containerd task has not started",
	}
	for _, marker := range transientMarkers {
		if strings.Contains(combined, marker) {
			return true
		}
	}
	return false
}

func RegisterDomain(domain string, targetContainer string) error {
	return RegisterDomains([]string{domain}, targetContainer)
}

func RegisterDomains(domains []string, targetContainer string) error {
	proxyContainer := "govard-proxy-caddy"
	config, err := fetchCaddyConfig(proxyContainer)
	if err != nil || len(config) == 0 {
		if err := initCaddy(proxyContainer); err != nil {
			return err
		}
		config, err = fetchCaddyConfig(proxyContainer)
		if err != nil {
			return err
		}
	}

	changed := ensureTLSConfig(config)
	for _, domain := range domains {
		if strings.HasSuffix(domain, ".test") {
			policies, ok := config["apps"].(map[string]interface{})["tls"].(map[string]interface{})["automation"].(map[string]interface{})["policies"].([]interface{})
			if ok {
				newPolicies, policyChanged := ensurePolicySubject(policies, domain, changed)
				if policyChanged {
					config["apps"].(map[string]interface{})["tls"].(map[string]interface{})["automation"].(map[string]interface{})["policies"] = newPolicies
					changed = true
				}
			}
		}

		if upsertDomainRoute(config, domain, targetContainer) {
			changed = true
		}
	}

	if !changed {
		return nil
	}
	return loadCaddyConfig(proxyContainer, config)
}

func UnregisterDomain(domain string) error {
	proxyContainer := "govard-proxy-caddy"
	config, err := fetchCaddyConfig(proxyContainer)
	if err != nil {
		return nil
	}

	if !removeDomainRoute(config, domain) {
		return nil
	}
	return loadCaddyConfig(proxyContainer, config)
}

func RegisterSearchDomains(domains []string, targetContainer string) error {
	proxyContainer := "govard-proxy-caddy"
	config, err := fetchCaddyConfig(proxyContainer)
	if err != nil || len(config) == 0 {
		if err := initCaddy(proxyContainer); err != nil {
			return err
		}
		config, err = fetchCaddyConfig(proxyContainer)
		if err != nil {
			return err
		}
	}

	changed := ensureSearchServerConfig(config)
	for _, domain := range domains {
		if upsertSearchRoute(config, domain, targetContainer) {
			changed = true
		}
	}

	if !changed {
		return nil
	}
	return loadCaddyConfig(proxyContainer, config)
}

func UnregisterSearchDomain(domain string) error {
	proxyContainer := "govard-proxy-caddy"
	config, err := fetchCaddyConfig(proxyContainer)
	if err != nil {
		return nil
	}

	if !removeSearchRoute(config, domain) {
		return nil
	}
	return loadCaddyConfig(proxyContainer, config)
}

func EnsureTLS() error {
	proxyContainer := "govard-proxy-caddy"

	config, err := fetchCaddyConfig(proxyContainer)
	if err != nil || len(config) == 0 {
		return initCaddy(proxyContainer)
	}

	changed := ensureTLSConfig(config)
	if !changed {
		return nil
	}

	return loadCaddyConfig(proxyContainer, config)
}

func fetchCaddyConfig(container string) (map[string]interface{}, error) {
	output, err := runCaddyExec(container, "curl", "-s", fmt.Sprintf("%s/config/", caddyAdminAPI))
	if err != nil {
		return nil, fmt.Errorf("fetch caddy config: %w", err)
	}
	if len(output) == 0 {
		return map[string]interface{}{}, nil
	}

	var config map[string]interface{}
	if err := json.Unmarshal(output, &config); err != nil {
		return nil, err
	}
	if config == nil {
		config = map[string]interface{}{}
	}
	return config, nil
}

func loadCaddyConfig(container string, config map[string]interface{}) error {
	payload, err := json.Marshal(config)
	if err != nil {
		return err
	}

	if _, err := runCaddyExec(container, "curl", "-s", "-X", "POST",
		fmt.Sprintf("%s/load", caddyAdminAPI),
		"-H", "Content-Type: application/json",
		"-d", string(payload)); err != nil {
		return fmt.Errorf("caddy load failed: %w", err)
	}
	return nil
}

func upsertRoute(config map[string]interface{}, serverName string, domain string, targetContainer string, defaultPort int, idPrefix string) bool {
	changed := false
	apps := getOrCreateMap(config, "apps", &changed)
	http := getOrCreateMap(apps, "http", &changed)
	servers := getOrCreateMap(http, "servers", &changed)
	server := getOrCreateMap(servers, serverName, &changed)

	dial := targetContainer
	if !strings.Contains(dial, ":") {
		dial = fmt.Sprintf("%s:%d", dial, defaultPort)
	}

	routeID := routeIDForDomain(domain, idPrefix)
	desiredRoute := map[string]interface{}{
		"@id": routeID,
		"match": []interface{}{
			map[string]interface{}{
				"host": []interface{}{domain},
			},
		},
		"handle": []interface{}{
			map[string]interface{}{
				"handler": "reverse_proxy",
				"upstreams": []interface{}{
					map[string]interface{}{"dial": dial},
				},
			},
		},
		"terminal": true,
	}

	routes, _ := server["routes"].([]interface{})
	if routes == nil {
		routes = []interface{}{}
		changed = true
	}

	newRoutes := make([]interface{}, 0, len(routes))
	inserted := false
	for _, route := range routes {
		routeMap, ok := route.(map[string]interface{})
		if !ok {
			newRoutes = append(newRoutes, route)
			continue
		}
		if !routeMatchesDomain(routeMap, domain, routeID) {
			newRoutes = append(newRoutes, route)
			continue
		}

		if !inserted {
			if !reflect.DeepEqual(routeMap, desiredRoute) {
				changed = true
			}
			newRoutes = append(newRoutes, desiredRoute)
			inserted = true
		} else {
			changed = true
		}
	}

	if !inserted {
		newRoutes = append(newRoutes, desiredRoute)
		changed = true
	}

	server["routes"] = newRoutes
	servers[serverName] = server
	http["servers"] = servers
	apps["http"] = http
	config["apps"] = apps
	return changed
}

func upsertDomainRoute(config map[string]interface{}, domain string, targetContainer string) bool {
	return upsertRoute(config, "srv0", domain, targetContainer, 80, "govard_route_")
}

func upsertSearchRoute(config map[string]interface{}, domain string, targetContainer string) bool {
	return upsertRoute(config, "srv_search", domain, targetContainer, conventions.SearchPort, "govard_search_route_")
}

func removeRoute(config map[string]interface{}, serverName string, domain string, idPrefix string) bool {
	apps, ok := config["apps"].(map[string]interface{})
	if !ok {
		return false
	}
	http, ok := apps["http"].(map[string]interface{})
	if !ok {
		return false
	}
	servers, ok := http["servers"].(map[string]interface{})
	if !ok {
		return false
	}
	server, ok := servers[serverName].(map[string]interface{})
	if !ok {
		return false
	}

	routes, ok := server["routes"].([]interface{})
	if !ok {
		return false
	}

	routeID := routeIDForDomain(domain, idPrefix)
	filtered := make([]interface{}, 0, len(routes))
	changed := false
	for _, route := range routes {
		routeMap, ok := route.(map[string]interface{})
		if !ok {
			filtered = append(filtered, route)
			continue
		}
		if routeMatchesDomain(routeMap, domain, routeID) {
			changed = true
			continue
		}
		filtered = append(filtered, route)
	}
	if !changed {
		return false
	}

	server["routes"] = filtered
	servers[serverName] = server
	http["servers"] = servers
	apps["http"] = http
	config["apps"] = apps
	return true
}

func removeDomainRoute(config map[string]interface{}, domain string) bool {
	return removeRoute(config, "srv0", domain, "govard_route_")
}

func removeSearchRoute(config map[string]interface{}, domain string) bool {
	return removeRoute(config, "srv_search", domain, "govard_search_route_")
}

func routeIDForDomain(domain string, idPrefix string) string {
	safe := strings.NewReplacer(".", "_", "-", "_", ":", "_").Replace(domain)
	return idPrefix + safe
}

func routeMatchesDomain(route map[string]interface{}, domain string, routeID string) bool {
	if id, ok := route["@id"].(string); ok && id == routeID {
		return true
	}

	matches, ok := route["match"].([]interface{})
	if !ok {
		return false
	}
	for _, matchRaw := range matches {
		match, ok := matchRaw.(map[string]interface{})
		if !ok {
			continue
		}
		hosts, ok := match["host"].([]interface{})
		if !ok {
			continue
		}
		for _, hostRaw := range hosts {
			if host, ok := hostRaw.(string); ok && host == domain {
				return true
			}
		}
	}
	return false
}

func ensureTLSConfig(config map[string]interface{}) bool {
	changed := false

	apps := getOrCreateMap(config, "apps", &changed)
	http := getOrCreateMap(apps, "http", &changed)
	servers := getOrCreateMap(http, "servers", &changed)
	srv0 := getOrCreateMap(servers, "srv0", &changed)

	listenVal, ok := srv0["listen"]
	var listen []interface{}
	if ok {
		if l, ok := listenVal.([]interface{}); ok {
			for _, v := range l {
				if s, ok := v.(string); ok && s == ":443" {
					listen = append(listen, v)
				} else if ok {
					changed = true
				}
			}
		}
	}
	if len(listen) == 0 {
		listen = []interface{}{":443"}
		changed = true
	}
	srv0["listen"] = listen

	// Ensure srv_redirect for :80 and global redirect
	srvRedirect := getOrCreateMap(servers, "srv_redirect", &changed)
	srvRedirect["listen"] = []interface{}{":80"}
	redirectRoute := map[string]interface{}{
		"handle": []interface{}{
			map[string]interface{}{
				"handler": "static_response",
				"headers": map[string]interface{}{
					"Location": []interface{}{"https://{http.request.host}{http.request.uri}"},
				},
				"status_code": 308,
			},
		},
	}
	srvRedirect["routes"] = []interface{}{redirectRoute}

	routesVal, ok := srv0["routes"]
	if ok {
		if routes, ok := routesVal.([]interface{}); ok {
			filtered := make([]interface{}, 0, len(routes))
			for _, r := range routes {
				if isDefaultFileServerRoute(r) {
					changed = true
					continue
				}
				filtered = append(filtered, r)
			}
			srv0["routes"] = filtered
		}
	}

	tls := getOrCreateMap(apps, "tls", &changed)
	automation := getOrCreateMap(tls, "automation", &changed)

	policiesVal, ok := automation["policies"]
	var policies []interface{}
	if ok {
		if p, ok := policiesVal.([]interface{}); ok {
			policies = p
		}
	}

	policies, changed = ensurePolicySubject(policies, "*.test", changed)
	policies, changed = ensurePolicySubject(policies, "*.govard.test", changed)

	if policies == nil {
		policies = []interface{}{}
	}
	automation["policies"] = policies
	tls["automation"] = automation
	apps["tls"] = tls
	config["apps"] = apps

	return changed
}

func ensureSearchServerConfig(config map[string]interface{}) bool {
	changed := false

	apps := getOrCreateMap(config, "apps", &changed)
	http := getOrCreateMap(apps, "http", &changed)
	servers := getOrCreateMap(http, "servers", &changed)
	srvSearch := getOrCreateMap(servers, "srv_search", &changed)

	listenAddr := fmt.Sprintf(":%d", conventions.SearchPort)
	listenVal, ok := srvSearch["listen"]
	var listen []interface{}
	if ok {
		if l, ok := listenVal.([]interface{}); ok {
			for _, v := range l {
				if s, ok := v.(string); ok && s == listenAddr {
					listen = append(listen, v)
				} else if ok {
					changed = true
				}
			}
		}
	}
	if len(listen) == 0 {
		listen = []interface{}{listenAddr}
		changed = true
	}
	srvSearch["listen"] = listen

	// srv_search matches routes by Host header on project domains that also
	// qualify for the *.test automatic-TLS policy configured for srv0. Without
	// explicitly disabling automatic HTTPS here, Caddy would silently wrap this
	// plain HTTP listener in TLS, breaking host-side access to the search API.
	wantAutoHTTPS := map[string]interface{}{"disable": true}
	if autoHTTPS, ok := srvSearch["automatic_https"].(map[string]interface{}); !ok || !reflect.DeepEqual(autoHTTPS, wantAutoHTTPS) {
		srvSearch["automatic_https"] = wantAutoHTTPS
		changed = true
	}

	servers["srv_search"] = srvSearch
	http["servers"] = servers
	apps["http"] = http
	config["apps"] = apps

	return changed
}

func getOrCreateMap(parent map[string]interface{}, key string, changed *bool) map[string]interface{} {
	val, ok := parent[key]
	if ok {
		if m, ok := val.(map[string]interface{}); ok {
			return m
		}
	}
	m := map[string]interface{}{}
	parent[key] = m
	*changed = true
	return m
}

func stringSliceContains(values []interface{}, target string) bool {
	for _, v := range values {
		if s, ok := v.(string); ok && s == target {
			return true
		}
	}
	return false
}

func policyIncludesSubject(policies []interface{}, subject string) bool {
	for _, p := range policies {
		policy, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		subjects, ok := policy["subjects"].([]interface{})
		if !ok {
			continue
		}
		for _, s := range subjects {
			if str, ok := s.(string); ok && str == subject {
				return true
			}
		}
	}
	return false
}

func ensurePolicySubject(policies []interface{}, subject string, changed bool) ([]interface{}, bool) {
	if policyIncludesSubject(policies, subject) {
		return policies, changed
	}
	policy := map[string]interface{}{
		"subjects": []interface{}{subject},
		"issuers": []interface{}{
			map[string]interface{}{
				"module": "internal",
			},
		},
	}
	policies = append(policies, policy)
	return policies, true
}

// EnsureTLSConfigForTest exposes TLS config normalization for tests.
func EnsureTLSConfigForTest(config map[string]interface{}) bool {
	return ensureTLSConfig(config)
}

// PolicyIncludesSubjectForTest exposes policy lookup for tests.
func PolicyIncludesSubjectForTest(policies []interface{}, subject string) bool {
	return policyIncludesSubject(policies, subject)
}

// StringSliceContainsForTest exposes string slice lookup for tests.
func StringSliceContainsForTest(values []interface{}, target string) bool {
	return stringSliceContains(values, target)
}

// UpsertDomainRouteForTest exposes domain route upsert behavior for tests.
func UpsertDomainRouteForTest(config map[string]interface{}, domain string, targetContainer string) bool {
	return upsertDomainRoute(config, domain, targetContainer)
}

// RemoveDomainRouteForTest exposes domain route removal behavior for tests.
func RemoveDomainRouteForTest(config map[string]interface{}, domain string) bool {
	return removeDomainRoute(config, domain)
}

// EnsureSearchServerConfigForTest exposes search server config normalization for tests.
func EnsureSearchServerConfigForTest(config map[string]interface{}) bool {
	return ensureSearchServerConfig(config)
}

// UpsertSearchRouteForTest exposes search route upsert behavior for tests.
func UpsertSearchRouteForTest(config map[string]interface{}, domain string, targetContainer string) bool {
	return upsertSearchRoute(config, domain, targetContainer)
}

// RemoveSearchRouteForTest exposes search route removal behavior for tests.
func RemoveSearchRouteForTest(config map[string]interface{}, domain string) bool {
	return removeSearchRoute(config, domain)
}

func isDefaultFileServerRoute(route interface{}) bool {
	routeMap, ok := route.(map[string]interface{})
	if !ok {
		return false
	}
	if _, ok := routeMap["match"]; ok {
		return false
	}
	handleVal, ok := routeMap["handle"]
	if !ok {
		return false
	}
	handlers, ok := handleVal.([]interface{})
	if !ok || len(handlers) < 2 {
		return false
	}
	first, ok := handlers[0].(map[string]interface{})
	if !ok {
		return false
	}
	if first["handler"] != "vars" {
		return false
	}
	if root, ok := first["root"]; !ok || root != "/usr/share/caddy" {
		return false
	}
	second, ok := handlers[1].(map[string]interface{})
	if !ok {
		return false
	}
	return second["handler"] == "file_server"
}

func initCaddy(container string) error {
	// Wipe existing config and set basic structure to ensure srv0 exists without default routes
	initJSON := `{
		"apps": {
			"http": {
				"servers": {
					"srv0": {
						"listen": [":443"],
						"routes": []
					},
					"srv_redirect": {
						"listen": [":80"],
						"routes": [
							{
								"handle": [
									{
										"handler": "static_response",
										"headers": {
											"Location": ["https://{http.request.host}{http.request.uri}"]
										},
										"status_code": 308
									}
								]
							}
						]
					}
				}
			},
			"tls": {
				"automation": {
					"policies": [
						{
							"subjects": ["*.test"],
							"issuers": [
								{
									"module": "internal"
								}
							]
						},
						{
							"subjects": ["*.govard.test"],
							"issuers": [
								{
									"module": "internal"
								}
							]
						}
					]
				}
			}
		}
	}`
	return initCaddyCommandRunner(container, initJSON)
}

// IsDefaultFileServerRouteForTest exposes default route detection for tests in /tests.
func IsDefaultFileServerRouteForTest(route interface{}) bool {
	return isDefaultFileServerRoute(route)
}

// InitCaddyForTest exposes initCaddy for tests in /tests.
func InitCaddyForTest(container string) error {
	return initCaddy(container)
}

// SetInitCaddyCommandRunnerForTest overrides init caddy command execution for tests.
func SetInitCaddyCommandRunnerForTest(fn func(container string, initJSON string) error) func() {
	previous := initCaddyCommandRunner
	if fn != nil {
		initCaddyCommandRunner = fn
	}
	return func() {
		initCaddyCommandRunner = previous
	}
}
