import { escapeHTML } from "../utils/dom.js";
import { sanitizeUIText } from "../utils/sanitize.js";

/** Toasts show short human messages: raw log tails stay in their panel. */
const TOAST_MAX_CHARS = 280;

// toastKind allowlists the style variant interpolated into the toast
// className: only known kinds reach the DOM, anything else falls back to the
// caller's default so a hostile or mistyped kind cannot inject markup.
const TOAST_KINDS = ["success", "error", "warning", "info"];

// toastKind is shared with main.js's headless loading-toast fallback so both
// paths collapse unknown kinds to a default instead of interpolating them.
export const toastKind = (type, fallback) =>
  TOAST_KINDS.includes(type) ? type : fallback;

// copyOpMessageText handles the toast "copy" action internally: the hint is
// already on screen, so only the clipboard write is needed. Failures stay
// silent on purpose, the hint remains visible either way.
const copyOpMessageText = (text) => {
  try {
    const result = globalThis.navigator?.clipboard?.writeText?.(String(text ?? ""));
    if (result?.catch) result.catch(() => {});
  } catch {
    // Clipboard unavailable (headless/WebKit context): hint stays visible.
  }
};

export const createToast = (container) => {
  const activeMessages = new Set();

  const getIcon = (type) => {
    switch (type) {
      case "error":
        return "report";
      case "warning":
        return "warning";
      case "info":
        return "info";
      default:
        return "check_circle";
    }
  };

  const show = (message, type = "success") => {
    if (!container) return;

    const kind = toastKind(type, "success");
    const msg = sanitizeUIText(message, TOAST_MAX_CHARS).trim();
    if (!msg) return;
    if (activeMessages.has(msg)) return;
    activeMessages.add(msg);

    const item = document.createElement("div");
    item.className = `toast toast--${kind} group`;

    const duration = kind === "error" || kind === "warning" ? 6000 : 4000;

    item.innerHTML = `
      <div class="toast-indicator"></div>
      <div class="toast-icon-wrapper">
        <span class="material-symbols-outlined toast-icon">${getIcon(type)}</span>
      </div>
      <div class="toast-content">
        <p class="toast-message">${escapeHTML(msg)}</p>
      </div>
      <button class="toast-close" aria-label="Close">
        <span class="material-symbols-outlined">close</span>
      </button>
      <div class="toast-progress" style="animation: toast-shrink ${duration}ms linear forwards"></div>
    `;

    container.appendChild(item);

    const close = () => {
      if (item.classList.contains("is-removing")) return;
      item.classList.add("is-removing");
      item.classList.remove("is-visible");
      setTimeout(() => {
        item.remove();
        activeMessages.delete(msg);
      }, 500);
    };

    item.querySelector(".toast-close").addEventListener("click", (e) => {
      e.stopPropagation();
      close();
    });

    requestAnimationFrame(() => item.classList.add("is-visible"));
    setTimeout(close, duration);
  };

  // showStreaming creates a persistent toast that can be updated externally for streams.
  // The stream line renders the latest short human message (e.g. a structured
  // sync:progress message), never the raw tail: every update goes through the
  // shared UI-text sanitizer, while full detail stays in the owning panel.
  const showStreaming = (title = "Syncing...", type = "info", options = {}) => {
    if (!container) return null;

    const kind = toastKind(type, "info");
    const msg = sanitizeUIText(title, TOAST_MAX_CHARS).trim() || "Syncing...";
    const dedupeKey =
      options?.dedupeKey === undefined ? msg : options?.dedupeKey;
    const shouldDedupe = dedupeKey !== false && dedupeKey !== null;

    if (shouldDedupe && activeMessages.has(dedupeKey)) return null;
    if (shouldDedupe) activeMessages.add(dedupeKey);

    const item = document.createElement("div");
    item.className = `toast toast--${kind} group`;

    item.innerHTML = `
      <div class="toast-indicator"></div>
      <div class="toast-icon-wrapper">
        <span class="material-symbols-outlined toast-icon">${getIcon(kind)}</span>
      </div>
      <div class="toast-content" style="max-height: 200px; overflow-y: auto; overflow-x: hidden;">
        <div style="display:flex;align-items:center;gap:6px;">
          <p class="toast-message font-bold" style="margin:0;">${escapeHTML(msg)}</p>
          <span class="toast-spinner" style="
            display: inline-block;
            width: 12px;
            height: 12px;
            border: 2px solid rgba(255,255,255,0.3);
            border-top-color: #fff;
            border-radius: 50%;
            animation: toast-spin 0.7s linear infinite;
            flex-shrink: 0;
          "></span>
        </div>
        <p class="toast-stream-line text-xs font-mono opacity-80 mt-1 wrap-break-word">Starting...</p>
      </div>
      <button class="toast-close" aria-label="Close">
        <span class="material-symbols-outlined">close</span>
      </button>
    `;

    // Inject the keyframe if not already present
    if (!document.getElementById("toast-spin-style")) {
      const style = document.createElement("style");
      style.id = "toast-spin-style";
      style.textContent = `@keyframes toast-spin { to { transform: rotate(360deg); } }`;
      document.head.appendChild(style);
    }

    container.appendChild(item);

    const streamLineEl = item.querySelector(".toast-stream-line");
    const spinnerEl = item.querySelector(".toast-spinner");

    const close = () => {
      if (item.classList.contains("is-removing")) return;
      item.classList.add("is-removing");
      item.classList.remove("is-visible");
      setTimeout(() => {
        item.remove();
        if (shouldDedupe) {
          activeMessages.delete(dedupeKey);
        }
      }, 500);
    };

    item.querySelector(".toast-close").addEventListener("click", (e) => {
      e.stopPropagation();
      close();
    });

    requestAnimationFrame(() => item.classList.add("is-visible"));

    return {
      update: (line) => {
        if (streamLineEl && line) {
          streamLineEl.textContent = sanitizeUIText(line, TOAST_MAX_CHARS);
          // Auto-scroll to bottom of the content area inside the toast
          const contentArea = item.querySelector(".toast-content");
          if (contentArea) contentArea.scrollTop = contentArea.scrollHeight;
        }
      },
      close: (finalLabel, finalType = "success") => {
        // Hide the spinner once the process is done
        if (spinnerEl) spinnerEl.style.display = "none";
        if (streamLineEl && finalLabel) {
          streamLineEl.textContent = sanitizeUIText(finalLabel, TOAST_MAX_CHARS);
        }
        if (finalType !== type) {
          const finalKind = toastKind(finalType, "success");
          item.className = `toast toast--${finalKind} group is-visible`;
          item.querySelector(".toast-icon").textContent = getIcon(finalKind);
        }
        // Auto-dismiss after 4 seconds once it reports completion
        setTimeout(close, 4000);
      },
    };
  };

  // showOpMessage renders a human operation message (see humanOpMessage in
  // internal/desktop/operation_messages.go): a bold title, a secondary hint,
  // and one button per action. The "copy" action is handled internally by
  // writing the hint to the clipboard; every other action id goes to
  // onAction. All strings pass through the shared UI-text sanitizer and
  // reach the DOM escaped, so CLI-controlled text cannot inject markup.
  const showOpMessage = (msg = {}, type = "error", onAction) => {
    if (!container) return null;

    const kind = toastKind(type, "error");
    const title = sanitizeUIText(msg?.title, TOAST_MAX_CHARS).trim();
    const hint = sanitizeUIText(msg?.hint, TOAST_MAX_CHARS).trim();
    if (!title && !hint) return null;
    const actions = Array.isArray(msg?.actions) ? msg.actions : [];

    const duration = 8000;

    const item = document.createElement("div");
    item.className = `toast toast--${kind} group`;

    item.innerHTML = `
      <div class="toast-indicator"></div>
      <div class="toast-icon-wrapper">
        <span class="material-symbols-outlined toast-icon">${getIcon(kind)}</span>
      </div>
      <div class="toast-content">
        ${title ? `<p class="toast-message font-bold" style="margin:0;">${escapeHTML(title)}</p>` : ""}
        ${hint ? `<p class="toast-hint text-xs opacity-80 mt-1 wrap-break-word" style="margin:4px 0 0;">${escapeHTML(hint)}</p>` : ""}
        <div class="toast-actions" style="display:flex;gap:6px;margin-top:8px;"></div>
      </div>
      <button class="toast-close" aria-label="Close">
        <span class="material-symbols-outlined">close</span>
      </button>
      <div class="toast-progress" style="animation: toast-shrink ${duration}ms linear forwards"></div>
    `;

    const actionsEl = item.querySelector(".toast-actions");
    const buttons = [];
    for (const action of actions) {
      const id = sanitizeUIText(action?.id, 64).trim();
      const label = sanitizeUIText(action?.label, 64).trim();
      if (!id || !label) continue;
      const button = document.createElement("button");
      button.className = "toast-action";
      button.textContent = label;
      button.dataset.action = id;
      button.setAttribute?.("type", "button");
      button.addEventListener("click", (e) => {
        e.stopPropagation();
        if (id === "copy") {
          copyOpMessageText(hint || title);
          return;
        }
        if (typeof onAction === "function") onAction(id);
      });
      actionsEl?.appendChild?.(button);
      buttons.push(button);
    }
    if (typeof container.appendChild === "function") container.appendChild(item);

    const close = () => {
      if (item.classList.contains("is-removing")) return;
      item.classList.add("is-removing");
      item.classList.remove("is-visible");
      setTimeout(() => {
        item.remove();
      }, 500);
    };

    item.querySelector(".toast-close").addEventListener("click", (e) => {
      e.stopPropagation();
      close();
    });

    requestAnimationFrame(() => item.classList.add("is-visible"));
    setTimeout(close, duration);

    return {
      close,
      actions: buttons.map((button) => button.dataset.action),
    };
  };

  return { show, showStreaming, showOpMessage };
};
