import test from "node:test";
import assert from "node:assert/strict";
import { formatUpdateMessage } from "../../desktop/frontend/modules/update-message.js";

// Characterization tests: update-message.js is unchanged by the island
// migration, so these pin what it does today.

test("keeps a custom message and can append the version transition", () => {
  const msg = formatUpdateMessage(
    {
      message: "Security fixes for the proxy.",
      currentVersion: "1.0.0",
      latestVersion: "1.1.0",
    },
    { includeVersionTransition: true },
  );
  assert.equal(msg, "Security fixes for the proxy (1.0.0 -> 1.1.0).");
});

test("keeps a custom message unchanged without the transition option", () => {
  const msg = formatUpdateMessage({
    message: "Security fixes for the proxy.",
    currentVersion: "1.0.0",
    latestVersion: "1.1.0",
  });
  assert.equal(msg, "Security fixes for the proxy.");
});

test("does not append a transition the message already carries", () => {
  const msg = formatUpdateMessage(
    {
      message: "Ships 1.0.0 and 1.1.0 fixes.",
      currentVersion: "1.0.0",
      latestVersion: "1.1.0",
    },
    { includeVersionTransition: true },
  );
  assert.equal(msg, "Ships 1.0.0 and 1.1.0 fixes.");
});

test("drops a server message that only repeats the version change", () => {
  for (const message of [
    "Update available: 1.0.0 -> 1.1.0",
    "Update available: 1.0.0 → 1.1.0",
    "Current 1.0.0, latest 1.1.0",
    "New version -> soon",
  ]) {
    const msg = formatUpdateMessage({
      message,
      currentVersion: "1.0.0",
      latestVersion: "1.1.0",
    });
    assert.equal(msg, "A new Govard Desktop version is ready to install.", message);
  }
});

test("a redundant message with the transition option gets the canonical text", () => {
  const msg = formatUpdateMessage(
    {
      message: "Update available: 1.0.0 -> 1.1.0",
      currentVersion: "1.0.0",
      latestVersion: "1.1.0",
    },
    { includeVersionTransition: true },
  );
  assert.equal(msg, "A new Govard Desktop version is ready to install (1.0.0 -> 1.1.0).");
});

test("falls back by which versions are known", () => {
  assert.equal(
    formatUpdateMessage({ message: "", currentVersion: "", latestVersion: "1.1.0" }),
    "Version 1.1.0 is ready to install.",
  );
  assert.equal(
    formatUpdateMessage({ message: "", currentVersion: "1.0.0", latestVersion: "" }),
    "A new Govard Desktop version is available.",
  );
  assert.equal(
    formatUpdateMessage({ message: "   " }),
    "A new Govard Desktop version is available.",
  );
});
