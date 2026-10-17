import { HealthBadge } from "@/components/health-badge";
import { Button } from "@/components/ui/button";

export function Home() {
  return (
    <main className="mx-auto max-w-3xl p-8">
      <h1 className="text-2xl font-semibold">Gallery</h1>
      <HealthBadge />
      <Button className="mt-4" variant="outline" onClick={() => window.location.reload()}>
        Reload
      </Button>
    </main>
  );
}
