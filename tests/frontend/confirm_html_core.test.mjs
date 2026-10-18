import test from "node:test";
import assert from "node:assert/strict";

import {
  sanitizeRichConfirmHTML,
} from "../../desktop/frontend/utils/sanitize.js";

// The confirm dialog renders rich markup (line breaks, muted notes), so it
// cannot use textContent — but raw innerHTML would execute a tag smuggled
// into a message. Only <br> and a plain <small> survive; everything else is
// escaped, attributes included.
test("sanitizeRichConfirmHTML keeps only br and plain small", async () => {
  assert.equal(
    sanitizeRichConfirmHTML("Sure?<br><small class='a b'>note</small>"),
    "Sure?<br><small class=\"a b\">note</small>",
  );
  const hostile =
    "<img src=x onerror=alert(1)><script>alert(1)</script><small onclick=evil>x</small>";
  const cleaned = sanitizeRichConfirmHTML(hostile);
  assert.ok(!cleaned.includes("<img"), `img must not survive: ${cleaned}`);
  assert.ok(!cleaned.includes("<script"), `script must not survive: ${cleaned}`);
  // Escaped handler text is inert; only a live tag carrying on*= matters.
  assert.ok(
    !/<[a-z][^>]*\son\w+\s*=/i.test(cleaned),
    `live handlers must not survive: ${cleaned}`,
  );
  assert.ok(cleaned.includes("alert(1)"), "text itself stays readable");
});
