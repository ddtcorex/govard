# Govard Desktop Improvement Plan

This plan outlines 5 phases to improve the Govard desktop application, focusing on offline capability, code structure, versioning, and maintainability.

---

## Phase 1: Bundle Local Assets (COMPLETED)

Eliminated CDN dependencies and set up local Tailwind CSS build.

## Phase 2: Consolidate CSS (COMPLETED)

Moved inline CSS from [index.html](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/index.html) to [styles-src.css](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/styles-src.css).

## Phase 3: Version Injection (COMPLETED)

Established single source of truth for versioning from Go to frontend.

---

## Phase 4: Extract God Object (COMPLETED)

Successfully refactored the monolithic [App](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/app.go#29-42) struct into domain-specific services (`SettingsService`, `OnboardingService`, etc.) and stabilized test coverage following domain normalization.

---

## Phase 6: Final Compliance Audit

Ensuring 100% alignment with the Review Report (Phases 1-5 addressed most, this is the final sweep).

### [MODIFY] [utils.go](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/utils.go) [NEW]

- Create [utils.go](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/utils.go) and move helper functions from [dashboard.go](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/dashboard.go) and [onboarding.go](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/onboarding.go).

### [MODIFY] [index.html](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/index.html)

- Replace external Google Hero image with a local asset or CSS gradient.
- Investigate localizing Material Symbols.

### [MODIFY] [wails.json](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/wails.json)

- Final check on metadata.

## Phase 5: Split Monolithic HTML (NEXT)

> **Goal:** Reduce [index.html](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/index.html) from ~1500 lines to ~200 lines by moving modals and drawers to JS templates.

### [MODIFY] [index.html](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/index.html)

- Remove hardcoded modal HTML (Onboarding, Sync, Settings).
- Replace with empty target containers.

### [MODIFY] Frontend Modules

- Implement [render()](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/modules/logs.js#300-501) functions in `onboarding.js`, [settings.js](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/modules/settings.js), and [remotes.js](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/modules/remotes.js) to inject HTML templates dynamically using template literals.

---

## Phase 7: Remove Dashboard System Resources

Completely remove the "System Resources" section from the Dashboard tab while preserving health metrics in the footer.

### [MODIFY] [index.html](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/index.html)

- Remove the HTML block for "System Resources" in the dashboard tab.
- Keep the footer elements for CPU and Memory.

### [MODIFY] [metrics.js](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/modules/metrics.js)

- Remove `renderMetricSummary`, [renderMetricSkeletons](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/modules/metrics.js#84-85), and `renderMetricProjects`.
- Simplify [refresh](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/modules/logs.js#177-200) to use [GetSystemMetrics](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/bridge_proxies.go#54-57) instead of [GetResourceMetrics](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/wailsjs/go/desktop/App.js#21-24).

### [MODIFY] [metrics.go](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/metrics.go)

- Remove `buildResourceMetricsInternal` and [GetResourceMetrics](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/wailsjs/go/desktop/App.js#21-24).
- Clean up unused types and helper functions.

### [MODIFY] [types.go](file:///home/kai/Work/htdocs/ddtcorex/govard/internal/desktop/types.go)

- Remove unused resource metric types.

## Phase 8: Final Polish & Audit (COMPLETED)

Successfully localized all external assets, refactored logic for robust error propagation, and modularized the Logs/Terminal UI.

## Visual Consistency and Style Preservation

## Goal

Ensure that the refactored frontend code maintains the premium modern aesthetic (dark mode, glassmorphism, animations) and that the removal of stale components did not cause layout shifts or visual regressions.

## Proposed Changes

### [VERIFY] Visual Audit

- Perform side-by-side comparison of current UI against previous working screenshots.
- Check "New Environment" button and sidebar hover states.
- Verify Hero section gradients and typography.
- Ensure "Loading..." skeletons for dashboard tiles (Active/Services/Queue) still work and match the design system.

## Phase 10: Environment Restoration and Verified Dashboard Launch

### Goal

Restore the development environment (Go, Wails) and perform a definitive verification of the fixed dashboard in a clean session.

### Proposed Changes

#### [VERIFY] Toolchain Activation

- Fix missing [go](file:///tmp/test_user.go) and `wails` binaries in the current shell session.
- Verify `make dev-fast` or equivalent command availability for starting the dashboard.

#### [NEW] High-Entropy Cache Busting

- Apply a uniquely generated cache buster in [index.html](file:///home/kai/Work/htdocs/ddtcorex/govard/desktop/frontend/index.html) (e.g., `?v=restore-20260301`) to guarantee zero stale asset interference.

#### [VERIFY] Functional Sign-Off

- Confirm environment switching (Hero title updates).
- Confirm zero `TypeError` in browser console.
- Confirm persistent style fidelity (dark mode, glassmorphism).

## Verification Plan

### Automated Tests

- Run `make test-fast` to ensure no regression.
- Execute a final subagent browser check with hard-reload logic.

## Verification Plan

### Manual Verification

- Hard refresh browser and check for flash of unstyled content (FOUC).
- Validate glass-card styling and borders in the sidebar.
- Inspect footer metrics for correct font and alignment.

## Final Verification Summary

- **Automated Tests**: Completed `make test-fast` - **100% Green**.
- **Offline Capability**: Verified - **All assets local**.
- **Error Handling**: Verified - **Explicit propagation implemented**.
- **UI Modularization**: Verified - **Atomic components established**.

## Phase 11: Design Fidelity Audit & Aesthetics Restoration

### Goal

Restore 100% design fidelity by auditing the current UI against the reference mockups in `desktop/frontend/design`.

### Proposed Changes

#### [VERIFY] Reference Comparison

- Analyze reference files in `desktop/frontend/design` (dashboard, logs, remotes).
- Identify missing HTML structures in modular versions of `dashboard.js`, `logs.js`, and `remotes.js`.
- Diff CSS tokens in `styles-src.css` against design styles.

#### [MODIFY] Restoration Implementation

- **Dashboard**: Restore **Active Services**, **Quick Actions**, and **Environment Variables** sections in `index.html` or `dashboard.js` (ensuring they populate correctly).
- **Remotes**: Restore the animated **Sync Flow diagram** and **Sync Configuration** toggles in `modules/remotes.js`.
- **Logs**: Re-introduce **Severity Filters** (`All`, `Error`, `Warn`) and **Service Filters** in the modular Logs header.
- **Settings**: Fix the **Settings Drawer** initialization in `main.js` to restore interactivity.
- **Aesthetics**: Restore evocative Hero background pattern/image and refine glassmorphism blur/opacity tokens.

---

## Phase 12: Comprehensive Validation & Functional Fixes

### Goal

Successfully pass the 8-step validation criteria provided by the user, ensuring data accuracy and full interactivity across all tabs.

### Proposed Changes

#### [VERIFY] Data Integrity & Environment Listing
- Check Go backend (`internal/desktop/dashboard.go` and `internal/engine/registry`) to ensure `magento2-test-instance` is properly registered or mocked for the dev session.
- Verify service mapping logic to ensure 5-6 services are returned for the selected project.
- Verify remote snapshot logic returns the 3 required environments (dev, staging, prod).

#### [FIX] UI Responsiveness & Bridge Calls
- Ensure all "Quick Actions" and "Environment Variables" are not just rendered but correctly wired to the bridge.
- Fix any potential race conditions where switching environments might fail to update the right panel.

#### [SETUP] Clean Dev Environment
- Automated cleanup script to kill legacy `wails` and `govard` processes on port 34115.

## Verification Plan

### Manual Verification (User Steps)
1. Launch `govard desktop --dev`.
2. Check `http://localhost:34115`.
3. Verify `magento2-test-instance` in sidebar.
4. Click and verify info update.
5. Count services in Dashboard (Target: 5-6).
6. Check Remotes list (Target: dev, staging, prod).
7. Verify Logs & Shell terminal interactivity.
