import test from "node:test"
import assert from "node:assert/strict"

import { normalizeSettingsPayload } from "../../desktop/frontend/modules/settings.js"

test("normalizeSettingsPayload maps settings payload", () => {
  const value = normalizeSettingsPayload({
    theme: "dark",
    proxyTarget: "govard.test",
    preferredBrowser: "firefox",
  })
  assert.deepEqual(value, {
    theme: "dark",
    proxyTarget: "govard.test",
    preferredBrowser: "firefox",
    codeEditor: "",
  })
})

test("normalizeSettingsPayload falls back to defaults", () => {
  const value = normalizeSettingsPayload({})
  assert.deepEqual(value, {
    theme: "system",
    proxyTarget: "",
    preferredBrowser: "",
    codeEditor: "",
  })
})

