/**
 * Authentication configuration and utilities
 */

import type {
  AuthConfig,
  OIDCConfig,
  JWTPayload,
  User,
  UserRole,
  PKCEPair,
  AuthorizationState,
  StoredTokens,
} from "@/types/auth";

// Storage keys
const STORAGE_KEYS = {
  tokens: "conductor-auth-tokens",
  authState: "conductor-auth-state",
  config: "conductor-auth-config",
} as const;

interface TokenResponsePayload {
  access_token: string;
  token_type: string;
  expires_in: number;
  refresh_token?: string;
  id_token?: string;
  scope?: string;
}

interface ErrorPayload {
  error?: string;
  error_description?: string;
}

/**
 * Get auth configuration from environment variables or API
 */
export async function getAuthConfig(): Promise<AuthConfig> {
  // Check for environment variables first
  const clientId = import.meta.env.VITE_OIDC_CLIENT_ID as string | undefined;
  const issuer = import.meta.env.VITE_OIDC_ISSUER as string | undefined;
  const redirectUri =
    (import.meta.env.VITE_OIDC_REDIRECT_URI as string | undefined) ||
    `${window.location.origin}/callback`;
  const postLogoutRedirectUri =
    (import.meta.env.VITE_OIDC_POST_LOGOUT_REDIRECT_URI as string | undefined) ||
    window.location.origin;
  const scopesEnv = import.meta.env.VITE_OIDC_SCOPES as string | undefined;
  const scopes = scopesEnv?.split(" ") || ["openid", "profile", "email"];

  if (clientId && issuer) {
    const config: AuthConfig = {
      clientId,
      issuer,
      redirectUri,
      postLogoutRedirectUri,
      scopes,
    };

    // Try to fetch OIDC discovery document
    try {
      const oidcConfig = await fetchOIDCConfig(issuer);
      config.oidcConfig = oidcConfig;
    } catch {
      console.warn("Failed to fetch OIDC discovery document");
    }

    return config;
  }

  // Fallback: fetch config from API
  const response = await fetch("/api/auth/config");
  if (!response.ok) {
    throw new Error(
      "Auth configuration not available. Set VITE_OIDC_CLIENT_ID and VITE_OIDC_ISSUER environment variables."
    );
  }
  return response.json() as Promise<AuthConfig>;
}

/**
 * Fetch OIDC configuration from discovery endpoint
 */
export async function fetchOIDCConfig(issuer: string): Promise<OIDCConfig> {
  const discoveryUrl = `${issuer.replace(/\/$/, "")}/.well-known/openid-configuration`;
  const response = await fetch(discoveryUrl);

  if (!response.ok) {
    throw new Error(`Failed to fetch OIDC configuration: ${response.statusText}`);
  }

  return response.json() as Promise<OIDCConfig>;
}

/**
 * Token storage with secure fallback
 */
export const TokenStorage = {
  /**
   * Store tokens securely
   */
  setTokens(tokens: StoredTokens): void {
    try {
      // Try sessionStorage first for better security (cleared on tab close)
      // Fall back to localStorage for persistence across sessions
      const storage = this.getStorage();
      storage.setItem(STORAGE_KEYS.tokens, JSON.stringify(tokens));
    } catch (error) {
      console.error("Failed to store tokens:", error);
    }
  },

  /**
   * Get stored tokens
   */
  getTokens(): StoredTokens | null {
    try {
      const storage = this.getStorage();
      const data = storage.getItem(STORAGE_KEYS.tokens);
      if (!data) return null;

      const tokens = JSON.parse(data) as StoredTokens;

      // Validate structure
      if (!tokens.accessToken || !tokens.expiresAt) {
        this.clearTokens();
        return null;
      }

      return tokens;
    } catch (error) {
      console.error("Failed to get tokens:", error);
      return null;
    }
  },

  /**
   * Clear stored tokens
   */
  clearTokens(): void {
    try {
      sessionStorage.removeItem(STORAGE_KEYS.tokens);
      localStorage.removeItem(STORAGE_KEYS.tokens);
      // Also clear legacy key
      localStorage.removeItem("conductor-auth-token");
    } catch (error) {
      console.error("Failed to clear tokens:", error);
    }
  },

  /**
   * Get the appropriate storage (prefer sessionStorage)
   */
  getStorage(): Storage {
    try {
      // Check if sessionStorage is available
      sessionStorage.setItem("test", "test");
      sessionStorage.removeItem("test");
      return sessionStorage;
    } catch {
      return localStorage;
    }
  },

  /**
   * Store authorization state before redirect
   */
  setAuthState(state: AuthorizationState): void {
    try {
      sessionStorage.setItem(STORAGE_KEYS.authState, JSON.stringify(state));
    } catch (error) {
      console.error("Failed to store auth state:", error);
    }
  },

  /**
   * Get and clear authorization state after callback
   */
  getAuthState(): AuthorizationState | null {
    try {
      const data = sessionStorage.getItem(STORAGE_KEYS.authState);
      if (!data) return null;

      // Clear after reading (one-time use)
      sessionStorage.removeItem(STORAGE_KEYS.authState);
      return JSON.parse(data) as AuthorizationState;
    } catch (error) {
      console.error("Failed to get auth state:", error);
      return null;
    }
  },

  /**
   * Clear authorization state
   */
  clearAuthState(): void {
    try {
      sessionStorage.removeItem(STORAGE_KEYS.authState);
    } catch (error) {
      console.error("Failed to clear auth state:", error);
    }
  },
};

