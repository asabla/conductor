/**
 * Login page - displays SSO login button
 */

import { useEffect, useState } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { Loader2, LogIn, AlertCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useAuth } from "@/hooks/use-auth";

interface LoginSearchParams {
  returnTo?: string;
  error?: string;
}

export function LoginPage() {
  const { isAuthenticated, isLoading: authLoading, error: authError, login } = useAuth();
  const navigate = useNavigate();
  const search: LoginSearchParams | undefined = useSearch({ strict: false });
  const searchParams = search ?? {};
  const [isRedirecting, setIsRedirecting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Redirect if already authenticated
  useEffect(() => {
    if (isAuthenticated && !authLoading) {
      const returnTo = searchParams.returnTo || "/";
      void navigate({ to: returnTo });
    }
  }, [isAuthenticated, authLoading, navigate, searchParams.returnTo]);

  // Set error from URL params or auth context
  useEffect(() => {
    if (searchParams.error) {
      setError(decodeURIComponent(searchParams.error));
    } else if (authError) {
      setError(authError);
    }
  }, [searchParams.error, authError]);

  const handleLogin = () => {
    setIsRedirecting(true);
    setError(null);

    login(searchParams.returnTo).catch((err: unknown) => {
      setIsRedirecting(false);
      setError(err instanceof Error ? err.message : "Failed to start login");
    });
  };

  // Show loading while checking auth state
  if (authLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <div className="flex flex-col items-center gap-4">
          <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
          <p className="text-sm text-muted-foreground">Loading...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-4">
      <Card className="w-full max-w-md">
        <CardHeader className="space-y-4 text-center">
          {/* Logo */}
          <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-primary/10">
            <svg
              viewBox="0 0 24 24"
              className="h-10 w-10 text-primary"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
            >
              <path d="M12 2L2 7l10 5 10-5-10-5z" />
              <path d="M2 17l10 5 10-5" />
              <path d="M2 12l10 5 10-5" />
            </svg>
          </div>

          <div>
            <CardTitle className="text-2xl font-bold">Conductor</CardTitle>
            <CardDescription className="mt-2">
              Distributed Test Orchestration Platform
            </CardDescription>
          </div>
        </CardHeader>

        <CardContent className="space-y-6">
          {/* Error message */}
          {error && (
            <div className="flex items-start gap-3 rounded-lg bg-destructive/10 p-4 text-sm text-destructive">
              <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" />
              <div>
                <p className="font-medium">Authentication failed</p>
                <p className="mt-1 text-destructive/80">{error}</p>
              </div>
            </div>
          )}

          {/* Login button */}
          <Button
            onClick={handleLogin}
            disabled={isRedirecting}
            className="w-full"
            size="lg"
          >
            {isRedirecting ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Redirecting...
              </>
            ) : (
              <>
                <LogIn className="mr-2 h-4 w-4" />
                Sign in with SSO
              </>
            )}
          </Button>

          {/* Help text */}
          <p className="text-center text-xs text-muted-foreground">
            You will be redirected to your organization&apos;s identity provider to sign
            in.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
