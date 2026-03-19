import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import {
  normalizeOnboardingDomain,
  normalizeOnboardingFramework,
  renderOnboardingModal,
} from "../../desktop/frontend/modules/onboarding.js";

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

test("renderOnboardingModal exposes streamlined onboarding UI contract", () => {
  const container = { innerHTML: "" };
  renderOnboardingModal(container);
  const markup = String(container.innerHTML || "");

  assert.equal(
    markup.includes('id="projectDomainHint"'),
    true,
    "missing domain hint",
  );
  assert.equal(
    markup.includes('id="onboardingSummaryDomain"'),
    true,
    "missing summary domain field",
  );
  assert.equal(
    markup.includes('id="onboardingSubmitHint"'),
    true,
    "missing submit readiness hint",
  );
  assert.equal(
    markup.includes('id="onboardingSubmitSpinner"'),
    true,
    "missing onboarding submit spinner",
  );
  assert.equal(
    markup.includes('id="onboardingBootstrapOptions"'),
    true,
    "missing onboarding bootstrap options container",
  );
  assert.equal(
    markup.includes('id="onboardFromGit"'),
    true,
    "missing git onboarding toggle",
  );
  assert.equal(
    markup.includes('id="gitProtocol"'),
    true,
    "missing git protocol selector",
  );
  assert.equal(
    markup.includes('id="gitUrl"'),
    true,
    "missing git URL input",
  );
  assert.equal(
    markup.includes('id="gitUrlHint"'),
    true,
    "missing git URL hint",
  );
  assert.equal(
    markup.includes('id="gitConfirmOverride"'),
    true,
    "missing git folder override confirmation",
  );
  assert.equal(
    markup.includes('id="gitConfirmHint"'),
    true,
    "missing git confirmation hint",
  );
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
    markup.includes('id="projectPathCard"'),
    true,
    "missing project path card container",
  );
  assert.equal(
    markup.includes('data-action="browse-project"'),
    true,
    "project path card should trigger browse action",
  );
  assert.equal(
    markup.includes('role="button"'),
    true,
    "project path card should be keyboard focusable",
  );
  const browseActionCount = (
    markup.match(/data-action="browse-project"/g) || []
  ).length;
  assert.equal(
    browseActionCount === 1,
    true,
    "expected exactly one browse action target",
  );
});
