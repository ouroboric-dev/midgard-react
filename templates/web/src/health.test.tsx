import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { HealthBadge } from "@/components/health-badge";

function renderBadge() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <HealthBadge />
    </QueryClientProvider>,
  );
}

function answer(status: number, body: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } })),
  );
}

describe("HealthBadge", () => {
  it("says the API is ok when /healthz says ok", async () => {
    answer(200, { status: "ok" });
    renderBadge();
    expect(await screen.findByText("API: ok")).toBeInTheDocument();
  });

  it("says the API is down when the database is unavailable", async () => {
    answer(503, { status: "database unavailable" });
    renderBadge();
    expect(await screen.findByText("API: database unavailable")).toBeInTheDocument();
  });

  it("says the API cannot be reached when the request fails", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => Promise.reject(new TypeError("network"))));
    renderBadge();
    expect(await screen.findByText("API: not reachable")).toBeInTheDocument();
  });
});
