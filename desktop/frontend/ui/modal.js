import { byId, setHTML, setText } from "../utils/dom.js";
import {
  focusFirstElement,
  rememberFocus,
  trapTabKey,
} from "../utils/focus-trap.js";

const modal = byId("confirmModal");
const titleEl = byId("confirmTitle");
const messageEl = byId("confirmMessage");
const iconEl = byId("confirmIcon");
const cancelBtn = byId("confirmCancelBtn");
const confirmBtn = byId("confirmConfirmBtn");

let onConfirm = null;
let onCancel = null;
let isOpen = false;
/** Focus restore for the element that was focused before the dialog opened. */
let restoreFocus = null;

const hide = () => {
  if (!modal) return;
  isOpen = false;
  modal.classList.add("opacity-0", "pointer-events-none");
  modal.classList.remove("opacity-100", "pointer-events-auto");
  const inner = modal.querySelector("div");
  if (inner) {
    inner.classList.add("scale-95");
    inner.classList.remove("scale-100");
  }
  if (restoreFocus) {
    restoreFocus();
    restoreFocus = null;
  }
};

const show = () => {
  if (!modal) return;
  isOpen = true;
  restoreFocus = rememberFocus();
  modal.classList.remove("opacity-0", "pointer-events-none");
  modal.classList.add("opacity-100", "pointer-events-auto");
  const inner = modal.querySelector("div");
  if (inner) {
    inner.classList.remove("scale-95");
    inner.classList.add("scale-100");
  }
  // Move focus into the dialog; the cancel control is the safe default.
  if (cancelBtn) {
    cancelBtn.focus();
  } else {
    focusFirstElement(modal);
  }
};

if (cancelBtn) {
  cancelBtn.addEventListener("click", () => {
    hide();
    if (onCancel) onCancel();
  });
}

if (confirmBtn) {
  confirmBtn.addEventListener("click", () => {
    hide();
    if (onConfirm) onConfirm();
  });
}

// Baseline dialog keyboard contract: Escape cancels, Tab cycles inside.
// Guarded to the function (not just the global) so the fake-document unit
// tests that stub getElementById-only keep importing this module.
if (typeof document !== "undefined" && typeof document.addEventListener === "function") {
  document.addEventListener("keydown", (event) => {
    if (!modal) return;
    // Two open paths drive this node: confirm() below (isOpen) and main.js
    // showConfirm (is-visible class). Both close on Escape.
    const visible = isOpen || modal.classList.contains("is-visible");
    if (!visible) return;
    if (event.key === "Escape") {
      event.preventDefault();
      if (isOpen) {
        hide();
        if (onCancel) onCancel();
      } else if (cancelBtn) {
        // The main.js path owns its promise: a cancel click runs its cleanup.
        cancelBtn.click();
      }
      return;
    }
    trapTabKey(modal, event);
  });
}

/**
 * Show a themed confirmation dialog.
 * 
 * @param {Object} options
 * @param {string} options.title - Modal title
 * @param {string} options.message - Modal message
 * @param {string} options.icon - Material icon name
 * @param {string} options.confirmLabel - Label for confirm button
 * @param {string} options.cancelLabel - Label for cancel button
 * @returns {Promise<boolean>} - Resolves to true if confirmed, false if cancelled
 */
export const confirm = ({ title, message, icon, confirmLabel, cancelLabel }) => {
  return new Promise((resolve) => {
    setText(titleEl, title || "Confirm Action");
    setHTML(messageEl, message || "Are you sure you want to proceed?");
    setText(iconEl, icon || "help");
    setText(confirmBtn, confirmLabel || "Confirm");
    setText(cancelBtn, cancelLabel || "Cancel");
    
    onConfirm = () => resolve(true);
    onCancel = () => resolve(false);
    
    show();
  });
};
