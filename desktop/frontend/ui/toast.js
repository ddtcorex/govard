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
    if (!container || !message) return;

    const msg = String(message).trim();
    if (activeMessages.has(msg)) return;
    activeMessages.add(msg);

    const item = document.createElement("div");
    item.className = `toast toast--${type} group`;

    const duration = type === "error" || type === "warning" ? 6000 : 4000;

    item.innerHTML = `
      <div class="toast-indicator"></div>
      <div class="toast-icon-wrapper">
        <span class="material-symbols-outlined toast-icon">${getIcon(type)}</span>
      </div>
      <div class="toast-content">
        <p class="toast-message">${msg}</p>
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

  // showStreaming creates a persistent toast that can be updated externally for streams
  const showStreaming = (title = "Syncing...", type = "info", options = {}) => {
    if (!container) return null;

    const msg = String(title).trim();
    const dedupeKey =
      options?.dedupeKey === undefined ? msg : options?.dedupeKey;
    const shouldDedupe = dedupeKey !== false && dedupeKey !== null;

    if (shouldDedupe && activeMessages.has(dedupeKey)) return null;
    if (shouldDedupe) activeMessages.add(dedupeKey);

    const item = document.createElement("div");
    item.className = `toast toast--${type} group`;

    item.innerHTML = `
      <div class="toast-indicator"></div>
      <div class="toast-icon-wrapper">
        <span class="material-symbols-outlined toast-icon">${getIcon(type)}</span>
      </div>
      <div class="toast-content" style="max-height: 200px; overflow-y: auto; overflow-x: hidden;">
        <div style="display:flex;align-items:center;gap:6px;">
          <p class="toast-message font-bold" style="margin:0;">${msg}</p>
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
        <p class="toast-stream-line text-xs font-mono opacity-80 mt-1 break-words">Starting...</p>
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
          streamLineEl.textContent = line;
          // Auto-scroll to bottom of the content area inside the toast
          const contentArea = item.querySelector(".toast-content");
          if (contentArea) contentArea.scrollTop = contentArea.scrollHeight;
        }
      },
      close: (finalLabel, finalType = "success") => {
        // Hide the spinner once the process is done
        if (spinnerEl) spinnerEl.style.display = "none";
        if (streamLineEl && finalLabel) {
          streamLineEl.textContent = finalLabel;
        }
        if (finalType !== type) {
          item.className = `toast toast--${finalType} group is-visible`;
          item.querySelector(".toast-icon").textContent = getIcon(finalType);
        }
        // Auto-dismiss after 4 seconds once it reports completion
        setTimeout(close, 4000);
      },
    };
  };

  return { show, showStreaming };
};
