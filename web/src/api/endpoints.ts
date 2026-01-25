/**
 * API endpoint definitions for Conductor Dashboard
 * Type-safe request/response types and endpoint paths
 */

import { get, post, patch, del } from "@/lib/api";
import type {
  TestRun,
  Agent,
  Artifact,
  DashboardStats,
  RunHistoryPoint,
} from "@/types/models";
import type {
  PaginatedResponse,
  TestRunFilterParams,
  AgentFilterParams,
} from "@/types/api";

// =============================================================================
// Service Types
// =============================================================================

export interface Service {
  id: string;
  name: string;
  gitUrl: string;
  defaultBranch: string;
  networkZones: string[];
  owner: string;
  contact?: {
    name: string;
    email: string;
    slack?: string;
  };
  defaultExecutionType: "subprocess" | "container";
  defaultContainerImage?: string;
  defaultTimeout: number;
  configPath: string;
  labels: Record<string, string>;
  active: boolean;
  createdAt: string;
  updatedAt: string;
  lastSyncedAt?: string;
  testCount: number;
}

export interface ServiceWithStats extends Service {
  recentRuns: RecentRunSummary[];
  passRate: number;
  avgDuration: number;
}

export interface RecentRunSummary {
  id: string;
  status: TestRun["status"];
  branch: string;
  commitSha: string;
  createdAt: string;
  durationMs?: number;
  passed: number;
  failed: number;
  total: number;
}

export interface TestDefinition {
  id: string;
  serviceId: string;
  name: string;
  path: string;
  type: "unit" | "integration" | "e2e" | "performance" | "security" | "smoke";
  command: string;
  resultFormat: "junit" | "jest" | "playwright" | "go_test" | "tap" | "json";
  timeout: number;
  tags: string[];
  environment: Record<string, string>;
  artifactPaths: string[];
  enabled: boolean;
  retryCount: number;
  requiredRuntimes: string[];
  requiredNetworkZones: string[];
  createdAt: string;
  updatedAt: string;
  estimatedDuration?: number;
  flakinessRate?: number;
}

export interface ServiceStats {
  serviceId: string;
  totalRuns: number;
  passRate: number;
  avgDurationMs: number;
  passRateTrend: PassRateTrendPoint[];
  durationTrend: DurationTrendPoint[];
  testCount: number;
  activeTests: number;
}

export interface PassRateTrendPoint {
  date: string;
  passRate: number;
  total: number;
  passed: number;
  failed: number;
}

export interface DurationTrendPoint {
  date: string;
  avgDurationMs: number;
  p50DurationMs: number;
  p95DurationMs: number;
}

// =============================================================================
// Run Types
// =============================================================================

export interface RunDetails extends TestRun {
  trigger?: {
    type: "manual" | "webhook" | "scheduled" | "ci" | "api" | "retry";
    user?: string;
    ciJobId?: string;
    ciPipelineUrl?: string;
  };
  labels: Record<string, string>;
  retryOfRunId?: string;
  retryCount: number;
}

export interface RunWithResults extends RunDetails {
  results: TestResult[];
  artifacts: Artifact[];
}

export interface TestResult {
  id: string;
  runId: string;
  testId: string;
  testName: string;
  testPath: string;
  suiteName?: string;
  status: "passed" | "failed" | "skipped" | "error";
  durationMs: number;
  errorMessage?: string;
  stackTrace?: string;
  retryAttempt: number;
  timestamp: string;
  metadata: Record<string, string>;
  stdout?: string;
  stderr?: string;
}

export interface LogEntry {
  sequence: number;
  timestamp: string;
  stream: "stdout" | "stderr";
  message: string;
  testId?: string;
}

export interface RunSummary {
  total: number;
  passed: number;
  failed: number;
  skipped: number;
  errored: number;
  passRate: number;
}

// =============================================================================
// Request/Response Types
// =============================================================================