/**
 * Parse a JWT token without verification (client-side only)
 * Note: Token verification should be done server-side
 */
export function parseJWT(token: string): JWTPayload | null {
  try {
    const parts = token.split(".");
    if (parts.length !== 3) {
      return null;
    }

    const payload = parts[1];
    if (!payload) {
      return null;
    }
    // Handle base64url encoding
    const base64 = payload.replace(/-/g, "+").replace(/_/g, "/");
    const padded = base64 + "=".repeat((4 - (base64.length % 4)) % 4);
    const decoded = atob(padded);
    return JSON.parse(decoded) as JWTPayload;
  } catch (error) {
    console.error("Failed to parse JWT:", error);
    return null;
  }
}

/**
 * Check if a token is expired
 * @param expiresAt - Token expiration timestamp in milliseconds
 * @param bufferSeconds - Buffer time before actual expiration (default 60s)
 */
export function isTokenExpired(expiresAt: number, bufferSeconds = 60): boolean {
  const bufferMs = bufferSeconds * 1000;
  return Date.now() >= expiresAt - bufferMs;
}

/**
 * Check if access token from stored tokens is expired
 */
export function isAccessTokenExpired(tokens: StoredTokens | null, bufferSeconds = 60): boolean {
  if (!tokens) return true;
  return isTokenExpired(tokens.expiresAt, bufferSeconds);
}

/**
 * Extract user information from tokens
 */
export function getCurrentUser(tokens: StoredTokens | null): User | null {
  if (!tokens) return null;

  // Prefer ID token for user info, fall back to access token
  const tokenToparse = tokens.idToken || tokens.accessToken;
  const payload = parseJWT(tokenToparse);

  if (!payload) return null;

  // Extract roles from various claim formats
  const roles = extractRoles(payload);

  return {
    id: payload.sub || "",
    email: payload.email || payload.preferred_username || "",
    name: payload.name || payload.email || "User",
    picture: payload.picture,
    roles,
  };
}

/**
 * Extract roles from JWT payload (supports various claim formats)
 */
function extractRoles(payload: JWTPayload): UserRole[] {
  const validRoles: UserRole[] = ["admin", "operator", "viewer"];
  const roles: UserRole[] = [];

  // Check 'roles' claim
  if (Array.isArray(payload.roles)) {
    for (const role of payload.roles) {
      const normalizedRole = role.toLowerCase() as UserRole;
      if (validRoles.includes(normalizedRole)) {
        roles.push(normalizedRole);
      }
    }
  }

  // Check 'groups' claim (common in enterprise IdPs)
  if (Array.isArray(payload.groups)) {
    for (const group of payload.groups) {
      const groupLower = group.toLowerCase();
      // Map group names to roles
      if (groupLower.includes("admin") || groupLower.includes("conductor-admin")) {
        if (!roles.includes("admin")) roles.push("admin");
      } else if (groupLower.includes("operator") || groupLower.includes("conductor-operator")) {
        if (!roles.includes("operator")) roles.push("operator");
      } else if (groupLower.includes("viewer") || groupLower.includes("conductor-viewer")) {
        if (!roles.includes("viewer")) roles.push("viewer");
      }
    }
  }

  // Default to viewer if no roles found
  if (roles.length === 0) {
    roles.push("viewer");
  }

  return roles;
}

/**
 * Generate a cryptographically random string
 */
export function generateRandomString(length: number): string {
  const array = new Uint8Array(length);
  crypto.getRandomValues(array);
  return Array.from(array, (byte) => byte.toString(16).padStart(2, "0")).join("");
}

/**
 * Generate a PKCE code verifier and challenge
 */
export async function generatePKCE(): Promise<PKCEPair> {
  // Generate code verifier (43-128 characters)
  const codeVerifier = generateRandomString(64);

  // Generate code challenge using SHA-256
  const encoder = new TextEncoder();
  const data = encoder.encode(codeVerifier);
  const digest = await crypto.subtle.digest("SHA-256", data);

  // Base64url encode the challenge
  const base64 = btoa(String.fromCharCode(...new Uint8Array(digest)));
  const codeChallenge = base64.replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");

  return { codeVerifier, codeChallenge };
}

