## 2025-02-18 - Missing ARIA Labels on Icon-Only Buttons
**Learning:** The application uses several icon-only buttons (notifications, settings, profile, search, etc.) which lack `aria-label` attributes. This makes them inaccessible to screen reader users who would only hear "button" or "unlabeled button".
**Action:** Always verify that icon-only buttons have an accessible name using `aria-label` or `aria-labelledby`.

## 2025-02-18 - Label in Name (WCAG 2.5.3)
**Learning:** Adding `aria-label` to an element with visible text overrides the visible text in the accessibility tree. This can be problematic if the visible text is "John Dev" but the accessible name becomes "User profile", potentially confusing speech recognition users.
**Action:** Avoid overriding visible text with `aria-label` unless necessary. Ensure the accessible name contains the visible text.
