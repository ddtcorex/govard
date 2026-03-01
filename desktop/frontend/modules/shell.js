export const createShellController = ({
  bridge,
  refs,
  readSelection,
  onStatus,
  onToast,
}) => {
  const updateRefs = (newRefs) => {
    refs = newRefs;
  };
  let term = null;
  let fitAddon = null;
  let currentSessionID = null;

  const initTerminal = () => {
    if (term || !refs.terminalContainer) return;

    term = new window.Terminal({
      cursorBlink: true,
      fontSize: 13,
      lineHeight: 1.25,
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
    requestAnimationFrame(() => fitAddon?.fit());

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
    const project = readSelection().project;
    if (!project || !refs.shellUser) {
      return;
    }
    try {
      const user = await bridge.getShellUser(project);
      refs.shellUser.value = user || "";
    } catch (_err) {
      refs.shellUser.value = "";
    }
  };

  const saveShellUser = async () => {
    const project = readSelection().project;
    if (!project || !refs.shellUser) {
      return;
    }
    try {
      await bridge.setShellUser(project, refs.shellUser.value || "");
    } catch (_err) {
      // Keep UI non-blocking for preference persistence.
    }
  };

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

      onStatus("Connected to environment.");
    } catch (err) {
      const message = "Failed to connect to environment.";
      onStatus(message);
      onToast(message, "error");
      term?.write(`\r\nError: ${err.message || err}\r\n`);
    }
  };

  const resetShellUsers = async () => {
    try {
      const message = await bridge.resetShellUsers();
      onStatus("Shell user preferences reset.");
      onToast("Shell user preferences reset.", "success");
      await loadShellUser();
    } catch (err) {
      const message = "Could not reset shell users.";
      onStatus(message);
      onToast(message, "error");
    }
  };

  const openRemoteShell = async (remoteName) => {
    console.log("[Shell] openRemoteShell", remoteName);
    const project = readSelection().project;
    console.log("[Shell] project:", project);
    if (!project) return;
    if (!term) initTerminal();

    try {
      term.reset();
      term.write(`Opening shell for remote: ${remoteName}...\r\n`);
      const sessionID = await bridge.startGovardTerminal(project, [
        "remote",
        "exec",
        remoteName,
        "--",
        "bash",
      ]);
      if (sessionID.startsWith("error:"))
        throw new Error(sessionID.replace("error: ", ""));
      currentSessionID = sessionID;
      setTimeout(() => {
        fitAddon?.fit();
        bridge.resizeTerminal(currentSessionID, term.cols, term.rows);
      }, 100);
      onStatus(`Connected to remote: ${remoteName}`);
    } catch (err) {
      onStatus(`Failed to connect to ${remoteName}.`);
      onToast(`Error connecting: ${err}`, "error");
      term?.write(`\r\nError: ${err.message || err}\r\n`);
    }
  };

  const openRemoteDB = async (remoteName) => {
    console.log("[Shell] openRemoteDB", remoteName);
    const project = readSelection().project;
    console.log("[Shell] project:", project);
    if (!project) return;
    if (!term) initTerminal();

    try {
      term.reset();
      term.write(`Connecting to remote database: ${remoteName}...\r\n`);
      const sessionID = await bridge.startGovardTerminal(project, [
        "open",
        "db",
        "-e",
        remoteName,
        "--client",
      ]);
      if (sessionID.startsWith("error:"))
        throw new Error(sessionID.replace("error: ", ""));
      currentSessionID = sessionID;
      setTimeout(() => {
        fitAddon?.fit();
        bridge.resizeTerminal(currentSessionID, term.cols, term.rows);
      }, 100);
      onStatus(`Opened DB client for remote: ${remoteName}`);
    } catch (err) {
      onStatus(`Failed to open remote db for ${remoteName}.`);
      onToast(`Error opening DB: ${err}`, "error");
      term?.write(`\r\nError: ${err.message || err}\r\n`);
    }
  };

  const openRemoteSFTP = async (remoteName) => {
    console.log("[Shell] openRemoteSFTP", remoteName);
    const project = readSelection().project;
    console.log("[Shell] project:", project);
    if (!project) return;
    if (!term) initTerminal();

    try {
      term.reset();
      term.write(`Connecting to remote SFTP: ${remoteName}...\r\n`);
      const sessionID = await bridge.startGovardTerminal(project, [
        "open",
        "sftp",
        "-e",
        remoteName,
      ]);
      if (sessionID.startsWith("error:"))
        throw new Error(sessionID.replace("error: ", ""));
      currentSessionID = sessionID;
      setTimeout(() => {
        fitAddon?.fit();
        bridge.resizeTerminal(currentSessionID, term.cols, term.rows);
      }, 100);
      onStatus(`Opened SFTP for remote: ${remoteName}`);
    } catch (err) {
      onStatus(`Failed to open remote SFTP for ${remoteName}.`);
      onToast(`Error opening SFTP: ${err}`, "error");
      term?.write(`\r\nError: ${err.message || err}\r\n`);
    }
  };

  return {
    updateRefs,
    loadShellUser,
    saveShellUser,
    openShell,
    resetShellUsers,
    openRemoteShell,
    openRemoteDB,
    openRemoteSFTP,
  };
};
