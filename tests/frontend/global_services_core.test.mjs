import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import {
  buildRoutingWarningMessage,
  formatBulkGlobalActionErrorForTest,
  hasRoutingImpact,
  hasRoutingWarningInSnapshot,
  isServiceActive,
  normalizeGlobalServicesSnapshot,
  summarizeActionMessage,
} from "../../desktop/frontend/modules/global-services.js";

const readIsland = (name) =>
  readFile(
    new URL(`../../desktop/frontend/islands/${name}`, import.meta.url),
    "utf8",
  );

const readEntryPoint = (name) =>
  readFile(new URL(`../../desktop/frontend/${name}`, import.meta.url), "utf8");

test("normalizeGlobalServicesSnapshot keeps DNSMasq directly below Caddy", () => {
  const snapshot = normalizeGlobalServicesSnapshot({
    Services: [
      { ID: "mail", Name: "Mailpit" },
      { ID: "dnsmasq", Name: "DNSMasq" },
      { ID: "portainer", Name: "Portainer" },
      { ID: "caddy", Name: "Caddy Proxy" },
      { ID: "pma", Name: "PHPMyAdmin" },
    ],
  });

  const ids = snapshot.services.map((item) => item.id);
  const caddyIndex = ids.indexOf("caddy");
  const dnsmasqIndex = ids.indexOf("dnsmasq");
  assert.equal(caddyIndex >= 0, true);
  assert.equal(dnsmasqIndex, caddyIndex + 1);
});

// The warning used to be asserted through renderGlobalServices' innerHTML; the
// island renders it now, so the gate is asserted on the helper that decides and
// the copy on the island that prints it. The distinction matters: with the
// warning copy in the island there is nothing left to test in the module, and
// with the gate in the module there is nothing to infer from the markup.
test("hasRoutingImpact flags a stopped routing service and nothing else", () => {
  assert.equal(
    hasRoutingImpact({ id: "caddy", status: "stopped", state: "stopped", running: false }),
    true,
  );
  assert.equal(
    hasRoutingImpact({ id: "dnsmasq", status: "exited", state: "exited", running: false }),
    true,
  );
  assert.equal(
    hasRoutingImpact({ id: "mail", status: "stopped", state: "stopped", running: false }),
    false,
    "a non-routing service never raises the routing warning",
  );
  assert.equal(
    hasRoutingImpact({ id: "caddy", status: "running", state: "running", running: true }),
    false,
  );
});

test("hasRoutingWarningInSnapshot also catches a port-conflict warning", () => {
  const running = [
    { id: "caddy", status: "running", state: "running", running: true },
    { id: "dnsmasq", status: "running", state: "running", running: true },
  ];
  assert.equal(hasRoutingWarningInSnapshot({ services: running, warnings: [] }), false);
  assert.equal(
    hasRoutingWarningInSnapshot({
      services: running,
      warnings: ["Port conflict 80/tcp: docker container warden-nginx-1"],
    }),
    true,
  );
});

test("the service list island renders the routing warning the module decides", async () => {
  const island = await readIsland("GlobalServicesList.tsx");
  assert.equal(
    island.includes("hasRoutingImpact"),
    true,
    "the warning must be gated by the module's helper",
  );
  assert.equal(island.includes("Routing warning: "), true, "missing the warning copy");
  assert.equal(
    island.includes("is stopped. Proxy/domain routing"),
    true,
    "missing the second half of the warning copy",
  );
  assert.equal(island.includes("may fail."), true);
});

test("isServiceActive reads either the status or the running flag", () => {
  assert.equal(isServiceActive({ status: "running" }), true);
  assert.equal(isServiceActive({ status: "exited", running: true }), true);
  assert.equal(isServiceActive({ status: "exited", running: false }), false);
});

test("buildRoutingWarningMessage includes detected port conflict list without hardcoded stacks", () => {
  const message = buildRoutingWarningMessage(
    [
      { id: "caddy", status: "created", state: "created", running: false },
      { id: "dnsmasq", status: "created", state: "created", running: false },
    ],
    [
      "Port conflict 80/tcp: docker container warden-nginx-1 (project: warden)",
      "Port conflict 53/udp: host process dnsmasq (pid: 845)",
    ],
  );

  assert.equal(message.includes("Warden"), false);
  assert.equal(message.split("\n").length, 3);
  assert.equal(message.includes("Missing bindings:"), true);
  assert.equal(message.includes("Occupied by:"), true);
  assert.equal(message.includes("warden-nginx-1 (80/tcp)"), true);
  assert.equal(message.includes("dnsmasq (53/udp)"), true);
  assert.equal(
    message.includes("Resolve conflicts, then click Restart All or Start All."),
    true,
  );
});

test("buildRoutingWarningMessage warns when services look running but bindings are degraded", () => {
  const message = buildRoutingWarningMessage(
    [
      { id: "caddy", status: "running", state: "running", running: true },
      { id: "dnsmasq", status: "running", state: "running", running: true },
    ],
    [
      "Port conflict 80/tcp: Caddy Proxy is running but govard-proxy-caddy is not published on host",
    ],
  );

  assert.equal(
    message.includes(
      "Routing services are running but port bindings are degraded.",
    ),
    true,
  );
  assert.equal(message.split("\n").length, 3);
  assert.equal(message.includes("Missing bindings: Caddy Proxy (80/tcp)."), true);
  assert.equal(
    message.includes("is running but govard-proxy-caddy is not published on host"),
    false,
  );
});