/**
 * Build OIDC authorization URL
 */
export async function buildAuthorizationUrl(
  config: AuthConfig,
  returnTo?: string
): Promise<string> {
  const { codeVerifier, codeChallenge } = await generatePKCE();
  const state = generateRandomString(32);
  const nonce = generateRandomString(32);

  // Store state for validation on callback
  TokenStorage.setAuthState({
    state,
    codeVerifier,
    returnTo: returnTo || window.location.pathname,
    nonce,
  });

  // Build authorization URL
  const authEndpoint =
    config.oidcConfig?.authorization_endpoint || `${config.issuer}/authorize`;

  const params = new URLSearchParams({
    client_id: config.clientId,
    response_type: "code",
    redirect_uri: config.redirectUri,
    scope: config.scopes.join(" "),
    state,
    nonce,
    code_challenge: codeChallenge,
    code_challenge_method: "S256",
  });

  return `${authEndpoint}?${params.toString()}`;
}

/**
 * Exchange authorization code for tokens
 */
export async function exchangeCodeForTokens(
  config: AuthConfig,
  code: string,
  codeVerifier: string
): Promise<StoredTokens> {
  const tokenEndpoint = config.oidcConfig?.token_endpoint || `${config.issuer}/oauth/token`;

  const response = await fetch(tokenEndpoint, {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
    },
    body: new URLSearchParams({
      grant_type: "authorization_code",
      client_id: config.clientId,
      code,
      redirect_uri: config.redirectUri,
      code_verifier: codeVerifier,
    }),
  });

  if (!response.ok) {
    const errorPayload = (await response.json().catch(() => ({}))) as ErrorPayload;
    throw new Error(errorPayload.error_description || errorPayload.error || "Token exchange failed");
  }

  const tokenResponse = (await response.json()) as TokenResponsePayload;
  const expiresAt = Date.now() + tokenResponse.expires_in * 1000;

  return {
    accessToken: tokenResponse.access_token,
    refreshToken: tokenResponse.refresh_token,
    idToken: tokenResponse.id_token,
    expiresAt,
  };
}

/**
 * Refresh access token using refresh token
 */
export async function refreshAccessToken(
  config: AuthConfig,
  refreshToken: string
): Promise<StoredTokens> {
  const tokenEndpoint = config.oidcConfig?.token_endpoint || `${config.issuer}/oauth/token`;

  const response = await fetch(tokenEndpoint, {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
    },
    body: new URLSearchParams({
      grant_type: "refresh_token",
      client_id: config.clientId,
      refresh_token: refreshToken,
    }),
  });

  if (!response.ok) {
    const errorPayload = (await response.json().catch(() => ({}))) as ErrorPayload;
    throw new Error(errorPayload.error_description || errorPayload.error || "Token refresh failed");
  }

  const tokenResponse = (await response.json()) as TokenResponsePayload;
  const expiresAt = Date.now() + tokenResponse.expires_in * 1000;

  return {
    accessToken: tokenResponse.access_token,
    refreshToken: tokenResponse.refresh_token || refreshToken,
    idToken: tokenResponse.id_token,
    expiresAt,
  };
}

/**
 * Build OIDC end session URL
 */
export function buildLogoutUrl(config: AuthConfig, idToken?: string): string {
  const endSessionEndpoint =
    config.oidcConfig?.end_session_endpoint || `${config.issuer}/logout`;

  const params = new URLSearchParams({
    post_logout_redirect_uri: config.postLogoutRedirectUri,
  });

  if (idToken) {
    params.set("id_token_hint", idToken);
  }

  return `${endSessionEndpoint}?${params.toString()}`;
}

/**
 * Get access token for API requests (returns null if expired)
 */
export function getAccessToken(): string | null {
  const tokens = TokenStorage.getTokens();
  if (!tokens || isAccessTokenExpired(tokens)) {
    return null;
  }
  return tokens.accessToken;
}

/**
 * Check if user has required role
 */
export function hasRole(user: User | null, requiredRole: UserRole): boolean {
  if (!user) return false;

  // Admin has all permissions
  if (user.roles.includes("admin")) return true;

  // Operator has viewer permissions
  if (requiredRole === "viewer" && user.roles.includes("operator")) return true;

  return user.roles.includes(requiredRole);
}

/**
 * Check if user has any of the required roles
 */
export function hasAnyRole(user: User | null, requiredRoles: UserRole[]): boolean {
  if (!user) return false;

  // Admin has all permissions
  if (user.roles.includes("admin")) return true;

  return requiredRoles.some((role) => {
    if (role === "viewer" && user.roles.includes("operator")) return true;
    return user.roles.includes(role);
  });
}
