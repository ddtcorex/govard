package tests

import (
	"reflect"
	"testing"

	"govard/internal/proxy"
)

func TestUpsertFrontendRegistrationAddsModeSpecificRoutesBeforeApplicationRoute(t *testing.T) {
	tests := []struct {
		name              string
		registration      proxy.FrontendRegistration
		wantEndpointPath  string
		wantEndpointDial  string
		wantInjectionDial string
	}{
		{
			name: "Hyva BrowserSync and HTML injection proxy",
			registration: proxy.FrontendRegistration{
				ProjectName: "synthetic-store",
				Domains:     []string{"synthetic-store.test"},
				Endpoint: proxy.FrontendEndpoint{
					Path:   "/browser-sync/*",
					Target: "synthetic-store-sync-1:3000",
				},
				HTMLInjectionTarget: "synthetic-store-inject-1:3000",
			},
			wantEndpointPath:  "/browser-sync/*",
			wantEndpointDial:  "synthetic-store-sync-1:3000",
			wantInjectionDial: "synthetic-store-inject-1:3000",
		},
		{
			name: "Luma LiveReload and HTML injection proxy",
			registration: proxy.FrontendRegistration{
				ProjectName: "synthetic-store",
				Domains:     []string{"synthetic-store.test"},
				Endpoint: proxy.FrontendEndpoint{
					Path:        "/livereload/*",
					Target:      "synthetic-store-sync-1:35729",
					StripPrefix: "/livereload",
				},
				HTMLInjectionTarget: "synthetic-store-inject-1:3000",
			},
			wantEndpointPath:  "/livereload/*",
			wantEndpointDial:  "synthetic-store-sync-1:35729",
			wantInjectionDial: "synthetic-store-inject-1:3000",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := map[string]interface{}{}
			_ = proxy.UpsertDomainRouteForTest(config, "synthetic-store.test", "synthetic-store-web-1")

			if changed := proxy.UpsertFrontendRegistrationForTest(config, test.registration); !changed {
				t.Fatal("expected frontend registration to change config")
			}

			routes := extractRoutes(t, config)
			wantCount := 2
			if test.wantInjectionDial != "" {
				wantCount = 3
			}
			if len(routes) != wantCount {
				t.Fatalf("route count = %d, want %d: %#v", len(routes), wantCount, routes)
			}
			assertFrontendRoute(t, routes[0], test.wantEndpointPath, test.wantEndpointDial)
			if test.wantInjectionDial != "" {
				assertFrontendRoute(t, routes[1], "", test.wantInjectionDial)
			}
			applicationRoute := routes[len(routes)-1].(map[string]interface{})
			if got := applicationRoute["@id"]; got != "govard_route_synthetic_store_test" {
				t.Fatalf("application fallback route id = %#v", got)
			}
		})
	}
}

func TestRemoveFrontendRegistrationRemovesRouteAndInjectionWithoutApplicationRoute(t *testing.T) {
	config := map[string]interface{}{}
	_ = proxy.UpsertDomainRouteForTest(config, "synthetic-store.test", "synthetic-store-web-1")
	registration := proxy.FrontendRegistration{
		ProjectName: "synthetic-store",
		Domains:     []string{"synthetic-store.test"},
		Endpoint: proxy.FrontendEndpoint{
			Path:        "/livereload/*",
			Target:      "synthetic-store-sync-1:35729",
			StripPrefix: "/livereload",
		},
		HTMLInjectionTarget: "synthetic-store-inject-1:3000",
	}
	_ = proxy.UpsertFrontendRegistrationForTest(config, registration)

	if changed := proxy.RemoveFrontendRegistrationForTest(config, "synthetic-store"); !changed {
		t.Fatal("expected frontend registration removal to change config")
	}
	routes := extractRoutes(t, config)
	if len(routes) != 1 {
		t.Fatalf("routes after removal = %#v", routes)
	}
	if got := routes[0].(map[string]interface{})["@id"]; got != "govard_route_synthetic_store_test" {
		t.Fatalf("remaining route id = %#v", got)
	}
	if changed := proxy.RemoveFrontendRegistrationForTest(config, "synthetic-store"); changed {
		t.Fatal("second frontend registration removal must be idempotent")
	}
}