export interface CreateRunRequest {
  serviceId: string;
  gitRef: {
    branch: string;
    commitSha?: string;
  };
  testIds?: string[];
  tags?: string[];
  environment?: Record<string, string>;
  priority?: number;
  executionType?: "subprocess" | "container";
  timeout?: number;
}

export interface CancelRunRequest {
  reason?: string;
}

export interface RetryRunRequest {
  failedOnly?: boolean;
  environmentOverride?: Record<string, string>;
}

export interface ServiceFilterParams {
  owner?: string;
  networkZone?: string;
  query?: string;
  labels?: Record<string, string>;
  page?: number;
  pageSize?: number;
}

export interface TestResultFilterParams {
  statuses?: string[];
  suiteName?: string;
  namePattern?: string;
  page?: number;
  pageSize?: number;
  sortBy?: "name" | "duration" | "status";
  sortOrder?: "asc" | "desc";
}

export interface DrainAgentRequest {
  reason?: string;
  cancelActive?: boolean;
}

export interface AgentStats {
  agentId: string;
  totalRuns: number;
  successfulRuns: number;
  failedRuns: number;
  avgDurationMs: number;
  totalExecutionTimeMs: number;
  uptimePercent: number;
  avgResourceUsage: {
    cpuPercent: number;
    memoryBytes: number;
    memoryTotalBytes: number;
  };
}

// =============================================================================
// API Endpoints
// =============================================================================

export const endpoints = {
  // Dashboard
  dashboard: {
    stats: "/api/v1/dashboard/stats",
    runHistory: "/api/v1/dashboard/run-history",
  },

  // Runs
  runs: {
    list: "/api/v1/runs",
    get: (id: string) => `/api/v1/runs/${id}`,
    create: "/api/v1/runs",
    cancel: (id: string) => `/api/v1/runs/${id}/cancel`,
    retry: (id: string) => `/api/v1/runs/${id}/retry`,
    logs: (id: string) => `/api/v1/runs/${id}/logs`,
    logsStream: (id: string) => `/api/v1/runs/${id}/logs/stream`,
    results: (id: string) => `/api/v1/runs/${id}/results`,
    testResults: (id: string) => `/api/v1/runs/${id}/results/tests`,
    artifacts: (id: string) => `/api/v1/runs/${id}/artifacts`,
  },

  // Services
  services: {
    list: "/api/v1/services",
    get: (id: string) => `/api/v1/services/${id}`,
    create: "/api/v1/services",
    update: (id: string) => `/api/v1/services/${id}`,
    delete: (id: string) => `/api/v1/services/${id}`,
    sync: (id: string) => `/api/v1/services/${id}/sync`,
    tests: (id: string) => `/api/v1/services/${id}/tests`,
    stats: (id: string) => `/api/v1/services/${id}/stats`,
  },

  // Agents
  agents: {
    list: "/api/v1/agents",
    get: (id: string) => `/api/v1/agents/${id}`,
    drain: (id: string) => `/api/v1/agents/${id}/drain`,
    undrain: (id: string) => `/api/v1/agents/${id}/undrain`,
    delete: (id: string) => `/api/v1/agents/${id}`,
    stats: (id: string) => `/api/v1/agents/${id}/stats`,
  },

  // Artifacts
  artifacts: {
    get: (id: string) => `/api/v1/artifacts/${id}`,
    download: (id: string) => `/api/v1/artifacts/${id}/download`,
  },

  // WebSocket
  ws: {
    connect: "/ws",
  },
} as const;

// =============================================================================
// API Functions
// =============================================================================

// Dashboard
export const dashboardApi = {
  getStats: () => get<DashboardStats>(endpoints.dashboard.stats),
  getRunHistory: (days: number = 14) =>
    get<RunHistoryPoint[]>(endpoints.dashboard.runHistory, { days }),
};

