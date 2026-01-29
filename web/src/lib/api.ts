import axios, {
  type AxiosError,
  type AxiosInstance,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from "axios";
import type { ApiError } from "@/types/api";
import { TokenStorage, isAccessTokenExpired, refreshAccessToken, getAuthConfig } from "@/lib/auth";
import type { AuthConfig } from "@/types/auth";

const BASE_URL = import.meta.env.VITE_API_URL || "/api";

// Store auth config for refresh
let authConfigPromise: Promise<AuthConfig> | null = null;
let isRefreshing = false;
let refreshPromise: Promise<void> | null = null;

/**
 * Get auth config (cached)
 */
async function getCachedAuthConfig(): Promise<AuthConfig> {
  if (!authConfigPromise) {
    authConfigPromise = getAuthConfig();
  }
  return authConfigPromise;
}

/**
 * Perform token refresh (with deduplication)
 */
async function performTokenRefresh(): Promise<void> {
  if (isRefreshing && refreshPromise) {
    return refreshPromise;
  }

  isRefreshing = true;
  refreshPromise = (async () => {
    try {
      const tokens = TokenStorage.getTokens();
      if (!tokens?.refreshToken) {
        throw new Error("No refresh token");
      }

      const config = await getCachedAuthConfig();
      const newTokens = await refreshAccessToken(config, tokens.refreshToken);
      TokenStorage.setTokens(newTokens);
    } finally {
      isRefreshing = false;
      refreshPromise = null;
    }
  })();

  return refreshPromise;
}

/**
 * Create and configure the API client
 */
function createApiClient(): AxiosInstance {
  const client = axios.create({
    baseURL: BASE_URL,
    timeout: 30000,
    headers: {
      "Content-Type": "application/json",
    },
  });

  // Request interceptor for auth
  client.interceptors.request.use(
    async (config: InternalAxiosRequestConfig) => {
      const tokens = TokenStorage.getTokens();

      if (tokens) {
        // Check if token is about to expire (within 30 seconds)
        if (isAccessTokenExpired(tokens, 30) && tokens.refreshToken) {
          try {
            await performTokenRefresh();
            // Get fresh token after refresh
            const freshTokens = TokenStorage.getTokens();
            if (freshTokens && config.headers) {
              config.headers.Authorization = `Bearer ${freshTokens.accessToken}`;
            }
          } catch {
            // Refresh failed, proceed with existing token (will likely 401)
            if (config.headers) {
              config.headers.Authorization = `Bearer ${tokens.accessToken}`;
            }
          }
        } else if (config.headers) {
          config.headers.Authorization = `Bearer ${tokens.accessToken}`;
        }
      }

      return config;
    },
    (error: AxiosError) => {
      return Promise.reject(error);
    }
  );

  // Response interceptor for error handling
  client.interceptors.response.use(
    (response: AxiosResponse) => response,
    async (error: AxiosError<ApiError>) => {
      const originalRequest = error.config as InternalAxiosRequestConfig & {
        _retry?: boolean;
      };

      if (error.response) {
        const { status, data } = error.response;

        // Handle unauthorized errors with token refresh
        if (status === 401 && !originalRequest._retry) {
          originalRequest._retry = true;

          const tokens = TokenStorage.getTokens();
          if (tokens?.refreshToken) {
            try {
              await performTokenRefresh();

              // Retry the original request with new token
              const freshTokens = TokenStorage.getTokens();
              if (freshTokens && originalRequest.headers) {
                originalRequest.headers.Authorization = `Bearer ${freshTokens.accessToken}`;
              }

              return client(originalRequest);
            } catch {
              // Refresh failed, redirect to login
              TokenStorage.clearTokens();
              const returnTo = encodeURIComponent(
                window.location.pathname + window.location.search
              );
              window.location.href = `/login?returnTo=${returnTo}`;
              return Promise.reject(error);
            }
          }

          // No refresh token, redirect to login
          TokenStorage.clearTokens();
          const returnTo = encodeURIComponent(
            window.location.pathname + window.location.search
          );
          window.location.href = `/login?returnTo=${returnTo}`;
          return Promise.reject(error);
        }

        // Enhance error with API error details
        const apiError = Object.assign(
          new Error(data?.message || error.message),
          {
            code: data?.code || `HTTP_${status}`,
            status,
            details: data?.details,
          }
        ) as ApiError & Error;

        return Promise.reject(apiError);
      }

      // Network or other errors
      const networkError = Object.assign(
        new Error(error.message || "Network error"),
        {
          code: "NETWORK_ERROR",
          status: 0,
        }
      ) as ApiError & Error;

      return Promise.reject(networkError);
    }
  );

  return client;
}

export const api = createApiClient();

/**
 * Type-safe GET request
 */
export async function get<T>(url: string, params?: Record<string, unknown>): Promise<T> {
  const response = await api.get<T>(url, { params });
  return response.data;
}

/**
 * Type-safe POST request
 */
export async function post<T, D = unknown>(url: string, data?: D): Promise<T> {
  const response = await api.post<T>(url, data);
  return response.data;
}

/**
 * Type-safe PUT request
 */
export async function put<T, D = unknown>(url: string, data?: D): Promise<T> {
  const response = await api.put<T>(url, data);
  return response.data;
}

/**
 * Type-safe PATCH request
 */
export async function patch<T, D = unknown>(url: string, data?: D): Promise<T> {
  const response = await api.patch<T>(url, data);
  return response.data;
}

/**
 * Type-safe DELETE request
 */
export async function del<T>(url: string): Promise<T> {
  const response = await api.delete<T>(url);
  return response.data;
}
