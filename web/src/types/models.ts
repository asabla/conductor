/**
 * Test run status
 */
export type TestRunStatus =
  | "pending"
  | "queued"
  | "running"
  | "passed"
  | "failed"
  | "cancelled"
  | "timed_out";

/**
 * Agent status
 */
export type AgentStatus = "online" | "offline" | "busy" | "draining";

/**
 * Execution mode for tests
 */
export type ExecutionMode = "subprocess" | "container";

/**
 * Test run model
 */
export interface TestRun {
  id: string;
  repositoryId: string;
  repositoryName: string;
  branch: string;
  commitSha: string;
  commitMessage?: string;
  status: TestRunStatus;
  executionMode: ExecutionMode;
  agentId?: string;
  agentName?: string;
  startedAt?: string;
  completedAt?: string;
  durationMs?: number;
  totalTests: number;
  passedTests: number;
  failedTests: number;
  skippedTests: number;
  shardCount?: number;
  shardsCompleted?: number;
  shardsFailed?: number;
  maxParallelTests?: number;
  errorMessage?: string;
  createdAt: string;
  updatedAt: string;
}

export interface RunShard {
  id: string;
  runId: string;
  shardIndex: number;
  shardCount: number;
  status: TestRunStatus;
  agentId?: string;
  totalTests: number;
  passedTests: number;
  failedTests: number;
  skippedTests: number;
  errorMessage?: string;
  startedAt?: string;
  finishedAt?: string;
}

/**
 * Test case result
 */
export interface TestCase {
  id: string;
  runId: string;
  name: string;
  suiteName?: string;
  status: "passed" | "failed" | "skipped" | "error";
  durationMs: number;
  errorMessage?: string;
  stackTrace?: string;
  stdout?: string;
  stderr?: string;
}

/**
 * Agent model
 */
export interface Agent {
  id: string;
  name: string;
  status: AgentStatus;
  labels: string[];
  version: string;
  hostname: string;
  ipAddress: string;
  os: string;
  arch: string;
  cpuCores: number;
  memoryMb: number;
  executionModes: ExecutionMode[];
  currentRunId?: string;
  lastHeartbeatAt: string;
  registeredAt: string;
}

/**
 * Repository model
 */
export interface Repository {
  id: string;
  name: string;
  fullName: string;
  provider: "github" | "gitlab" | "bitbucket";
  cloneUrl: string;
  defaultBranch: string;
  testCommand: string;
  testDirectory?: string;
  executionMode: ExecutionMode;
  dockerImage?: string;
  timeout: number;
  labels: string[];
  isActive: boolean;
  createdAt: string;
  updatedAt: string;
}

/**
 * Artifact model
 */
export interface Artifact {
  id: string;
  runId: string;
  name: string;
  contentType: string;
  sizeBytes: number;
  downloadUrl: string;
  createdAt: string;
}

/**
 * Dashboard statistics
 */
export interface DashboardStats {
  totalRuns: number;
  runsTrend: number;
  passRate: number;
  passRateTrend: number;
  activeAgents: number;
  totalAgents: number;
  avgDurationMs: number;
  durationTrend: number;
}

/**
 * Run history data point for charts
 */
export interface RunHistoryPoint {
  date: string;
  total: number;
  passed: number;
  failed: number;
}

/**
 * User model
 */
export interface User {
  id: string;
  email: string;
  name: string;
  avatarUrl?: string;
  role: "admin" | "member" | "viewer";
  createdAt: string;
}