test("summarizeActionMessage keeps only the first line from multiline command output", () => {
  const message = summarizeActionMessage(
    "Global services restarted.\nContainer govard-proxy-caddy Started\nContainer govard-proxy-pma Started",
    "fallback",
  );

  assert.equal(message, "Global services restarted.");
});

test("summarizeActionMessage uses fallback when message is empty", () => {
  const message = summarizeActionMessage("   \n\t", "Global restart completed.");

  assert.equal(message, "Global restart completed.");
});

test("formatBulkGlobalActionErrorForTest extracts concise root cause from docker multiline error", () => {
  const rawError = `restart global services: exit status 1: Container govard-proxy-mail Creating
Container govard-proxy-pma Creating
Container govard-proxy-portainer Creating
Container govard-proxy-dnsmasq Creating
Container govard-proxy-caddy Creating
Container govard-proxy-portainer Created
Container govard-proxy-dnsmasq Created
Container govard-proxy-caddy Created
Container govard-proxy-pma Created
Container govard-proxy-mail Created
Container govard-proxy-portainer Starting
Container govard-proxy-caddy Starting
Container govard-proxy-pma Starting
Container govard-proxy-dnsmasq Starting
Container govard-proxy-mail Starting
Container govard-proxy-portainer Started
Container govard-proxy-mail Started
Container govard-proxy-dnsmasq Started
Container govard-proxy-pma Started
Error response from daemon: failed to set up container networking: driver failed programming external connectivity on endpoint govard-proxy-caddy (4b3af875264477d27d5e4527d9a913141b73e675574c9b3903cd9ce6517a8adb): Bind for 127.0.0.1:80 failed: port is already allocated`;
  const message = formatBulkGlobalActionErrorForTest("restart", rawError);

  assert.equal(message.includes("Global restart failed"), true);
  assert.equal(message.toLowerCase().includes("port 80"), true);
  assert.equal(message.includes("\n"), false);
  assert.equal(message.length <= 180, true);
});

test("formatBulkGlobalActionErrorForTest falls back to a concise one-line message", () => {
  const message = formatBulkGlobalActionErrorForTest(
    "pull",
    "pull global services: exit status 1: very long unexpected output without a clear marker that should still be rendered in one line for the bulk action feedback area",
  );

  assert.equal(message.includes("Global pull failed"), true);
  assert.equal(message.includes("\n"), false);
  assert.equal(message.length <= 180, true);
});

test("formatBulkGlobalActionErrorForTest returns default text when error is empty", () => {
  assert.equal(
    formatBulkGlobalActionErrorForTest("start", ""),
    "Global start failed.",
  );
});

test("the deck island owns the health, strip and feedback elements", async () => {
  const deck = await readIsland("GlobalHealthHeader.tsx");
  for (const id of [
    "globalServiceHealthPercent",
    "globalServiceHealthBar",
    "globalServiceStatusStrip",
    "globalActionFeedback",
  ]) {
    assert.equal(deck.includes(`id="${id}"`), true, `missing deck element ${id}`);
  }
  for (const action of ["start", "restart", "stop", "pull"]) {
    assert.equal(
      deck.includes(`testid: "global-bulk-${action}"`),
      true,
      `missing bulk action ${action}`,
    );
  }
  for (const label of [
    "Starting All...",
    "Restarting All...",
    "Stopping All...",
    "Pulling All...",
  ]) {
    assert.equal(deck.includes(`"${label}"`), true, `missing bulk loading label ${label}`);
  }
});

test("the log pane's controls live in its island, not in the entry points", async () => {
  const island = await readIsland("GlobalLogsPanel.tsx");
  for (const id of [
    "globalLogSearch",
    "globalLogSeverity",
    "globalLogServiceName",
    "globalLogViewport",
    "globalLogOutput",
    "globalToggleLive",
  ]) {
    assert.equal(island.includes(`id="${id}"`), true, `missing ${id} in the log pane island`);
  }
  for (const testid of [
    "refresh-global-logs",
    "clear-global-logs",
    "download-global-logs",
    "global-toggle-live",
  ]) {
    assert.equal(island.includes(`data-testid="${testid}"`), true, `missing ${testid}`);
  }

  // ... and the entry points hand the whole panel to one container instead.
  for (const name of ["index.html", "preview.html"]) {
    const html = await readEntryPoint(name);
    assert.equal(
      html.includes('id="globalLogsIsland"'),
      true,
      `${name} is missing the log pane container`,
    );
    for (const action of [
      "toggle-global-live",
      "refresh-global-logs",
      "clear-global-logs",
      "download-global-logs",
      "filter-global-severity",
    ]) {
      assert.equal(
        html.includes(`data-action="${action}"`),
        false,
        `${name} still routes ${action} through the delegate`,
      );
    }
  }
});

test("the card island keeps the per-service loading contracts", async () => {
  const island = await readIsland("GlobalServicesList.tsx");
  assert.equal(island.includes('"Restarting..."'), true);
  assert.equal(island.includes('"Starting..."'), true);
  assert.equal(island.includes('data-loading-label="Stopping..."'), true);
  assert.equal(island.includes('data-loading-label="Opening..."'), true);
  assert.equal(
    island.includes("progress_activity"),
    true,
    "missing the loading spinner glyph for global service actions",
  );
  assert.equal(island.includes("aria-busy"), true, "missing the aria-busy state");
});
