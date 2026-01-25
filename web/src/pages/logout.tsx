/**
 * Logout page - clears session and redirects to IdP logout
 */

import { useEffect, useState } from "react";
import { Loader2, LogOut, CheckCircle } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useAuth } from "@/hooks/use-auth";

export function LogoutPage() {
  const { logout, isAuthenticated } = useAuth();
  const [isLoggingOut, setIsLoggingOut] = useState(false);
  const [isComplete, setIsComplete] = useState(false);

  useEffect(() => {
    if (isLoggingOut || isComplete) {
      return;
    }

    // If not authenticated, redirect to login
    if (!isAuthenticated) {
      window.location.href = "/login";
      return;
    }

    setIsLoggingOut(true);

    // Short delay to show the message, then perform logout
    const timer = setTimeout(() => {
      setIsComplete(true);
      logout().catch((error) => {
        console.error("Logout error:", error);
        // Even on error, redirect to login
        window.location.href = "/login";
      });
    }, 500);

    return () => {
      clearTimeout(timer);
    };
  }, [isAuthenticated, logout, isLoggingOut, isComplete]);

  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-4">
      <Card className="w-full max-w-md">
        <CardHeader className="text-center">
          <div className="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-muted">
            {isComplete ? (
              <CheckCircle className="h-8 w-8 text-green-500" />
            ) : (
              <LogOut className="h-8 w-8 text-muted-foreground" />
            )}
          </div>
          <CardTitle className="text-xl">
            {isComplete ? "Signed out" : "Signing out..."}
          </CardTitle>
          <CardDescription>
            {isComplete
              ? "You have been signed out successfully."
              : "Please wait while we sign you out."}
          </CardDescription>
        </CardHeader>

        <CardContent className="flex justify-center">
          {!isComplete && <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />}
        </CardContent>
      </Card>
    </div>
  );
}
