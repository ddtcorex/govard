import http from "node:http";

const upstream = new URL(
  process.env.GOVARD_FRONTEND_INJECT_UPSTREAM || "http://127.0.0.1:80",
);
const port = Number.parseInt(
  process.env.GOVARD_FRONTEND_INJECT_PORT || "3000",
  10,
);
const injectedScript = process.env.GOVARD_FRONTEND_INJECT_SCRIPT_HTML || "";

function copyResponseHeaders(headers, bodyChanged) {
  const copied = { ...headers };
  if (bodyChanged) {
    delete copied["content-encoding"];
    delete copied["content-length"];
    delete copied.etag;
  }
  delete copied.connection;
  delete copied["keep-alive"];
  delete copied["proxy-authenticate"];
  delete copied["proxy-authorization"];
  delete copied.te;
  delete copied.trailer;
  delete copied["transfer-encoding"];
  delete copied.upgrade;
  return copied;
}

function injectScript(html) {
  if (!injectedScript || html.includes(injectedScript)) {
    return html;
  }
  const closingBody = /<\/body\s*>/i;
  if (!closingBody.test(html)) {
    return html;
  }
  return html.replace(closingBody, `${injectedScript}</body>`);
}

function isLocalHealthCheck(request) {
  if (request.url !== "/__govard_frontend_health") {
    return false;
  }
  try {
    const hostname = new URL(`http://${request.headers.host}`).hostname;
    return (
      hostname === "127.0.0.1" ||
      hostname === "localhost" ||
      hostname === "[::1]"
    );
  } catch {
    return false;
  }
}

const server = http.createServer((request, response) => {
  if (isLocalHealthCheck(request)) {
    response.writeHead(200, { "content-type": "text/plain" });
    response.end("ok");
    return;
  }

  const headers = { ...request.headers, "accept-encoding": "identity" };
  const upstreamRequest = http.request(
    {
      protocol: upstream.protocol,
      hostname: upstream.hostname,
      port: upstream.port,
      method: request.method,
      path: request.url,
      headers,
    },
    (upstreamResponse) => {
      const contentType = String(
        upstreamResponse.headers["content-type"] || "",
      ).toLowerCase();
      if (!contentType.startsWith("text/html")) {
        response.writeHead(
          upstreamResponse.statusCode || 502,
          copyResponseHeaders(upstreamResponse.headers, false),
        );
        upstreamResponse.pipe(response);
        return;
      }

      const chunks = [];
      upstreamResponse.on("data", (chunk) => chunks.push(chunk));
      upstreamResponse.on("end", () => {
        const original = Buffer.concat(chunks).toString("utf8");
        const injected = injectScript(original);
        response.writeHead(
          upstreamResponse.statusCode || 502,
          copyResponseHeaders(
            upstreamResponse.headers,
            injected !== original,
          ),
        );
        response.end(injected);
      });
    },
  );

  upstreamRequest.on("error", (error) => {
    if (!response.headersSent) {
      response.writeHead(502, { "content-type": "text/plain" });
    }
    response.end(`frontend injection proxy error: ${error.message}`);
  });
  request.pipe(upstreamRequest);
});

server.listen(port, "0.0.0.0");

function shutdown() {
  server.close(() => process.exit(0));
}

process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);
