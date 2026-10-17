import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";

/** What /healthz says, as a short sentence. */
async function readHealth(): Promise<string> {
  const { data, error, response } = await api.GET("/healthz");
  if (data) return data.status;
  // 503 is an answer: the server is up and its database is not.
  if (error && response.status === 503) return error.status;
  throw new Error(`unexpected answer ${response.status}`);
}

export function HealthBadge() {
  const health = useQuery({ queryKey: ["health"], queryFn: readHealth, refetchInterval: 30_000 });
  const text = health.isPending ? "checking" : health.isError ? "not reachable" : health.data;
  return <p className="text-sm text-neutral-600">API: {text}</p>;
}
