/**
 * Hooks index - exports all custom hooks
 */

// Theme
export { useTheme, ThemeProvider } from "./use-theme";

// Auth
export { AuthProvider, useAuth, useHasRole, useHasAnyRole } from "./use-auth";

// Data fetching hooks
export {
  useRuns,
  useRun,
  useRunResults,
  useRunSummary,
  useRunLogs,
  useRunArtifacts,
  useCreateRun,
  useCancelRun,
  useRetryRun,
  runKeys,
  type RunDetails,
  type RunWithResults,
  type TestResult,
  type LogEntry,
} from "./use-runs";

export {
  useServices,
  useService,
  useServiceTests,
  useServiceStats,
  useCreateService,
  useUpdateService,
  useDeleteService,
  useSyncService,
  serviceKeys,
  type Service,
  type ServiceStats,
  type TestDefinition,
  type RecentRunSummary,
  type ServiceFilterParams,
} from "./use-services";

export {
  useAgents,
  useAgent,
  useAgentStats,
  useDrainAgent,
  useUndrainAgent,
  useDeleteAgent,
  agentKeys,
  type AgentStats,
  type DrainAgentRequest,
} from "./use-agents";

// Analytics hooks
export {
  useFailureGroups,
  useFlakyTests,
  useFlakyTestHistory,
  useQuarantineTest,
  useUnquarantineTest,
  usePassRateTrend,
  useDurationTrend,
  useDailyStats,
  useTopFailingTests,
  useTopSlowestTests,
  useServiceComparison,
  analyticsKeys,
  type FlakyTest,
  type PassRateTrendPoint,
  type DurationTrendPoint,
  type DailyStats,
  type TopTest,
  type ServiceComparison,
  type FlakinessHistoryPoint,
} from "./use-analytics";

// WebSocket hooks
export {
  useWebSocket,
  useRunLogs as useRunLogsWs,
  useRunStatus,
  useAgentStatus,
  type WebSocketStatus,
  type WebSocketMessage,
  type LogMessage,
  type RunStatusUpdate,
  type AgentStatusUpdate,
  type UseWebSocketOptions,
  type UseWebSocketReturn,
} from "./use-websocket";
