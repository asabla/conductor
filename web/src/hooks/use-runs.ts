/**
 * Hooks for managing test runs data
 */

import {
  useQuery,
  useMutation,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";
import { runsApi, type RunDetails, type RunWithResults, type TestResult, type CreateRunRequest, type RetryRunRequest, type TestResultFilterParams, type LogEntry } from "@/api/endpoints";
import type { TestRun } from "@/types/models";
import type { PaginatedResponse, TestRunFilterParams } from "@/types/api";

// =============================================================================
// Query Keys
// =============================================================================

export const runKeys = {
  all: ["runs"] as const,
  lists: () => [...runKeys.all, "list"] as const,
  list: (filters: TestRunFilterParams) =>
    [...runKeys.lists(), filters] as const,
  details: () => [...runKeys.all, "detail"] as const,
  detail: (id: string) => [...runKeys.details(), id] as const,
  results: (id: string) => [...runKeys.detail(id), "results"] as const,
  testResults: (id: string, filters?: TestResultFilterParams) =>
    [...runKeys.detail(id), "testResults", filters] as const,
  logs: (id: string) => [...runKeys.detail(id), "logs"] as const,
  artifacts: (id: string) => [...runKeys.detail(id), "artifacts"] as const,
};

// =============================================================================
// Hooks
// =============================================================================

/**
 * Fetch paginated list of test runs with optional filters
 */
export function useRuns(
  filters: TestRunFilterParams = {},
  options?: Omit<
    UseQueryOptions<PaginatedResponse<TestRun>>,
    "queryKey" | "queryFn"
  >
) {
  return useQuery({
    queryKey: runKeys.list(filters),
    queryFn: () => runsApi.list(filters),
    ...options,
  });
}

/**
 * Fetch a single test run by ID
 */
export function useRun(
  id: string | undefined,
  options?: {
    includeResults?: boolean;
    includeArtifacts?: boolean;
    includeShards?: boolean;
    enabled?: boolean;
    refetchInterval?: number | false;
  }
) {
  const {
    includeResults = false,
    includeArtifacts = false,
    includeShards = false,
    enabled = true,
    refetchInterval,
  } = options ?? {};

  return useQuery({
    queryKey: runKeys.detail(id!),
    queryFn: () => runsApi.get(id!, includeResults, includeArtifacts, includeShards),
    enabled: !!id && enabled,
    refetchInterval,
  });
}

/**
 * Fetch test results for a run
 */
export function useRunResults(
  runId: string | undefined,
  filters?: TestResultFilterParams,
  options?: { enabled?: boolean }
) {
  const { enabled = true } = options ?? {};

  return useQuery({
    queryKey: runKeys.testResults(runId!, filters),
    queryFn: () => runsApi.getTestResults(runId!, filters),
    enabled: !!runId && enabled,
  });
}

/**
 * Fetch run summary/aggregated results
 */
export function useRunSummary(
  runId: string | undefined,
  options?: { enabled?: boolean }
) {
  const { enabled = true } = options ?? {};

  return useQuery({
    queryKey: runKeys.results(runId!),
    queryFn: () => runsApi.getResults(runId!),
    enabled: !!runId && enabled,
  });
}

/**
 * Fetch logs for a run
 */
export function useRunLogs(
  runId: string | undefined,
  options?: {
    stream?: "stdout" | "stderr";
    testId?: string;
    enabled?: boolean;
  }
) {
  const { stream, testId, enabled = true } = options ?? {};

  return useQuery({
    queryKey: runKeys.logs(runId!),
    queryFn: () => runsApi.getLogs(runId!, stream, testId),
    enabled: !!runId && enabled,
  });
}

/**
 * Fetch artifacts for a run
 */
export function useRunArtifacts(
  runId: string | undefined,
  options?: { enabled?: boolean }
) {
  const { enabled = true } = options ?? {};

  return useQuery({
    queryKey: runKeys.artifacts(runId!),
    queryFn: () => runsApi.getArtifacts(runId!),
    enabled: !!runId && enabled,
  });
}

/**
 * Create a new test run
 */
export function useCreateRun() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateRunRequest) => runsApi.create(data),
    onSuccess: () => {
      // Invalidate runs list to refetch
      void queryClient.invalidateQueries({ queryKey: runKeys.lists() });
    },
  });
}

/**
 * Cancel a running test
 */
export function useCancelRun() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      runId,
      reason,
      shardId,
    }: {
      runId: string;
      reason?: string;
      shardId?: string;
    }) => runsApi.cancel(runId, reason, shardId),
    onSuccess: (data) => {
      // Update the specific run in cache
      queryClient.setQueryData(runKeys.detail(data.run.id), data.run);
      // Invalidate runs list
      void queryClient.invalidateQueries({ queryKey: runKeys.lists() });
    },
  });
}

/**
 * Retry a failed run
 */
export function useRetryRun() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      runId,
      ...data
    }: { runId: string } & RetryRunRequest) =>
      runsApi.retry(runId, data),
    onSuccess: () => {
      // Invalidate runs list to show new run
      void queryClient.invalidateQueries({ queryKey: runKeys.lists() });
    },
  });
}

// =============================================================================
// Utility Types
// =============================================================================

export type { RunDetails, RunWithResults, TestResult, LogEntry };
