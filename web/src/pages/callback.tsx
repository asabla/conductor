/**
 * OIDC callback handler page
 */

import { useEffect, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { Loader2, AlertCircle, CheckCircle } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { useAuth } from "@/hooks/use-auth";

type CallbackState = "processing" | "success" | "error";

export function CallbackPage() {
  const { handleCallback, isLoading: authLoading } = useAuth();
  const navigate = useNavigate();
  const [state, setState] = useState<CallbackState>("processing");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    // Don't process if auth is still loading
    if (authLoading) {
      return;
    }

    let cancelled = false;

    async function processCallback() {
      try {
        const returnTo = await handleCallback();
        if (cancelled) return;
        setState("success");

        // Brief delay to show success state, then redirect
        setTimeout(() => {
          if (!cancelled) {
            void navigate({ to: returnTo });
          }
        }, 500);
      } catch (err) {
        if (cancelled) return;
        console.error("Callback error:", err);
        setState("error");
        setError(err instanceof Error ? err.message : "Authentication failed");
      }
    }

    void processCallback();

    return () => {
      cancelled = true;
    };
  }, [authLoading, handleCallback, navigate]);

  const handleRetry = () => {
    void navigate({ to: "/login" });
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-4">
      <Card className="w-full max-w-md">
        <CardHeader className="text-center">
          <CardTitle className="text-xl">
            {state === "processing" && "Signing you in..."}
            {state === "success" && "Welcome back!"}
            {state === "error" && "Authentication failed"}
          </CardTitle>
          <CardDescription>
            {state === "processing" && "Please wait while we complete the authentication."}
            {state === "success" && "Redirecting to your destination..."}
            {state === "error" && "There was a problem signing you in."}
          </CardDescription>
        </CardHeader>

        <CardContent className="flex flex-col items-center gap-4">
          {state === "processing" && (
            <Loader2 className="h-12 w-12 animate-spin text-primary" />
          )}

          {state === "success" && (
            <CheckCircle className="h-12 w-12 text-green-500" />
          )}

          {state === "error" && (
            <>
              <AlertCircle className="h-12 w-12 text-destructive" />
              {error && (
                <div className="w-full rounded-lg bg-destructive/10 p-4 text-sm text-destructive">
                  <p className="text-center">{error}</p>
                </div>
              )}
              <Button onClick={handleRetry} className="mt-2">
                Try again
              </Button>
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
