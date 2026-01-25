import { Outlet } from "@tanstack/react-router";
import { AppShell } from "./components/layout/app-shell";
import { ProtectedRoute } from "./components/auth/protected-route";

export function App() {
  return (
    <ProtectedRoute>
      <AppShell>
        <Outlet />
      </AppShell>
    </ProtectedRoute>
  );
}
