// Shared baseline dialog accessibility helper (Task 9: dialog a11y baseline).
//
// Every dialog root (confirm, onboarding, sync, settings) wires the same three
// behaviours through these functions: Tab cycles inside the dialog, opening
// moves focus to the first control, and closing restores the focus the dialog
// stole. Keeping them here stops the fourth dialog from inventing its own.
//
// The functions take the container (and optionally the document) as arguments
// and touch no globals at import time, so node tests can drive them with fake
// elements.

export const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/** Focusable descendants, minus disabled and explicitly untabbable nodes. */
export const getFocusableElements = (container) => {
  if (!container || typeof container.querySelectorAll !== "function") {
    return [];
  }
  const nodes = Array.from(container.querySelectorAll(FOCUSABLE_SELECTOR));
  return nodes.filter((el) => !el.disabled && el.tabIndex !== -1);
};

const resolveDoc = (doc) => {
  if (doc) return doc;
  if (typeof globalThis.document !== "undefined") return globalThis.document;
  return null;
};

/**
 * Keep Tab inside an open dialog. Call from a keydown handler; returns true
 * when the event was a wrapped Tab (caller has nothing left to do).
 */
export const trapTabKey = (container, event) => {
  if (!event || event.key !== "Tab") {
    return false;
  }
  const focusable = getFocusableElements(container);
  if (focusable.length === 0) {
    return false;
  }
  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  if (event.shiftKey && event.target === first) {
    event.preventDefault();
    last.focus();
    return true;
  }
  if (!event.shiftKey && event.target === last) {
    event.preventDefault();
    first.focus();
    return true;
  }
  return false;
};

/** Focus the first control, falling back to the container itself. */
export const focusFirstElement = (container) => {
  const [first] = getFocusableElements(container);
  if (first && typeof first.focus === "function") {
    first.focus();
    return;
  }
  if (container && typeof container.focus === "function") {
    container.focus();
  }
};

/**
 * Remember who is focused now; the returned closure restores them. Dialog
 * open paths call this before moving focus, close paths call the closure.
 */
export const rememberFocus = (doc) => {
  const resolved = resolveDoc(doc);
  const previous =
    resolved && "activeElement" in resolved ? resolved.activeElement : null;
  return () => {
    if (previous && typeof previous.focus === "function") {
      previous.focus();
    }
  };
};
