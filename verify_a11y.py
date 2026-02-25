from playwright.sync_api import sync_playwright

def run(playwright):
    browser = playwright.chromium.launch()
    page = browser.new_page()
    page.goto("http://localhost:8000/index.html")

    print("Verifying accessibility labels...")

    # Header
    page.get_by_label("View notifications").wait_for()
    print("Found: View notifications")

    page.get_by_label("Open settings").wait_for()
    print("Found: Open settings")

    page.get_by_label("User Profile").wait_for()
    print("Found: User Profile")

    # Refresh is hidden by default
    refresh_btn = page.get_by_label("Refresh dashboard")
    if refresh_btn.is_hidden():
        print("Found: Refresh dashboard (hidden)")
    else:
        print("Found: Refresh dashboard (visible)")

    # Hero
    page.get_by_label("Stop environment").wait_for()
    print("Found: Stop environment")

    # Service List (multiple items)
    # Dynamic content - might be 0 in headless without bridge
    logs_btns = page.get_by_label("View logs").all()
    print(f"Found {len(logs_btns)} View logs buttons")

    term_btns = page.get_by_label("Open terminal").all()
    print(f"Found {len(term_btns)} Open terminal buttons")

    # Logs Tab Controls
    # We need to switch tab to see them visibly, but they exist in DOM.
    # However, wait_for() waits for visibility.

    print("Switching to Logs tab...")
    page.get_by_role("link", name="Logs & Shell").click()

    # Wait for tab content to be visible
    page.locator("#tab-logs").wait_for()

    # Log Controls
    page.get_by_label("Clear logs").wait_for()
    print("Found: Clear logs")

    page.get_by_label("Download logs").wait_for()
    print("Found: Download logs")

    # Sync button in footer is always visible
    page.get_by_label("Sync status").wait_for()
    print("Found: Sync status")

    # Modals (hidden)
    if page.get_by_label("Close onboarding").count() > 0:
        print("Found: Close onboarding")

    if page.get_by_label("Close settings").count() > 0:
        print("Found: Close settings")

    # Take screenshot
    page.screenshot(path="verification.png")
    print("Screenshot saved to verification.png")

    browser.close()

with sync_playwright() as playwright:
    run(playwright)
