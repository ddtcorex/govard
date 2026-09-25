// @ts-check
import { spawn } from "node:child_process";
import { createServer } from "node:net";
import { join } from "node:path";

async function findFreePort() {
  return new Promise((resolve, reject) => {
    const srv = createServer();
    srv.listen(0, () => {
      const { port } = srv.address();
      srv.close(() => resolve(port));
    });
    srv.on("error", reject);
  });
}

/**
 * Starts a throwaway Vite dev server for the behaviour suite, on its own
 * ephemeral port so it never collides with a developer's own `pnpm dev`.
 *
 * The local vite binary is spawned directly rather than through
 * `pnpm exec vite`: `pnpm` would be an extra process between the test and vite,
 * so killing the child would leave vite holding the port.
 * @param {{cwd: string}} opts
 */
export async function startVite({ cwd }) {
  const port = await findFreePort();
  const proc = spawn(join(cwd, "node_modules", ".bin", "vite"), [
    "--port",
    String(port),
    "--strictPort",
  ], { cwd });
  const baseUrl = `http://127.0.0.1:${port}`;
  const deadline = Date.now() + 15000;
  let up = false;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`${baseUrl}/preview.html`);
      if (res.ok) {
        up = true;
        break;
      }
    } catch {
      // not up yet
    }
    await new Promise((r) => setTimeout(r, 200));
  }
  if (!up) {
    proc.kill();
    throw new Error(`vite did not serve ${baseUrl}/preview.html within 15s`);
  }
  return {
    baseUrl,
    async stop() {
      proc.kill();
    },
  };
}
