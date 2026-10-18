import path from "node:path";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The API is at API_URL (the mock in a task's loop, the real server in the acceptance run). The browser
// calls it through /api so it never needs CORS, and the dev and preview servers forward the calls.
const api = process.env.API_URL ?? "http://localhost:8080";
const proxy = { "/api": { target: api, changeOrigin: true, rewrite: (p: string) => p.replace(/^\/api/, "") } };

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(import.meta.dirname, "src") } },
  server: { proxy },
  preview: { proxy },
  test: { environment: "jsdom", setupFiles: ["./src/test-setup.ts"], include: ["src/**/*.test.{ts,tsx}"] },
});
