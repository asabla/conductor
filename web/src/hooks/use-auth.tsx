/**
 * Authentication context and hooks
 */

import {
  createContext,
  useContext,
  useEffect,
  useState,
  useCallback,
  useRef,
  type ReactNode,
} from "react";
import type { AuthState, AuthConfig, StoredTokens } from "@/types/auth";
import type { User } from "@/types/auth";
import {
  getAuthConfig,
  TokenStorage,
  getCurrentUser,
  isAccessTokenExpired,
  buildAuthorizationUrl,
  exchangeCodeForTokens,
  refreshAccessToken,
  buildLogoutUrl,
} from "@/lib/auth";

interface AuthContextValue extends AuthState {
  login: (returnTo?: string) => Promise<void>;
  logout: () => Promise<void>;
  handleCallback: () => Promise<string>;
  refreshToken: () => Promise<void>;
  getAccessToken: () => string | null;
}

const AuthContext = createContext<AuthContextValue | null>(null);

const AUTH_DISABLED =
  import.meta.env.DEV &&
  (import.meta.env.VITE_AUTH_DISABLED as string | undefined) !== "false";
const DEV_USER: User = {
  id: "dev-user",
  email: "dev@conductor.local",
  name: "Dev User",
  roles: ["admin"],
};

interface AuthProviderProps {
  children: ReactNode;
}

/**
 * Auth provider component
 */
