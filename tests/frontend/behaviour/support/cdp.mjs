// @ts-check
import { spawn } from "node:child_process";
import { createServer } from "node:net";

/**
 * Asks the OS for a free port. Hard-coding a debug port (or picking one at
 * random) races a leaked Chrome from an earlier run and fails the suite
 * intermittently with "Chrome did not open its DevTools port".
 */
export async function findFreePort() {
  return new Promise((resolve, reject) => {
    const srv = createServer();
    srv.listen(0, "127.0.0.1", () => {
      const { port } = srv.address();
      srv.close(() => resolve(port));
    });
    srv.on("error", reject);
  });
}

/**
 * Fails loudly with the real Node floor requirement instead of a bare
 * ReferenceError, per the spec's risk table.
 */
function assertWebSocketAvailable() {
  if (typeof WebSocket !== "function") {
    throw new Error(
      "tests/frontend/behaviour needs Node's built-in WebSocket (Node 24+, per docs/workflows/desktop-app.md); this process is missing it.",
    );
  }
}

/**
 * @param {{chromeBin: string, port: number, userDataDir: string}} opts
 */
export function launchChrome({ chromeBin, port, userDataDir }) {
  const proc = spawn(chromeBin, [
    "--headless=new",
    `--remote-debugging-port=${port}`,
    `--user-data-dir=${userDataDir}`,
    "--no-first-run",
    "--no-default-browser-check",
    "--disable-dev-shm-usage",
  ]);
  return proc;
}

/**
 * Polls Chrome's /json/version endpoint until it answers.
 * @param {number} port
 */
async function waitForChrome(port, timeoutMs = 10000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`http://127.0.0.1:${port}/json/version`);
      if (res.ok) return;
    } catch {
      // not up yet
    }
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error(`Chrome did not open its DevTools port ${port} within ${timeoutMs}ms`);
}

/**
 * Opens a new tab and connects to its DevTools WebSocket. Chrome requires PUT
 * on /json/new since version 111.
 * @param {{port: number, url: string}} opts
 */
export async function openTab({ port, url }) {
  assertWebSocketAvailable();
  await waitForChrome(port);
  const res = await fetch(`http://127.0.0.1:${port}/json/new?${encodeURIComponent(url)}`, {
    method: "PUT",
  });
  const target = await res.json();
  if (!target.webSocketDebuggerUrl) {
    throw new Error(`Chrome opened no debuggable target for ${url}: ${JSON.stringify(target)}`);
  }
  return new Session(target.webSocketDebuggerUrl, target.id, port);
}

export class Session {
  constructor(wsUrl, targetId, port) {
    this.targetId = targetId;
    this.port = port;
    this.nextId = 1;
    this.pending = new Map();
    this.consoleErrors = [];
    this.ws = new WebSocket(wsUrl);
    this.ready = new Promise((resolve, reject) => {
      this.ws.addEventListener("open", () => resolve());
      this.ws.addEventListener("error", reject);
    });
    this.ws.addEventListener("message", (event) => {
      const msg = JSON.parse(event.data);
      if (msg.id && this.pending.has(msg.id)) {
        const { resolve, reject } = this.pending.get(msg.id);
        this.pending.delete(msg.id);
        if (msg.error) reject(new Error(msg.error.message));
        else resolve(msg.result);
      }
      if (msg.method === "Runtime.consoleAPICalled" && msg.params.type === "error") {
        this.consoleErrors.push(msg.params.args.map((a) => a.value ?? a.description).join(" "));
      }
      if (msg.method === "Runtime.exceptionThrown") {
        const details = msg.params.exceptionDetails || {};
        this.consoleErrors.push(details.exception?.description ?? details.text ?? "exception");
      }
    });
  }

  async send(method, params = {}) {
    await this.ready;
    const id = this.nextId++;
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      this.ws.send(JSON.stringify({ id, method, params }));
    });
  }

  async navigate(url) {
    await this.send("Page.enable");
    await this.send("Runtime.enable");
    await this.send("Page.navigate", { url });
    // Wait for load; poll document.readyState rather than trusting a single event.
    const deadline = Date.now() + 10000;
    while (Date.now() < deadline) {
      const state = await this.evaluate("document.readyState");
      if (state === "complete") return;
      await new Promise((r) => setTimeout(r, 100));
    }
    throw new Error(`navigation to ${url} did not reach readyState=complete`);
  }

  /** @param {string} expression */
  async evaluate(expression) {
    const result = await this.send("Runtime.evaluate", {
      expression,
      returnByValue: true,
      awaitPromise: true,
    });
    if (result.exceptionDetails) {
      throw new Error(result.exceptionDetails.exception?.description ?? "Runtime.evaluate threw");
    }
    return result.result.value;
  }

  /**
   * Polls an expression until it returns the wanted value.
   * @param {string} expression
   * @param {any} want
   * @param {{timeoutMs?: number, intervalMs?: number}} [opts]
   */
  async waitFor(expression, want, { timeoutMs = 5000, intervalMs = 100 } = {}) {
    const deadline = Date.now() + timeoutMs;
    let seen;
    while (Date.now() < deadline) {
      seen = await this.evaluate(expression);
      if (seen === want) return seen;
      await new Promise((r) => setTimeout(r, intervalMs));
    }
    throw new Error(`waitFor(${expression}) never became ${JSON.stringify(want)}; last saw ${JSON.stringify(seen)}`);
  }

  /** @param {{x:number,y:number,width:number,height:number}} clip */
  async screenshot(clip) {
    const result = await this.send("Page.captureScreenshot", {
      format: "png",
      clip: { ...clip, scale: 1 },
    });
    return Buffer.from(result.data, "base64");
  }

  async close() {
    await fetch(`http://127.0.0.1:${this.port}/json/close/${this.targetId}`).catch(() => {});
    this.ws.close();
  }
}
