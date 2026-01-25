/**
 * Hooks for managing agents data
 */

import {
  useQuery,
  useMutation,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";
import {
  agentsApi,
  type AgentStats,
  type DrainAgentRequest,
} from "@/api/endpoints";
import type { Agent } from "@/types/models";
import type { PaginatedResponse, AgentFilterParams } from "@/types/api";

// =============================================================================
// Query Keys
// =============================================================================

export const agentKeys = {
  all: ["agents"] as const,
  lists: () => [...agentKeys.all, "list"] as const,
  list: (filters: AgentFilterParams) =>
    [...agentKeys.lists(), filters] as const,
  details: () => [...agentKeys.all, "detail"] as const,
  detail: (id: string) => [...agentKeys.details(), id] as const,
  stats: (id: string, startDate?: string, endDate?: string) =>
    [...agentKeys.detail(id), "stats", startDate, endDate] as const,
};

// =============================================================================
// Hooks
// =============================================================================

/**
 * Fetch paginated list of agents with optional filters
 */
export function useAgents(
  filters: AgentFilterParams = {},
  options?: Omit<
    UseQueryOptions<PaginatedResponse<Agent>>,
    "queryKey" | "queryFn"
  >
) {
  return useQuery({
    queryKey: agentKeys.list(filters),
    queryFn: () => agentsApi.list(filters),
    ...options,
  });
}

/**
 * Fetch a single agent by ID
 */
export function useAgent(
  id: string | undefined,
  options?: {
    includeCurrentRuns?: boolean;
    enabled?: boolean;
    refetchInterval?: number | false;
  }
) {
  const {
    includeCurrentRuns = false,
    enabled = true,
    refetchInterval,
  } = options ?? {};

  return useQuery({
    queryKey: agentKeys.detail(id!),
    queryFn: () => agentsApi.get(id!, includeCurrentRuns),
    enabled: !!id && enabled,
    refetchInterval,
  });
}

/**
 * Fetch statistics for an agent
 */
export function useAgentStats(
  agentId: string | undefined,
  options?: {
    startDate?: string;
    endDate?: string;
    enabled?: boolean;
  }
) {
  const { startDate, endDate, enabled = true } = options ?? {};

  return useQuery({
    queryKey: agentKeys.stats(agentId!, startDate, endDate),
    queryFn: () => agentsApi.getStats(agentId!, startDate, endDate),
    enabled: !!agentId && enabled,
  });
}

/**
 * Drain an agent (stop accepting new work)
 */
export function useDrainAgent() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, ...data }: { id: string } & DrainAgentRequest) =>
      agentsApi.drain(id, data),
    onSuccess: (data) => {
      // Update the specific agent in cache
      queryClient.setQueryData(agentKeys.detail(data.agent.id), {
        agent: data.agent,
      });
      // Invalidate agents list
      void queryClient.invalidateQueries({ queryKey: agentKeys.lists() });
    },
  });
}

/**
 * Undrain an agent (allow accepting new work)
 */
export function useUndrainAgent() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => agentsApi.undrain(id),
    onSuccess: (data) => {
      // Update the specific agent in cache
      queryClient.setQueryData(agentKeys.detail(data.agent.id), {
        agent: data.agent,
      });
      // Invalidate agents list
      void queryClient.invalidateQueries({ queryKey: agentKeys.lists() });
    },
  });
}

/**
 * Delete an agent
 */
export function useDeleteAgent() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, force = false }: { id: string; force?: boolean }) =>
      agentsApi.delete(id, force),
    onSuccess: (_, variables) => {
      // Remove the agent from cache
      queryClient.removeQueries({ queryKey: agentKeys.detail(variables.id) });
      // Invalidate agents list
      void queryClient.invalidateQueries({ queryKey: agentKeys.lists() });
    },
  });
}

// =============================================================================
// Utility Types
// =============================================================================

export type { AgentStats, DrainAgentRequest };
