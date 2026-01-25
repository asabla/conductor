/**
 * Protected route component - guards routes that require authentication
 */

import { type ReactNode, useEffect } from "react";
import { useLocation, useNavigate } from "@tanstack/react-router";
import { Loader2 } from "lucide-react";
import { useAuth, useHasAnyRole } from "@/hooks/use-auth";
import type { UserRole } from "@/types/auth";

interface ProtectedRouteProps {
  children: ReactNode;
  /** Required roles to access this route (user must have at least one) */
  requiredRoles?: UserRole[];
  /** Custom fallback component while loading */
  fallback?: ReactNode;
}

/**
 * Route guard that redirects to login if not authenticated
 * Optionally checks for required roles
 */
export function ProtectedRoute({
  children,
  requiredRoles,
  fallback,
}: ProtectedRouteProps) {
  const { isAuthenticated, isLoading, user } = useAuth();
  const location = useLocation();
  const navigate = useNavigate();
  const hasRequiredRole = useHasAnyRole(requiredRoles || ["viewer"]);

  // Redirect to login if not authenticated
  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      const searchStr = typeof location.search === "string" ? location.search : "";
      const returnTo = encodeURIComponent(location.pathname + searchStr);
      void navigate({
        to: "/login",
        search: { returnTo },
      });
    }
  }, [isAuthenticated, isLoading, location.pathname, location.search, navigate]);

  // Show loading state while checking authentication
  if (isLoading) {
    return (
      fallback || (
        <div className="flex min-h-screen items-center justify-center">
          <div className="flex flex-col items-center gap-4">
            <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
            <p className="text-sm text-muted-foreground">Loading...</p>
          </div>
        </div>
      )
    );
  }

  // Show loading while redirecting to login
  if (!isAuthenticated) {
    return (
      fallback || (
        <div className="flex min-h-screen items-center justify-center">
          <div className="flex flex-col items-center gap-4">
            <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
            <p className="text-sm text-muted-foreground">Redirecting to login...</p>
          </div>
        </div>
      )
    );
  }

  // Check role-based access
  if (requiredRoles && requiredRoles.length > 0 && !hasRequiredRole) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold text-destructive">Access Denied</h1>
          <p className="mt-2 text-muted-foreground">
            You don&apos;t have permission to access this page.
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            Required role: {requiredRoles.join(" or ")}
          </p>
          {user && (
            <p className="mt-1 text-sm text-muted-foreground">
              Your roles: {user.roles.join(", ")}
            </p>
          )}
        </div>
      </div>
    );
  }

  return <>{children}</>;
}

/**
 * Higher-order component to wrap a component with ProtectedRoute
 */
export function withAuth<P extends object>(
  Component: React.ComponentType<P>,
  requiredRoles?: UserRole[]
) {
  function WrappedComponent(props: P) {
    return (
      <ProtectedRoute requiredRoles={requiredRoles}>
        <Component {...props} />
      </ProtectedRoute>
    );
  }
  WrappedComponent.displayName = `withAuth(${Component.displayName || Component.name || "Component"})`;
  return WrappedComponent;
}
