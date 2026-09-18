package tests

import (
	"testing"

	"govard/internal/proxy"
)

func TestEnsureRabbitMQServerConfigAddsListenPort(t *testing.T) {
	config := map[string]interface{}{}

	changed := proxy.EnsureRabbitMQServerConfigForTest(config)
	if !changed {
		t.Fatalf("Expected ensureRabbitMQServerConfig to report changes")
	}

	srvRabbitMQ := extractServer(t, config, "srv_rabbitmq")

	listen, ok := srvRabbitMQ["listen"].([]interface{})
	if !ok {
		t.Fatalf("Expected listen to be a slice")
	}
	if !proxy.StringSliceContainsForTest(listen, ":15672") {
		t.Fatalf("Expected srv_rabbitmq to include :15672")
	}

	autoHTTPS, ok := srvRabbitMQ["automatic_https"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected srv_rabbitmq to have automatic_https map, got %#v", srvRabbitMQ["automatic_https"])
	}
	if disable, ok := autoHTTPS["disable"].(bool); !ok || !disable {
		t.Fatalf("Expected srv_rabbitmq automatic_https.disable to be true, got %#v", autoHTTPS["disable"])
	}
}

func TestUpsertRabbitMQRouteIsIdempotentAndIsolated(t *testing.T) {
	config := map[string]interface{}{}

	if changed := proxy.UpsertRabbitMQRouteForTest(config, "demo.test", "demo-rabbitmq-1"); !changed {
		t.Fatal("expected first upsert to change config")
	}
	if changed := proxy.UpsertRabbitMQRouteForTest(config, "demo.test", "demo-rabbitmq-1"); changed {
		t.Fatal("expected second upsert with same target to be idempotent")
	}

	// A web route and a search route for the same domain must coexist with,
	// not replace, the rabbitmq route: three servers, three routes.
	if changed := proxy.UpsertDomainRouteForTest(config, "demo.test", "demo-web-1"); !changed {
		t.Fatal("expected web route upsert to change config")
	}
	if changed := proxy.UpsertSearchRouteForTest(config, "demo.test", "demo-elasticsearch-1"); !changed {
		t.Fatal("expected search route upsert to change config")
	}

	rabbitmqRoutes := extractRoutesForServer(t, config, "srv_rabbitmq")
	if len(rabbitmqRoutes) != 1 {
		t.Fatalf("expected exactly one rabbitmq route, got %d", len(rabbitmqRoutes))
	}
	webRoutes := extractRoutesForServer(t, config, "srv0")
	if len(webRoutes) != 1 {
		t.Fatalf("expected exactly one web route, got %d", len(webRoutes))
	}
	searchRoutes := extractRoutesForServer(t, config, "srv_search")
	if len(searchRoutes) != 1 {
		t.Fatalf("expected exactly one search route, got %d", len(searchRoutes))
	}

	if changed := proxy.RemoveRabbitMQRouteForTest(config, "demo.test"); !changed {
		t.Fatal("expected removal to change config")
	}
	if changed := proxy.RemoveRabbitMQRouteForTest(config, "demo.test"); changed {
		t.Fatal("expected second removal to be a no-op")
	}
	rabbitmqRoutes = extractRoutesForServer(t, config, "srv_rabbitmq")
	if len(rabbitmqRoutes) != 0 {
		t.Fatalf("expected zero rabbitmq routes after removal, got %d", len(rabbitmqRoutes))
	}
	// The neighbors must survive the removal.
	if webRoutes = extractRoutesForServer(t, config, "srv0"); len(webRoutes) != 1 {
		t.Fatalf("expected the web route to survive, got %d", len(webRoutes))
	}
	if searchRoutes = extractRoutesForServer(t, config, "srv_search"); len(searchRoutes) != 1 {
		t.Fatalf("expected the search route to survive, got %d", len(searchRoutes))
	}
}
