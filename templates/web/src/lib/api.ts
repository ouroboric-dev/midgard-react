import createClient from "openapi-fetch";
import type { paths } from "@/api/schema";

/**
 * The API client, typed by contracts/api.openapi.yaml. Calls go to /api, which the dev and preview servers
 * forward to the API (see vite.config.ts) and a deployment routes to it, so the browser needs no CORS. The
 * URL is absolute because the Fetch API's Request refuses a relative one outside a browser, in the tests.
 */
export const api = createClient<paths>({
  baseUrl: new URL("/api", window.location.origin).href,
  // openapi-fetch keeps the fetch it finds when the client is made; this one looks it up at each call, so a
  // test can replace it (vi.stubGlobal) after the module is loaded.
  fetch: (request) => globalThis.fetch(request),
});
