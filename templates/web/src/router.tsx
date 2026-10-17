import { createRootRoute, createRoute, createRouter, Outlet } from "@tanstack/react-router";
import { Home } from "@/routes";

const root = createRootRoute({ component: () => <Outlet /> });
const index = createRoute({ getParentRoute: () => root, path: "/", component: Home });

/** The routes of the app. A page is a route here and a component under src/routes. */
export const router = createRouter({ routeTree: root.addChildren([index]) });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
