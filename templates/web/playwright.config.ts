import { defineConfig } from "@playwright/test";

// The e2e run needs the app at E2E_URL (default: the Vite preview of a build, on 4173) and an API behind
// it: a mock of contracts/api.openapi.yaml in a task's loop (web-e2e.sh), the real server in the
// acceptance run (accept-e2e.sh). One spec per acceptance criterion, in e2e/.
export default defineConfig({
  testDir: "e2e",
  fullyParallel: true,
  reporter: [["list"]],
  use: { baseURL: process.env.E2E_URL ?? "http://localhost:4173", trace: "off" },
  projects: [{ name: "chromium", use: { browserName: "chromium" } }],
});
