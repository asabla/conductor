/**
 * Analytics API endpoints for Conductor Dashboard
 * Provides failure analysis, flaky tests, and trend data
 */

import { get, post } from "@/lib/api";

// =============================================================================
// Types
// =============================================================================

export interface FailureGroup {
  id: string;
  errorSignature: string;
  errorMessage: string;
  count: number;
  firstOccurrence: string;
  lastOccurrence: string;
  affectedTests: AffectedTest[];
  trendData: TrendPoint[];
  services: string[];
}

export interface AffectedTest {
  testId: string;
  testName: string;
  testPath: string;
  serviceId: string;
  serviceName: string;
  runId: string;
  occurredAt: string;
}

export interface TrendPoint {
  date: string;
  count: number;
}

export interface FlakyTest {
  id: string;
  testId: string;
  testName: string;
  testPath: string;
  serviceId: string;
  serviceName: string;
  flakinessScore: number;
  totalRuns: number;
  flakyRuns: number;
  lastFlakyDate: string;
  isQuarantined: boolean;
  quarantinedAt?: string;
  quarantinedBy?: string;
  historyData: FlakinessHistoryPoint[];
}

export interface FlakinessHistoryPoint {
  date: string;
  flakinessScore: number;
  totalRuns: number;
  flakyRuns: number;
}

export interface DailyStats {
  date: string;
  totalRuns: number;
  passedRuns: number;
  failedRuns: number;
  passRate: number;
  avgDurationMs: number;
  totalTests: number;
}

export interface TopTest {
  testId: string;
  testName: string;
  testPath: string;
  serviceId: string;
  serviceName: string;
  value: number; // failure count or duration
  trend: number; // percentage change
}

export interface ServiceComparison {
  serviceId: string;
  serviceName: string;
  passRate: number;
  passRateTrend: number;
  avgDurationMs: number;
  durationTrend: number;
  totalRuns: number;
  flakyTestCount: number;
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
// Request/Response Types
// =============================================================================

export interface FailureGroupsParams {
  timeRange: "24h" | "7d" | "30d";
  serviceId?: string;
  page?: number;
  pageSize?: number;
}

export interface FailureGroupsResponse {
  groups: FailureGroup[];
  total: number;
  page: number;
  pageSize: number;
}

export interface FlakyTestsParams {
  serviceId?: string;
  showQuarantined?: boolean;
  sortBy?: "flakinessScore" | "lastFlakyDate" | "totalRuns";
  sortOrder?: "asc" | "desc";
  page?: number;
  pageSize?: number;
}

export interface FlakyTestsResponse {
  tests: FlakyTest[];
  total: number;
  page: number;
  pageSize: number;
}

export interface TrendParams {
  serviceId?: string;
  days: number;
}

export interface ServiceStatsParams {
  days: number;
}

// =============================================================================
// API Endpoints
// =============================================================================

export const analyticsEndpoints = {
  failureGroups: "/api/v1/analytics/failure-groups",
  flakyTests: "/api/v1/analytics/flaky-tests",
  quarantineTest: (testId: string) => `/api/v1/analytics/flaky-tests/${testId}/quarantine`,
  unquarantineTest: (testId: string) => `/api/v1/analytics/flaky-tests/${testId}/unquarantine`,
  flakyTestHistory: (testId: string) => `/api/v1/analytics/flaky-tests/${testId}/history`,
  passRateTrend: "/api/v1/analytics/trends/pass-rate",
  durationTrend: "/api/v1/analytics/trends/duration",
  dailyStats: "/api/v1/analytics/trends/daily-stats",
  topFailingTests: "/api/v1/analytics/top-failing",
  topSlowestTests: "/api/v1/analytics/top-slowest",
  serviceComparison: "/api/v1/analytics/service-comparison",
} as const;

// =============================================================================
// API Functions
// =============================================================================

export const analyticsApi = {
  // Failure analysis
  getFailureGroups: (params: FailureGroupsParams) =>
    get<FailureGroupsResponse>(analyticsEndpoints.failureGroups, params as unknown as Record<string, unknown>),

  // Flaky tests
  getFlakyTests: (params: FlakyTestsParams) =>
    get<FlakyTestsResponse>(analyticsEndpoints.flakyTests, params as unknown as Record<string, unknown>),

  quarantineTest: (testId: string, reason?: string) =>
    post<{ success: boolean; test: FlakyTest }>(
      analyticsEndpoints.quarantineTest(testId),
      { reason }
    ),

  unquarantineTest: (testId: string) =>
    post<{ success: boolean; test: FlakyTest }>(
      analyticsEndpoints.unquarantineTest(testId),
      {}
    ),

  getFlakyTestHistory: (testId: string, days: number = 30) =>
    get<{ history: FlakinessHistoryPoint[] }>(
      analyticsEndpoints.flakyTestHistory(testId),
      { days }
    ),

  // Trends
  getPassRateTrend: (params: TrendParams) =>
    get<{ data: PassRateTrendPoint[] }>(analyticsEndpoints.passRateTrend, params as unknown as Record<string, unknown>),

  getDurationTrend: (params: TrendParams) =>
    get<{ data: DurationTrendPoint[] }>(analyticsEndpoints.durationTrend, params as unknown as Record<string, unknown>),

  getDailyStats: (params: TrendParams) =>
    get<{ data: DailyStats[] }>(analyticsEndpoints.dailyStats, params as unknown as Record<string, unknown>),

  getTopFailingTests: (limit: number = 10, serviceId?: string) =>
    get<{ tests: TopTest[] }>(analyticsEndpoints.topFailingTests, { limit, serviceId }),

  getTopSlowestTests: (limit: number = 10, serviceId?: string) =>
    get<{ tests: TopTest[] }>(analyticsEndpoints.topSlowestTests, { limit, serviceId }),

  getServiceComparison: (params: ServiceStatsParams) =>
    get<{ services: ServiceComparison[] }>(analyticsEndpoints.serviceComparison, params as unknown as Record<string, unknown>),
};
