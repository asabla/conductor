/**
 * Analytics hooks for fetching failure analysis, flaky tests, and trends data
 */

import {
  useQuery,
  useMutation,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";
import {
  analyticsApi,
  type FailureGroupsParams,
  type FailureGroupsResponse,
  type FlakyTestsParams,
  type FlakyTestsResponse,
  type FlakyTest,
  type PassRateTrendPoint,
  type DurationTrendPoint,
  type DailyStats,
  type TopTest,
  type ServiceComparison,
  type FlakinessHistoryPoint,
} from "@/api/analytics";

// =============================================================================
// Query Keys
// =============================================================================

export const analyticsKeys = {
  all: ["analytics"] as const,
  failureGroups: () => [...analyticsKeys.all, "failure-groups"] as const,
  failureGroupsList: (params: FailureGroupsParams) =>
    [...analyticsKeys.failureGroups(), params] as const,
  flakyTests: () => [...analyticsKeys.all, "flaky-tests"] as const,
  flakyTestsList: (params: FlakyTestsParams) =>
    [...analyticsKeys.flakyTests(), params] as const,
  flakyTestHistory: (testId: string, days: number) =>
    [...analyticsKeys.flakyTests(), "history", testId, days] as const,
  trends: () => [...analyticsKeys.all, "trends"] as const,
  passRateTrend: (serviceId?: string, days?: number) =>
    [...analyticsKeys.trends(), "pass-rate", serviceId, days] as const,
  durationTrend: (serviceId?: string, days?: number) =>
    [...analyticsKeys.trends(), "duration", serviceId, days] as const,
  dailyStats: (serviceId?: string, days?: number) =>
    [...analyticsKeys.trends(), "daily-stats", serviceId, days] as const,
  topFailingTests: (limit?: number, serviceId?: string) =>
    [...analyticsKeys.all, "top-failing", limit, serviceId] as const,
  topSlowestTests: (limit?: number, serviceId?: string) =>
    [...analyticsKeys.all, "top-slowest", limit, serviceId] as const,
  serviceComparison: (days?: number) =>
    [...analyticsKeys.all, "service-comparison", days] as const,
};

// =============================================================================
// Failure Groups Hooks
// =============================================================================

/**
 * Fetch failure groups with optional filters
 */
export function useFailureGroups(
  params: FailureGroupsParams,
  options?: Omit<
    UseQueryOptions<FailureGroupsResponse>,
    "queryKey" | "queryFn"
  >
) {
  return useQuery({
    queryKey: analyticsKeys.failureGroupsList(params),
    queryFn: () => analyticsApi.getFailureGroups(params),
    ...options,
  });
}

// =============================================================================
// Flaky Tests Hooks
// =============================================================================

/**
 * Fetch flaky tests list with optional filters
 */
export function useFlakyTests(
  params: FlakyTestsParams = {},
  options?: Omit<
    UseQueryOptions<FlakyTestsResponse>,
    "queryKey" | "queryFn"
  >
) {
  return useQuery({
    queryKey: analyticsKeys.flakyTestsList(params),
    queryFn: () => analyticsApi.getFlakyTests(params),
    ...options,
  });
}

/**
 * Fetch flaky test history for charts
 */
export function useFlakyTestHistory(
  testId: string | undefined,
  days: number = 30,
  options?: { enabled?: boolean }
) {
  const { enabled = true } = options ?? {};

  return useQuery({
    queryKey: analyticsKeys.flakyTestHistory(testId!, days),
    queryFn: () => analyticsApi.getFlakyTestHistory(testId!, days),
    enabled: !!testId && enabled,
  });
}

/**
 * Quarantine a flaky test
 */
export function useQuarantineTest() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ testId, reason }: { testId: string; reason?: string }) =>
      analyticsApi.quarantineTest(testId, reason),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: analyticsKeys.flakyTests() });
    },
  });
}

/**
 * Unquarantine a flaky test
 */
export function useUnquarantineTest() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (testId: string) => analyticsApi.unquarantineTest(testId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: analyticsKeys.flakyTests() });
    },
  });
}

// =============================================================================
// Trend Hooks
// =============================================================================

/**
 * Fetch pass rate trend over time
 */
export function usePassRateTrend(
  serviceId?: string,
  days: number = 30,
  options?: { enabled?: boolean }
) {
  const { enabled = true } = options ?? {};

  return useQuery({
    queryKey: analyticsKeys.passRateTrend(serviceId, days),
    queryFn: () => analyticsApi.getPassRateTrend({ serviceId, days }),
    enabled,
  });
}

/**
 * Fetch duration trend over time
 */
export function useDurationTrend(
  serviceId?: string,
  days: number = 30,
  options?: { enabled?: boolean }
) {
  const { enabled = true } = options ?? {};

  return useQuery({
    queryKey: analyticsKeys.durationTrend(serviceId, days),
    queryFn: () => analyticsApi.getDurationTrend({ serviceId, days }),
    enabled,
  });
}

/**
 * Fetch daily aggregated stats
 */
export function useDailyStats(
  serviceId?: string,
  days: number = 30,
  options?: { enabled?: boolean }
) {
  const { enabled = true } = options ?? {};

  return useQuery({
    queryKey: analyticsKeys.dailyStats(serviceId, days),
    queryFn: () => analyticsApi.getDailyStats({ serviceId, days }),
    enabled,
  });
}

/**
 * Fetch top failing tests
 */
export function useTopFailingTests(
  limit: number = 10,
  serviceId?: string,
  options?: { enabled?: boolean }
) {
  const { enabled = true } = options ?? {};

  return useQuery({
    queryKey: analyticsKeys.topFailingTests(limit, serviceId),
    queryFn: () => analyticsApi.getTopFailingTests(limit, serviceId),
    enabled,
  });
}

/**
 * Fetch top slowest tests
 */
export function useTopSlowestTests(
  limit: number = 10,
  serviceId?: string,
  options?: { enabled?: boolean }
) {
  const { enabled = true } = options ?? {};

  return useQuery({
    queryKey: analyticsKeys.topSlowestTests(limit, serviceId),
    queryFn: () => analyticsApi.getTopSlowestTests(limit, serviceId),
    enabled,
  });
}

/**
 * Fetch service comparison data
 */
export function useServiceComparison(
  days: number = 30,
  options?: { enabled?: boolean }
) {
  const { enabled = true } = options ?? {};

  return useQuery({
    queryKey: analyticsKeys.serviceComparison(days),
    queryFn: () => analyticsApi.getServiceComparison({ days }),
    enabled,
  });
}

// =============================================================================
// Utility Types
// =============================================================================

export type {
  FlakyTest,
  PassRateTrendPoint,
  DurationTrendPoint,
  DailyStats,
  TopTest,
  ServiceComparison,
  FlakinessHistoryPoint,
};
