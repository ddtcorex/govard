import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import {
  normalizeOnboardingDomain,
  normalizeOnboardingFramework,
} from "../../desktop/frontend/modules/onboarding.js";
import { desktopBridge, __setBindingsLoaderForTest } from "../../desktop/frontend/services/bridge.js";

test("normalizeOnboardingFramework canonicalizes empty and aliases", () => {
  assert.equal(normalizeOnboardingFramework(""), "");
  assert.equal(normalizeOnboardingFramework("auto"), "");
  assert.equal(normalizeOnboardingFramework("m2"), "magento2");
  assert.equal(normalizeOnboardingFramework("magento2"), "magento2");
  assert.equal(normalizeOnboardingFramework("custom"), "custom");
});

test("normalizeOnboardingDomain auto-appends .test for plain values", () => {
  assert.equal(
    normalizeOnboardingDomain("shop", "/tmp/ignored"),
    "shop.test",
  );
  assert.equal(
    normalizeOnboardingDomain("", "/tmp/sample-project"),
    "sample-project.test",
  );
  assert.equal(
    normalizeOnboardingDomain("custom.test", "/tmp/ignored"),
    "custom.test",
  );
});

test("desktop layout exposes onboarding mount point", async () => {
  const html = await readFile(
    new URL("../../desktop/frontend/index.html", import.meta.url),
    "utf8",
  );
  assert.equal(
    html.includes('id="onboardingModalMount"'),
    true,
    "missing onboarding mount point",
  );
});

test("the onboarding island renders the wizard's UI contract", async () => {
  const markup = await readFile(
    new URL("../../desktop/frontend/islands/OnboardingModal.tsx", import.meta.url),
    "utf8",
  );

  for (const id of [
    "projectDomainHint",
    "onboardingSummaryDomain",
    "projectFrameworkVersion",
    "projectFrameworkVersionHint",
    "onboardingSubmitHint",
    "onboardingSubmitSpinner",
    "onboardingBootstrapOptions",
    "onboardFromGit",
    "gitProtocol",
    "gitUrl",
    "gitUrlHint",
    "gitConfirmOverride",
    "gitConfirmHint",
    "projectPathCard",
  ]) {
    assert.equal(markup.includes(`id="${id}"`), true, `missing ${id}`);
  }

  assert.equal(
    markup.includes('id="detectionState"'),
    false,
    "legacy detection card should be removed",
  );
  assert.equal(
    markup.includes("Step 1 of 3"),
    false,
    "legacy timeline block should be removed",
  );
  assert.equal(
    markup.includes('role="button"'),
    true,
    "project path card should be keyboard focusable",
  );
  // The card was a delegate target; the island owns the click and the keyboard
  // path main.js's document-wide keydown handler used to provide for it.
  const browseCount = (markup.match(/data-testid="browse-project"/g) || []).length;
  assert.equal(browseCount, 1, "expected exactly one browse action target");
  assert.equal(
    markup.includes("onKeyDown"),
    true,
    "the path card must keep its keyboard activation",
  );
});

test("desktopBridge onboarding forwards framework version", async () => {
  let capturedPayload = null;
  const restore = __setBindingsLoaderForTest(async () => ({
    OnboardingService: {
      OnboardProject: async (payload) => {
        capturedPayload = payload;
        return "ok";
      },
    },
  }));

  try {
    await desktopBridge.onboardProject({
      projectPath: "/tmp/sample-project",
      framework: "laravel",
      frameworkVersion: "11",
      domain: "sample-project.test",
    });
  } finally {
    restore();
  }

  assert.equal(capturedPayload?.frameworkVersion, "11");
});
