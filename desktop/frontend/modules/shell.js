export const createShellController = ({
  bridge,
  refs,
  readSelection,
  onStatus,
  onToast,
}) => {
  let term = null;
  let fitAddon = null;
  let currentSessionID = null;

  const initTerminal = () => {
    if (term || !refs.terminalContainer) return;

    term = new window.Terminal({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
      theme: {
        background: "#0c1810",
        foreground: "#cbd5e1",
        cursor: "#0df259",
        selection: "rgba(13, 242, 89, 0.3)",
      },
    });

    fitAddon = new window.FitAddon.FitAddon();
    term.loadAddon(fitAddon);
    term.open(refs.terminalContainer);
    fitAddon.fit();

    term.onData((data) => {
      if (currentSessionID) {
        bridge.writeTerminal(currentSessionID, data);
      }
    });

    window.addEventListener("resize", () => {
      fitAddon?.fit();
      if (currentSessionID && term) {
        bridge.resizeTerminal(currentSessionID, term.cols, term.rows);
      }
    });

    if (bridge.runtime?.EventsOn) {
      bridge.runtime.EventsOn("terminal:output", (payload) => {
        if (payload.id === currentSessionID) {
          term.write(payload.data);
        }
      });

      bridge.runtime.EventsOn("terminal:exit", (payload) => {
        if (payload.id === currentSessionID) {
          term.write("\r\n[Process completed]\r\n");
          currentSessionID = null;
        }
      });
    }
  };

  const loadShellUser = async () => {
    const project = readSelection().project
    if (!project || !refs.shellUser) {
      return
    }
    try {
      const user = await bridge.getShellUser(project)
      refs.shellUser.value = user || ""
    } catch (_err) {
      refs.shellUser.value = ""
    }
  }

  const saveShellUser = async () => {
    const project = readSelection().project
    if (!project || !refs.shellUser) {
      return
    }
    try {
      await bridge.setShellUser(project, refs.shellUser.value || "")
    } catch (_err) {
      // Keep UI non-blocking for preference persistence.
    }
  }

  const openShell = async () => {
    const { project, service } = readSelection();
    if (!project) {
      onStatus("Select an environment to open shell.");
      return;
    }

    if (!term) initTerminal();

    const shellUser = refs.shellUser?.value || "";
    const shell = refs.shellCommand?.value || "bash";

    try {
      term.reset();
      term.write(`Connecting to ${project} (${service || "default"})...\r\n`);

      const sessionID = await bridge.startTerminal(
        project,
        service,
        shellUser,
        shell,
      );

      if (sessionID.startsWith("error:")) {
        throw new Error(sessionID.replace("error: ", ""));
      }

      currentSessionID = sessionID;
      setTimeout(() => {
        fitAddon?.fit();
        bridge.resizeTerminal(currentSessionID, term.cols, term.rows);
      }, 100);

      onStatus(`Terminal opened: ${sessionID}`);
    } catch (err) {
      const message = `Failed to open terminal: ${err}`;
      onStatus(message);
      onToast(message, "error");
      term?.write(`\r\nError: ${err.message || err}\r\n`);
    }
  };

  const resetShellUsers = async () => {
    try {
      const message = await bridge.resetShellUsers()
      onStatus(message)
      onToast(message, "success")
      await loadShellUser()
    } catch (err) {
      const message = `Failed to reset shell users: ${err}`
      onStatus(message)
      onToast(message, "error")
    }
  }

  return { loadShellUser, saveShellUser, openShell, resetShellUsers }
}