export function AuthProvider({ children }: AuthProviderProps) {
  const [state, setState] = useState<AuthState>({
    user: null,
    isAuthenticated: false,
    isLoading: true,
    error: null,
  });

  const [config, setConfig] = useState<AuthConfig | null>(null);
  const refreshTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const isRefreshingRef = useRef(false);

  const authDisabled = AUTH_DISABLED;
  const devUser = DEV_USER;

  /**
   * Schedule token refresh before expiration
   */
  const scheduleTokenRefresh = useCallback(
    (tokens: StoredTokens, _authConfig: AuthConfig, performRefresh: () => void) => {
      if (refreshTimeoutRef.current) {
        clearTimeout(refreshTimeoutRef.current);
      }

      if (!tokens.refreshToken) {
        return;
      }

      // Refresh 60 seconds before expiration
      const refreshTime = tokens.expiresAt - Date.now() - 60000;

      if (refreshTime <= 0) {
        // Token about to expire, refresh now
        performRefresh();
        return;
      }

      refreshTimeoutRef.current = setTimeout(() => {
        performRefresh();
      }, refreshTime);
    },
    []
  );

  /**
   * Perform token refresh
   */
  const performTokenRefresh = useCallback(
    async (authConfig: AuthConfig) => {
      if (isRefreshingRef.current) {
        return;
      }

      isRefreshingRef.current = true;

      try {
        const tokens = TokenStorage.getTokens();
        if (!tokens?.refreshToken) {
          throw new Error("No refresh token available");
        }

        const newTokens = await refreshAccessToken(authConfig, tokens.refreshToken);
        TokenStorage.setTokens(newTokens);

        const user = getCurrentUser(newTokens);
        setState((prev) => ({
          ...prev,
          user,
          isAuthenticated: !!user,
          error: null,
        }));

        // Schedule next refresh
        scheduleTokenRefresh(newTokens, authConfig, () => {
          void performTokenRefresh(authConfig);
        });
      } catch (error) {
        console.error("Token refresh failed:", error);

        // Clear state and redirect to login
        TokenStorage.clearTokens();
        setState({
          user: null,
          isAuthenticated: false,
          isLoading: false,
          error: "Session expired",
        });
      } finally {
        isRefreshingRef.current = false;
      }
    },
    [scheduleTokenRefresh]
  );

  /**
   * Initialize auth state from stored tokens
   */
  useEffect(() => {
    let mounted = true;

    async function initAuth() {
      try {
        if (authDisabled) {
          setState({
            user: devUser,
            isAuthenticated: true,
            isLoading: false,
            error: null,
          });
          return;
        }
        // Load auth config
        const authConfig = await getAuthConfig();
        if (!mounted) return;
        setConfig(authConfig);

        // Check for existing tokens
        const tokens = TokenStorage.getTokens();

        if (tokens && !isAccessTokenExpired(tokens)) {
          const user = getCurrentUser(tokens);
          setState({
            user,
            isAuthenticated: !!user,
            isLoading: false,
            error: null,
          });

          // Schedule token refresh
          scheduleTokenRefresh(tokens, authConfig, () => {
            void performTokenRefresh(authConfig);
          });
        } else if (tokens?.refreshToken) {
          // Try to refresh expired token
          try {
            const newTokens = await refreshAccessToken(authConfig, tokens.refreshToken);
            TokenStorage.setTokens(newTokens);

            const user = getCurrentUser(newTokens);
            if (!mounted) return;
            setState({
              user,
              isAuthenticated: !!user,
              isLoading: false,
              error: null,
            });

            scheduleTokenRefresh(newTokens, authConfig, () => {
              void performTokenRefresh(authConfig);
            });
          } catch {
            // Refresh failed, clear tokens
            TokenStorage.clearTokens();
            if (!mounted) return;
            setState({
              user: null,
              isAuthenticated: false,
              isLoading: false,
              error: null,
            });
          }
        } else {
          // No valid tokens
          TokenStorage.clearTokens();
          if (!mounted) return;
          setState({
            user: null,
            isAuthenticated: false,
            isLoading: false,
            error: null,
          });
        }
      } catch (error) {
        console.error("Failed to initialize auth:", error);
        if (!mounted) return;
        setState({
          user: null,
          isAuthenticated: false,
          isLoading: false,
          error: error instanceof Error ? error.message : "Failed to initialize auth",
        });
      }
    }

    void initAuth();

    // Cleanup refresh timeout on unmount
    return () => {
      mounted = false;
      if (refreshTimeoutRef.current) {
        clearTimeout(refreshTimeoutRef.current);
      }
    };
  }, [scheduleTokenRefresh, performTokenRefresh, authDisabled, devUser]);

  /**
   * Redirect to OIDC provider for login
   */
  const login = useCallback(
    async (returnTo?: string) => {
      if (authDisabled) {
        setState({
          user: devUser,
          isAuthenticated: true,
          isLoading: false,
          error: null,
        });
        return;
      }
      if (!config) {
        throw new Error("Auth config not loaded");
      }

      setState((prev) => ({ ...prev, isLoading: true, error: null }));

      try {
        const authUrl = await buildAuthorizationUrl(config, returnTo);
        window.location.href = authUrl;
      } catch (error) {
        setState((prev) => ({
          ...prev,
          isLoading: false,
          error: error instanceof Error ? error.message : "Failed to start login",
        }));
        throw error;
      }
    },
    [config, authDisabled, devUser]
  );

  /**
   * Handle OIDC callback - exchange code for tokens
   */
  const handleCallback = useCallback(async (): Promise<string> => {
    if (authDisabled) {
      setState({
        user: devUser,
        isAuthenticated: true,
        isLoading: false,
        error: null,
      });
      return "/";
    }
    if (!config) {
      throw new Error("Auth config not loaded");
    }

    const params = new URLSearchParams(window.location.search);
    const code = params.get("code");
    const returnedState = params.get("state");
    const error = params.get("error");
    const errorDescription = params.get("error_description");

    // Check for errors from IdP
    if (error) {
      const errorMessage = errorDescription || error;
      setState((prev) => ({
        ...prev,
        isLoading: false,
        error: errorMessage,
      }));
      throw new Error(errorMessage);
    }

    // Validate state parameter
    const authState = TokenStorage.getAuthState();
    if (!authState || authState.state !== returnedState) {
      const errorMessage = "Invalid state parameter - possible CSRF attack";
      setState((prev) => ({
        ...prev,
        isLoading: false,
        error: errorMessage,
      }));
      throw new Error(errorMessage);
    }

    if (!code) {
      const errorMessage = "No authorization code received";
      setState((prev) => ({
        ...prev,
        isLoading: false,
        error: errorMessage,
      }));
      throw new Error(errorMessage);
    }

    try {
      // Exchange code for tokens
      const tokens = await exchangeCodeForTokens(config, code, authState.codeVerifier);
      TokenStorage.setTokens(tokens);

      // Update auth state
      const user = getCurrentUser(tokens);
      setState({
        user,
        isAuthenticated: !!user,
        isLoading: false,
        error: null,
      });

      // Schedule token refresh
      scheduleTokenRefresh(tokens, config, () => {
        void performTokenRefresh(config);
      });

      // Return the original destination
      return authState.returnTo || "/";
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : "Token exchange failed";
      setState((prev) => ({
        ...prev,
        isLoading: false,
        error: errorMessage,
      }));
      throw error;
    }
  }, [config, scheduleTokenRefresh, performTokenRefresh, authDisabled, devUser]);

  /**
   * Logout - clear tokens and redirect to IdP logout
   */
  const logout = useCallback(() => {
    if (authDisabled) {
      TokenStorage.clearTokens();
      setState({
        user: null,
        isAuthenticated: false,
        isLoading: false,
        error: null,
      });
      return Promise.resolve();
    }
    if (!config) {
      // Clear local state even without config
      TokenStorage.clearTokens();
      setState({
        user: null,
        isAuthenticated: false,
        isLoading: false,
        error: null,
      });
      window.location.href = "/login";
      return Promise.resolve();
    }

    // Clear refresh timeout
    if (refreshTimeoutRef.current) {
      clearTimeout(refreshTimeoutRef.current);
    }

    // Get ID token for logout hint
    const tokens = TokenStorage.getTokens();
    const idToken = tokens?.idToken;

    // Clear local tokens
    TokenStorage.clearTokens();
    setState({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      error: null,
    });

    // Redirect to IdP logout endpoint
    const logoutUrl = buildLogoutUrl(config, idToken);
    window.location.href = logoutUrl;
    return Promise.resolve();
  }, [config, authDisabled]);

  /**
   * Manual token refresh
   */
  const refreshTokenFn = useCallback(async () => {
    if (authDisabled) {
      return;
    }
    if (!config) {
      throw new Error("Auth config not loaded");
    }

    await performTokenRefresh(config);
  }, [config, performTokenRefresh, authDisabled]);

  /**
   * Get current access token (for API requests)
   */
  const getAccessTokenFn = useCallback((): string | null => {
    if (authDisabled) {
      return null;
    }
    const tokens = TokenStorage.getTokens();
    if (!tokens || isAccessTokenExpired(tokens)) {
      return null;
    }
    return tokens.accessToken;
  }, [authDisabled]);

  const value: AuthContextValue = {
    ...state,
    login,
    logout,
    handleCallback,
    refreshToken: refreshTokenFn,
    getAccessToken: getAccessTokenFn,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

/**
 * Hook to access auth context
 */
export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
}

/**
 * Hook to check if user has required role
 */
export function useHasRole(role: "admin" | "operator" | "viewer"): boolean {
  const { user } = useAuth();

  if (!user) return false;
  if (user.roles.includes("admin")) return true;
  if (role === "viewer" && user.roles.includes("operator")) return true;

  return user.roles.includes(role);
}

/**
 * Hook to check if user has any of the required roles
 */
export function useHasAnyRole(roles: Array<"admin" | "operator" | "viewer">): boolean {
  const { user } = useAuth();

  if (!user) return false;
  if (user.roles.includes("admin")) return true;

  return roles.some((role) => {
    if (role === "viewer" && user.roles.includes("operator")) return true;
    return user.roles.includes(role);
  });
}

// Re-export User type for convenience
export type { User };