func TestRemoveFrontendRegistrationDoesNotRemoveProjectWithSharedNamePrefix(t *testing.T) {
	config := map[string]interface{}{}
	for _, projectName := range []string{"synthetic", "synthetic_extra"} {
		_ = proxy.UpsertFrontendRegistrationForTest(config, proxy.FrontendRegistration{
			ProjectName: projectName,
			Domains:     []string{projectName + ".test"},
			Endpoint: proxy.FrontendEndpoint{
				Path:   "/browser-sync/*",
				Target: projectName + "-sync-1:3000",
			},
		})
	}

	if changed := proxy.RemoveFrontendRegistrationForTest(config, "synthetic"); !changed {
		t.Fatal("expected first project registration removal")
	}
	routes := extractRoutes(t, config)
	if len(routes) != 1 {
		t.Fatalf("routes after project-scoped removal = %#v", routes)
	}
	match := routes[0].(map[string]interface{})["match"].([]interface{})[0].(map[string]interface{})
	if got := match["host"]; !reflect.DeepEqual(got, []interface{}{"synthetic_extra.test"}) {
		t.Fatalf("remaining route host = %#v", got)
	}
}

func assertFrontendRoute(t *testing.T, raw interface{}, wantPath, wantDial string) {
	t.Helper()
	route, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("route = %#v", raw)
	}
	matches := route["match"].([]interface{})
	match := matches[0].(map[string]interface{})
	if wantPath == "" {
		if _, exists := match["path"]; exists {
			t.Fatalf("HTML injection route unexpectedly has path matcher: %#v", match)
		}
	} else if got := match["path"]; !reflect.DeepEqual(got, []interface{}{wantPath}) {
		t.Fatalf("path matcher = %#v, want %q", got, wantPath)
	}
	handlers := route["handle"].([]interface{})
	handlerIndex := 0
	if wantPath == "/livereload/*" {
		rewrite := handlers[0].(map[string]interface{})
		if got := rewrite["handler"]; got != "rewrite" {
			t.Fatalf("LiveReload first handler = %#v, want rewrite", rewrite)
		}
		if got := rewrite["strip_path_prefix"]; got != "/livereload" {
			t.Fatalf("LiveReload strip prefix = %#v", got)
		}
		handlerIndex = 1
	}
	handler := handlers[handlerIndex].(map[string]interface{})
	upstreams := handler["upstreams"].([]interface{})
	if got := upstreams[0].(map[string]interface{})["dial"]; got != wantDial {
		t.Fatalf("route dial = %#v, want %q", got, wantDial)
	}
}

func TestUpsertDomainRouteIsIdempotent(t *testing.T) {
	config := map[string]interface{}{}

	if changed := proxy.UpsertDomainRouteForTest(config, "demo.test", "demo-web-1"); !changed {
		t.Fatal("expected first upsert to change config")
	}

	if changed := proxy.UpsertDomainRouteForTest(config, "demo.test", "demo-web-1"); changed {
		t.Fatal("expected second upsert with same target to be idempotent")
	}

	if changed := proxy.UpsertDomainRouteForTest(config, "demo.test", "demo-varnish-1"); !changed {
		t.Fatal("expected upsert with different target to change config")
	}

	routes := extractRoutes(t, config)
	if len(routes) != 1 {
		t.Fatalf("expected exactly one route for domain, got %d", len(routes))
	}
}

func TestRemoveDomainRoute(t *testing.T) {
	config := map[string]interface{}{}
	_ = proxy.UpsertDomainRouteForTest(config, "demo.test", "demo-web-1")

	if changed := proxy.RemoveDomainRouteForTest(config, "demo.test"); !changed {
		t.Fatal("expected remove to change config")
	}

	if changed := proxy.RemoveDomainRouteForTest(config, "demo.test"); changed {
		t.Fatal("expected second remove to be a no-op")
	}
}

func extractRoutes(t *testing.T, config map[string]interface{}) []interface{} {
	t.Helper()
	apps, ok := config["apps"].(map[string]interface{})
	if !ok {
		t.Fatal("missing apps map")
	}
	http, ok := apps["http"].(map[string]interface{})
	if !ok {
		t.Fatal("missing http map")
	}
	servers, ok := http["servers"].(map[string]interface{})
	if !ok {
		t.Fatal("missing servers map")
	}
	srv0, ok := servers["srv0"].(map[string]interface{})
	if !ok {
		t.Fatal("missing srv0 map")
	}
	routes, ok := srv0["routes"].([]interface{})
	if !ok {
		t.Fatal("missing routes slice")
	}
	return routes
}
