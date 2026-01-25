/**
 * Hooks for managing services data
 */

import {
  useQuery,
  useMutation,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";
import {
  servicesApi,
  type Service,
  type ServiceStats,
  type TestDefinition,
  type RecentRunSummary,
  type ServiceFilterParams,
} from "@/api/endpoints";
import type { PaginatedResponse } from "@/types/api";

// =============================================================================
// Query Keys
// =============================================================================

export const serviceKeys = {
  all: ["services"] as const,
  lists: () => [...serviceKeys.all, "list"] as const,
  list: (filters: ServiceFilterParams) =>
    [...serviceKeys.lists(), filters] as const,
  details: () => [...serviceKeys.all, "detail"] as const,
  detail: (id: string) => [...serviceKeys.details(), id] as const,
  tests: (id: string) => [...serviceKeys.detail(id), "tests"] as const,
  stats: (id: string, days?: number) =>
    [...serviceKeys.detail(id), "stats", days] as const,
  recentRuns: (id: string) =>
    [...serviceKeys.detail(id), "recentRuns"] as const,
};

// =============================================================================
// Hooks
// =============================================================================

/**
 * Fetch paginated list of services with optional filters
 */
export function useServices(
  filters: ServiceFilterParams = {},
  options?: Omit<
    UseQueryOptions<PaginatedResponse<Service>>,
    "queryKey" | "queryFn"
  >
) {
  return useQuery({
    queryKey: serviceKeys.list(filters),
    queryFn: () => servicesApi.list(filters),
    ...options,
  });
}

/**
 * Fetch a single service by ID
 */
export function useService(
  id: string | undefined,
  options?: {
    includeTests?: boolean;
    includeRecentRuns?: boolean;
    enabled?: boolean;
  }
) {
  const {
    includeTests = false,
    includeRecentRuns = false,
    enabled = true,
  } = options ?? {};

  return useQuery({
    queryKey: serviceKeys.detail(id!),
    queryFn: () => servicesApi.get(id!, includeTests, includeRecentRuns),
    enabled: !!id && enabled,
  });
}

/**
 * Fetch test definitions for a service
 */
export function useServiceTests(
  serviceId: string | undefined,
  options?: { enabled?: boolean }
) {
  const { enabled = true } = options ?? {};

  return useQuery({
    queryKey: serviceKeys.tests(serviceId!),
    queryFn: () => servicesApi.getTests(serviceId!),
    enabled: !!serviceId && enabled,
  });
}

/**
 * Fetch statistics for a service
 */
export function useServiceStats(
  serviceId: string | undefined,
  options?: { days?: number; enabled?: boolean }
) {
  const { days = 30, enabled = true } = options ?? {};

  return useQuery({
    queryKey: serviceKeys.stats(serviceId!, days),
    queryFn: () => servicesApi.getStats(serviceId!, days),
    enabled: !!serviceId && enabled,
  });
}

/**
 * Create a new service
 */
export function useCreateService() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: Partial<Service>) => servicesApi.create(data),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: serviceKeys.lists() });
    },
  });
}

/**
 * Update an existing service
 */
export function useUpdateService() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, ...data }: { id: string } & Partial<Service>) =>
      servicesApi.update(id, data),
    onSuccess: (data) => {
      queryClient.setQueryData(
        serviceKeys.detail(data.service.id),
        data
      );
      void queryClient.invalidateQueries({ queryKey: serviceKeys.lists() });
    },
  });
}

/**
 * Delete a service
 */
export function useDeleteService() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      id,
      deleteHistory = false,
    }: {
      id: string;
      deleteHistory?: boolean;
    }) => servicesApi.delete(id, deleteHistory),
    onSuccess: (_, variables) => {
      queryClient.removeQueries({
        queryKey: serviceKeys.detail(variables.id),
      });
      void queryClient.invalidateQueries({ queryKey: serviceKeys.lists() });
    },
  });
}

/**
 * Sync service tests from repository
 */
export function useSyncService() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      id,
      branch,
      deleteMissing = false,
    }: {
      id: string;
      branch?: string;
      deleteMissing?: boolean;
    }) => servicesApi.sync(id, branch, deleteMissing),
    onSuccess: (_, variables) => {
      // Invalidate service detail and tests
      void queryClient.invalidateQueries({
        queryKey: serviceKeys.detail(variables.id),
      });
      void queryClient.invalidateQueries({
        queryKey: serviceKeys.tests(variables.id),
      });
    },
  });
}

// =============================================================================
// Utility Types
// =============================================================================

export type {
  Service,
  ServiceStats,
  TestDefinition,
  RecentRunSummary,
  ServiceFilterParams,
};
