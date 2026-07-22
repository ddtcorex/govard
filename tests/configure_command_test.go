package tests

import (
	"strings"
	"testing"

	"govard/internal/engine"
)

func TestConfigureAddsBaseUrl(t *testing.T) {
	config := engine.Config{Domain: "store.test"}
	cmds := engine.MagentoConfigCommandsForTest("proj", config)
	found := false
	for _, cmd := range cmds {
		for _, arg := range cmd.Args {
			if strings.Contains(arg, "--base-url=https://store.test/") {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected base url command")
	}
}

func TestConfigureDatabaseSetupDoesNotSetSearchFlags(t *testing.T) {
	config := engine.Config{
		FrameworkVersion: "2.4.8",
		Stack: engine.Stack{
			Services: engine.Services{
				Search: "opensearch",
			},
		},
	}

	cmds := engine.MagentoConfigCommandsForTest("proj", config)
	foundDBSetup := false
	for _, cmd := range cmds {
		if cmd.Desc != "Setting Database connection" {
			continue
		}
		foundDBSetup = true
		for _, arg := range cmd.Args {
			if strings.HasPrefix(arg, "--search-engine=") {
				t.Fatalf("did not expect --search-engine in setup:config:set args: %v", cmd.Args)
			}
			if strings.HasPrefix(arg, "--opensearch-") || strings.HasPrefix(arg, "--elasticsearch-") {
				t.Fatalf("did not expect search host flags in setup:config:set args: %v", cmd.Args)
			}
		}
	}

	if !foundDBSetup {
		t.Fatal("expected database setup command")
	}
}

func TestConfigureDatabaseSetupIncludesTablePrefix(t *testing.T) {
	config := engine.Config{
		TablePrefix: "demo_",
	}

	cmds := engine.MagentoConfigCommandsForTest("proj", config)
	for _, cmd := range cmds {
		if cmd.Desc != "Setting Database connection" {
			continue
		}
		joined := strings.Join(cmd.Args, " ")
		if !strings.Contains(joined, "--db-prefix=demo_") {
			t.Fatalf("expected setup:config:set args to contain table prefix, got: %v", cmd.Args)
		}
		return
	}

	t.Fatal("expected database setup command")
}

func TestConfigureSearchHostCommandsUseMagentoConfigSet(t *testing.T) {
	config := engine.Config{
		FrameworkVersion: "2.4.8",
		Stack: engine.Stack{
			Services: engine.Services{
				Search: "opensearch",
			},
		},
	}

	cmds := engine.MagentoConfigCommandsForTest("proj", config)
	joined := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		joined = append(joined, strings.Join(cmd.Args, " "))
	}
	all := strings.Join(joined, "\n")

	if !strings.Contains(all, "catalog/search/opensearch_server_hostname") {
		t.Fatalf("expected opensearch hostname config:set command, got:\n%s", all)
	}
	if !strings.Contains(all, "catalog/search/opensearch_server_port") {
		t.Fatalf("expected opensearch port config:set command, got:\n%s", all)
	}
}

func TestConfigureEnablesWebServerRewrites(t *testing.T) {
	config := engine.Config{
		Domain: "store.test",
	}

	cmds := engine.MagentoConfigCommandsForTest("proj", config)
	joined := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		joined = append(joined, strings.Join(cmd.Args, " "))
	}
	all := strings.Join(joined, "\n")

	if !strings.Contains(all, "web/seo/use_rewrites") {
		t.Fatalf("expected web server rewrites to be enabled, got:\n%s", all)
	}
	if !strings.Contains(all, "web/seo/use_rewrites 1") {
		t.Fatalf("expected web/seo/use_rewrites to be set to 1, got:\n%s", all)
	}
}

func TestConfigureRabbitMQAmqpCommands(t *testing.T) {
	config := engine.Config{
		Stack: engine.Stack{
			Services: engine.Services{
				Queue: "rabbitmq",
			},
		},
	}

	cmds := engine.MagentoConfigCommandsForTest("proj", config)
	joined := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		joined = append(joined, strings.Join(cmd.Args, " "))
	}
	all := strings.Join(joined, "\n")

	if !strings.Contains(all, "--amqp-host=rabbitmq") {
		t.Fatalf("expected --amqp-host=rabbitmq config:set command, got:\n%s", all)
	}
	if !strings.Contains(all, "--amqp-port=5672") {
		t.Fatalf("expected --amqp-port=5672 config:set command, got:\n%s", all)
	}
}

func TestConfigureWithoutRabbitMQSkipsAmqpCommands(t *testing.T) {
	config := engine.Config{
		Stack: engine.Stack{
			Services: engine.Services{
				Queue: "none",
			},
		},
	}

	cmds := engine.MagentoConfigCommandsForTest("proj", config)
	for _, cmd := range cmds {
		for _, arg := range cmd.Args {
			if strings.HasPrefix(arg, "--amqp-") {
				t.Fatalf("did not expect --amqp- flags when queue is disabled, got: %v", cmd.Args)
			}
		}
	}
}

func TestConfigureMageOSDatabaseSetupUsesMageOSCredentials(t *testing.T) {
	config := engine.Config{Framework: "mageos"}

	cmds := engine.MagentoConfigCommandsForTest("proj", config)
	for _, cmd := range cmds {
		if cmd.Desc != "Setting Database connection" {
			continue
		}
		joined := strings.Join(cmd.Args, " ")
		if !strings.Contains(joined, "--db-name=mageos") || !strings.Contains(joined, "--db-user=mageos") || !strings.Contains(joined, "--db-password=mageos") {
			t.Fatalf("expected mageos db credentials in setup:config:set, got: %s", joined)
		}
		return
	}
	t.Fatal("expected a Setting Database connection command")
}

func TestConfigureMagento2DatabaseSetupStillUsesMagentoCredentials(t *testing.T) {
	config := engine.Config{Framework: "magento2"}

	cmds := engine.MagentoConfigCommandsForTest("proj", config)
	for _, cmd := range cmds {
		if cmd.Desc != "Setting Database connection" {
			continue
		}
		joined := strings.Join(cmd.Args, " ")
		if !strings.Contains(joined, "--db-name=magento") || !strings.Contains(joined, "--db-user=magento") || !strings.Contains(joined, "--db-password=magento") {
			t.Fatalf("expected magento db credentials in setup:config:set, got: %s", joined)
		}
		return
	}
	t.Fatal("expected a Setting Database connection command")
}
