/**
 * Authentication types for OIDC
 */

/**
 * User role for role-based access control
 */
export type UserRole = "admin" | "operator" | "viewer";

/**
 * Authenticated user information extracted from token
 */
export interface User {
  id: string;
  email: string;
  name: string;
  picture?: string;
  roles: UserRole[];
}

/**
 * Authentication state
 */
export interface AuthState {
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  error: string | null;
}

/**
 * Token response from OIDC provider
 */
export interface TokenResponse {
  access_token: string;
  token_type: string;
  expires_in: number;
  refresh_token?: string;
  id_token?: string;
  scope?: string;
}

/**
 * OIDC configuration from discovery endpoint or static config
 */
export interface OIDCConfig {
  issuer: string;
  authorization_endpoint: string;
  token_endpoint: string;
  userinfo_endpoint?: string;
  end_session_endpoint?: string;
  jwks_uri?: string;
  scopes_supported?: string[];
}

/**
 * Auth configuration for the application
 */
export interface AuthConfig {
  clientId: string;
  issuer: string;
  redirectUri: string;
  postLogoutRedirectUri: string;
  scopes: string[];
  oidcConfig?: OIDCConfig;
}

/**
 * JWT payload structure (standard claims)
 */
export interface JWTPayload {
  iss?: string;
  sub?: string;
  aud?: string | string[];
  exp?: number;
  nbf?: number;
  iat?: number;
  jti?: string;
  // Custom claims
  email?: string;
  name?: string;
  picture?: string;
  roles?: string[];
  groups?: string[];
  preferred_username?: string;
}

/**
 * PKCE code verifier and challenge
 */
export interface PKCEPair {
  codeVerifier: string;
  codeChallenge: string;
}

/**
 * Authorization request state stored before redirect
 */
export interface AuthorizationState {
  state: string;
  codeVerifier: string;
  returnTo: string;
  nonce: string;
}

/**
 * Stored tokens
 */
export interface StoredTokens {
  accessToken: string;
  refreshToken?: string;
  idToken?: string;
  expiresAt: number;
}
