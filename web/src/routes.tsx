import { createRootRouteWithContext, createRoute, Outlet } from "@tanstack/react-router";
import type { QueryClient } from "@tanstack/react-query";
import { App } from "./App";
import { DashboardPage } from "./pages/dashboard";
import { TestRunsPage } from "./pages/test-runs";
import { RunDetailsPage } from "./pages/run-details";
import { AgentsPage } from "./pages/agents";
import { ServicesPage } from "./pages/services";
import { ServiceDetailsPage } from "./pages/service-details";
import { SettingsPage } from "./pages/settings";
import { FailureAnalysisPage } from "./pages/failure-analysis";
import { FlakyTestsPage } from "./pages/flaky-tests";
import { TrendsPage } from "./pages/trends";
import { LoginPage } from "./pages/login";
import { CallbackPage } from "./pages/callback";
import { LogoutPage } from "./pages/logout";

interface RouterContext {
  queryClient: QueryClient;
}

// Root route - just renders outlet
const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: () => <Outlet />,
});

// Auth routes (unprotected) - rendered without AppShell
const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  component: LoginPage,
});

const callbackRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/callback",
  component: CallbackPage,
});

const logoutRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/logout",
  component: LogoutPage,
});

// Protected layout route - wraps protected pages with App (includes AppShell + ProtectedRoute)
const appLayoutRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: "app",
  component: App,
});

// Protected routes - nested under appLayoutRoute
const indexRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: "/",
  component: DashboardPage,
});

const testRunsRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: "/test-runs",
  component: TestRunsPage,
});

const runDetailsRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: "/test-runs/$runId",
  component: RunDetailsPage,
});

const agentsRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: "/agents",
  component: AgentsPage,
});

const servicesRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: "/services",
  component: ServicesPage,
});

const serviceDetailsRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: "/services/$serviceId",
  component: ServiceDetailsPage,
});

const settingsRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: "/settings",
  component: SettingsPage,
});

const failureAnalysisRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: "/failure-analysis",
  component: FailureAnalysisPage,
});

const flakyTestsRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: "/flaky-tests",
  component: FlakyTestsPage,
});

const trendsRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: "/trends",
  component: TrendsPage,
});

export const routeTree = rootRoute.addChildren([
  // Auth routes (unprotected)
  loginRoute,
  callbackRoute,
  logoutRoute,
  // Protected routes (wrapped with App)
  appLayoutRoute.addChildren([
    indexRoute,
    testRunsRoute,
    runDetailsRoute,
    agentsRoute,
    servicesRoute,
    serviceDetailsRoute,
    settingsRoute,
    failureAnalysisRoute,
    flakyTestsRoute,
    trendsRoute,
  ]),
]);