// Runs
export const runsApi = {
  list: (params?: TestRunFilterParams) =>
    get<PaginatedResponse<TestRun>>(endpoints.runs.list, params as Record<string, unknown>),
  get: (id: string, includeResults = false, includeArtifacts = false) =>
    get<RunWithResults>(endpoints.runs.get(id), {
      includeResults,
      includeArtifacts,
    }),
  create: (data: CreateRunRequest) =>
    post<{ run: RunDetails }>(endpoints.runs.create, data),
  cancel: (id: string, reason?: string) =>
    post<{ run: RunDetails }>(endpoints.runs.cancel(id), { reason }),
  retry: (id: string, data?: RetryRunRequest) =>
    post<{ run: RunDetails; originalRunId: string }>(
      endpoints.runs.retry(id),
      data
    ),
  getLogs: (id: string, stream?: "stdout" | "stderr", testId?: string) =>
    get<{ entries: LogEntry[] }>(endpoints.runs.logs(id), { stream, testId }),
  getResults: (id: string) =>
    get<{ runId: string; status: string; summary: RunSummary }>(
      endpoints.runs.results(id)
    ),
  getTestResults: (id: string, params?: TestResultFilterParams) =>
    get<PaginatedResponse<TestResult>>(endpoints.runs.testResults(id), params as Record<string, unknown>),
  getArtifacts: (id: string) =>
    get<PaginatedResponse<Artifact>>(endpoints.runs.artifacts(id)),
};

// Services
export const servicesApi = {
  list: (params?: ServiceFilterParams) =>
    get<PaginatedResponse<Service>>(endpoints.services.list, params as Record<string, unknown>),
  get: (id: string, includeTests = false, includeRecentRuns = false) =>
    get<{
      service: Service;
      tests?: TestDefinition[];
      recentRuns?: RecentRunSummary[];
    }>(endpoints.services.get(id), { includeTests, includeRecentRuns }),
  create: (data: Partial<Service>) =>
    post<{ service: Service }>(endpoints.services.create, data),
  update: (id: string, data: Partial<Service>) =>
    patch<{ service: Service }>(endpoints.services.update(id), data),
  delete: (id: string, deleteHistory = false) =>
    del<{ success: boolean }>(
      `${endpoints.services.delete(id)}?deleteHistory=${deleteHistory}`
    ),
  sync: (id: string, branch?: string, deleteMissing = false) =>
    post<{
      testsAdded: number;
      testsUpdated: number;
      testsRemoved: number;
      errors: string[];
    }>(endpoints.services.sync(id), { branch, deleteMissing }),
  getTests: (id: string) =>
    get<PaginatedResponse<TestDefinition>>(endpoints.services.tests(id)),
  getStats: (id: string, days = 30) =>
    get<ServiceStats>(endpoints.services.stats(id), { days }),
};

// Agents
export const agentsApi = {
  list: (params?: AgentFilterParams) =>
    get<PaginatedResponse<Agent>>(endpoints.agents.list, params as Record<string, unknown>),
  get: (id: string, includeCurrentRuns = false) =>
    get<{
      agent: Agent;
      currentRuns?: Array<{
        runId: string;
        serviceName: string;
        startedAt: string;
        progressPercent: number;
      }>;
    }>(endpoints.agents.get(id), { includeCurrentRuns }),
  drain: (id: string, data?: DrainAgentRequest) =>
    post<{ agent: Agent; cancelledRuns: number }>(
      endpoints.agents.drain(id),
      data
    ),
  undrain: (id: string) =>
    post<{ agent: Agent }>(endpoints.agents.undrain(id), {}),
  delete: (id: string, force = false) =>
    del<{ success: boolean; message: string }>(
      `${endpoints.agents.delete(id)}?force=${force}`
    ),
  getStats: (id: string, startDate?: string, endDate?: string) =>
    get<AgentStats>(endpoints.agents.stats(id), { startDate, endDate }),
};

// Artifacts
export const artifactsApi = {
  get: (id: string) => get<{ artifact: Artifact }>(endpoints.artifacts.get(id)),
  getDownloadUrl: (id: string, expirationSeconds = 300) =>
    get<{ downloadUrl: string; expiresAt: string }>(
      endpoints.artifacts.download(id),
      { expirationSeconds }
    ),
};
