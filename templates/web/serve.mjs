// The web leaf's own server (node:http and node:fs only, no package). It serves the built bundle (DIST, default
// "dist") on HOST:PORT, and forwards two prefixes:
//   /api/...        to API_URL without the "/api" prefix, with X-Forwarded-Host and X-Forwarded-Proto set;
//   /<S3_BUCKET>/.. to S3_ENDPOINT with its method, path, query, body and Host unchanged and no X-Forwarded-*
//                   header, so that a presigned signature still matches the address the browser used.
// Any other path answers a file of the bundle, or index.html when it names no file. It reads no secret.
import fs from "node:fs";
import http from "node:http";
import https from "node:https";
import path from "node:path";
import { pipeline } from "node:stream";

const port = Number(process.env.PORT);
const host = process.env.HOST || "127.0.0.1";
const apiUrl = new URL(process.env.API_URL || "http://localhost:8080");
const storeUrl = process.env.S3_ENDPOINT ? new URL(process.env.S3_ENDPOINT) : null;
const storePrefix = process.env.S3_BUCKET ? `/${process.env.S3_BUCKET}/` : null;
const dist = path.resolve(process.env.DIST || "dist");

if (!Number.isInteger(port) || port <= 0) {
  console.error("serve.mjs: PORT must be a number");
  process.exit(1);
}

const types = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".mjs": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".map": "application/json; charset=utf-8",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".jpg": "image/jpeg",
  ".jpeg": "image/jpeg",
  ".gif": "image/gif",
  ".webp": "image/webp",
  ".ico": "image/x-icon",
  ".txt": "text/plain; charset=utf-8",
  ".woff": "font/woff",
  ".woff2": "font/woff2",
  ".wasm": "application/wasm",
};

// forward sends req to target as path, with headers, and streams the answer back.
function forward(req, res, target, urlPath, headers) {
  const secure = target.protocol === "https:";
  const upstream = (secure ? https : http).request(
    {
      protocol: target.protocol,
      hostname: target.hostname,
      port: target.port || (secure ? 443 : 80),
      method: req.method,
      path: urlPath,
      headers,
    },
    (answer) => {
      res.writeHead(answer.statusCode, answer.statusMessage, answer.headers);
      pipeline(answer, res, () => {});
    },
  );
  upstream.on("error", (err) => {
    if (res.headersSent) {
      res.destroy();
      return;
    }
    res.writeHead(502, { "content-type": "text/plain; charset=utf-8" });
    res.end(`serve.mjs: ${target.origin} did not answer: ${err.code || err.message}\n`);
  });
  res.on("close", () => upstream.destroy());
  pipeline(req, upstream, () => {});
}

function hopFree(headers) {
  const out = { ...headers };
  for (const h of ["connection", "keep-alive", "proxy-connection", "transfer-encoding", "upgrade", "te", "trailer"]) {
    // A body keeps its framing: node adds chunked encoding itself when no content-length is set.
    delete out[h];
  }
  return out;
}

function sendFile(req, res, file) {
  const type = types[path.extname(file).toLowerCase()] || "application/octet-stream";
  const cache = path.basename(file) === "index.html" ? "no-cache" : "public, max-age=3600";
  const stream = fs.createReadStream(file);
  stream.once("open", () => {
    res.writeHead(200, { "content-type": type, "cache-control": cache });
    if (req.method === "HEAD") {
      stream.destroy();
      res.end();
      return;
    }
    pipeline(stream, res, () => {});
  });
  stream.once("error", () => {
    res.writeHead(404, { "content-type": "text/plain; charset=utf-8" });
    res.end("not found\n");
  });
}

function serveBundle(req, res, pathname) {
  if (req.method !== "GET" && req.method !== "HEAD") {
    res.writeHead(405, { allow: "GET, HEAD", "content-type": "text/plain; charset=utf-8" });
    res.end("method not allowed\n");
    return;
  }
  let decoded;
  try {
    decoded = decodeURIComponent(pathname);
  } catch {
    decoded = null;
  }
  if (decoded === null || decoded.includes("\0")) {
    res.writeHead(400, { "content-type": "text/plain; charset=utf-8" });
    res.end("bad path\n");
    return;
  }
  // Normalising a rooted path first removes every ".." that would climb above the root.
  const file = path.join(dist, path.posix.normalize(`/${decoded}`));
  fs.stat(file, (err, info) => {
    sendFile(req, res, !err && info.isFile() && file.startsWith(dist + path.sep) ? file : path.join(dist, "index.html"));
  });
}

const server = http.createServer((req, res) => {
  const url = req.url || "/";
  const pathname = url.split("?")[0];

  if (storeUrl && storePrefix && pathname.startsWith(storePrefix)) {
    // Path, query, body and Host as the browser sent them; nothing added.
    forward(req, res, storeUrl, url, hopFree(req.headers));
    return;
  }
  if (pathname === "/api" || pathname.startsWith("/api/")) {
    const headers = hopFree(req.headers);
    delete headers.host;
    headers["x-forwarded-host"] = req.headers.host || "";
    headers["x-forwarded-proto"] = "http";
    forward(req, res, apiUrl, `/${url.slice("/api".length).replace(/^\//, "")}`, headers);
    return;
  }
  serveBundle(req, res, pathname);
});

server.listen(port, host, () => {
  console.log(`serve.mjs: serving ${dist} on ${host}:${port}`);
});
for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => process.exit(0));
}
