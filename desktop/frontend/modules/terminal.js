export const createTerminalController = ({
  bridge,
  runtime,
  refs,
  onStatus,
  onToast,
  readSelection,
}) => {
  let activeTerminal = null;
  let activeSessionId = null;
  let fitAddon = null;
  let resizeObserver = null;

  const updateRefs = (newRefs) => {
    refs = newRefs;
  };

  const initTerminal = () => {
    if (activeTerminal || !refs.terminalContainer) return;

    // Check if xterm.js loaded from vendor
    if (!window.Terminal || !window.FitAddon) {
      if (onToast)
        onToast(
          "Terminal emulator failed to load from vendor scripts",
          "warning",
        );
      return;
    }

    activeTerminal = new window.Terminal({
      theme: {
        background: "#0c1810",
        foreground: "#f8fafc",
        cursor: "#0df259",
      },
      fontFamily: "monospace",
      fontSize: 13,
      cursorBlink: true,
    });

    fitAddon = new window.FitAddon.FitAddon();
    activeTerminal.loadAddon(fitAddon);
    activeTerminal.open(refs.terminalContainer);
    fitAddon.fit();

    activeTerminal.onData((data) => {
      if (activeSessionId) {
        bridge.writeTerminal(activeSessionId, data).catch((err) => {
          console.error("Write terminal failed:", err);
        });
      }
    });

    resizeObserver = new ResizeObserver(() => {
      if (fitAddon && activeSessionId) {
        fitAddon.fit();
        bridge
          .resizeTerminal(
            activeSessionId,
            activeTerminal.cols,
            activeTerminal.rows,
          )
          .catch(() => {});
      }
    });
    resizeObserver.observe(refs.terminalContainer);

    if (runtime?.EventsOn) {
      runtime.EventsOn("terminal:output", (payload) => {
        if (payload && payload.id === activeSessionId && activeTerminal) {
          activeTerminal.write(payload.data);
        }
      });

      runtime.EventsOn("terminal:exit", (payload) => {
        if (payload && payload.id === activeSessionId && activeTerminal) {
          activeTerminal.write("\r\n\r\n[Process Exited]\r\n");
          if (onStatus) onStatus("Status: Terminal exited");
        }
      });
    }
  };

  const startSession = async () => {
    try {
      const { project, service } = readSelection();
      if (!project) {
        onToast("Please select an environment", "warning");
        return;
      }

      if (!refs.terminalContainer) {
        onToast("Terminal requires a parent element.", "error");
        return;
      }

      const userSelect = document.getElementById("shellUser");
      const cmdSelect = document.getElementById("shellCommand");
      const user = userSelect?.value || "";
      const shell = cmdSelect?.value || "";

      if (!activeTerminal) {
        initTerminal();
      }

      if (!activeTerminal) return; // failed to init

      activeTerminal.clear();
      activeTerminal.write(`Connecting to ${service}...\r\n`);

      const sessionID = await bridge.startTerminal(
        project,
        service,
        user,
        shell,
      );

      activeSessionId = sessionID;

      if (fitAddon) {
        fitAddon.fit();
        await bridge.resizeTerminal(
          activeSessionId,
          activeTerminal.cols,
          activeTerminal.rows,
        );
      }

      if (onStatus) onStatus(`Status: Terminal connected to ${service}`);
    } catch (err) {
      if (onToast) onToast(err.message || "Failed to start terminal", "error");
    }
  };

  const resize = () => {
    if (fitAddon && activeSessionId) {
      fitAddon.fit();
      bridge
        .resizeTerminal(
          activeSessionId,
          activeTerminal.cols,
          activeTerminal.rows,
        )
        .catch(() => {});
    }
  };

  const startGovardSession = async (project, commandName, argsList) => {
    try {
      if (!activeTerminal) {
        initTerminal();
      }

      if (!activeTerminal) return;

      activeTerminal.clear();
      activeTerminal.write(`Running govard ${commandName}...\r\n`);

      const sessionID = await bridge.startGovardTerminal(project, argsList);

      activeSessionId = sessionID;

      if (fitAddon) {
        fitAddon.fit();
        await bridge.resizeTerminal(
          activeSessionId,
          activeTerminal.cols,
          activeTerminal.rows,
        );
      }

      if (onStatus) onStatus(`Status: Running govard ${commandName}`);
    } catch (err) {
      if (onToast)
        onToast(err.message || `Failed to run govard ${commandName}`, "error");
    }
  };

  return {
    updateRefs,
    startSession,
    startGovardSession,
    resize,
  };
};
