/**
 * Test Run Details Page
 * Shows comprehensive information about a single test run
 */

import { useState } from "react";
import { useParams, Link } from "@tanstack/react-router";
import {
  ArrowLeft,
  GitBranch,
  GitCommit,
  Clock,
  Play,
  XCircle,
  RefreshCw,
  Calendar,
  Download,
  CheckCircle,
  AlertCircle,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import { LogViewer } from "@/components/runs/log-viewer";
import { TestResultsTable, type TestResultRow } from "@/components/runs/test-results-table";
import {
  useRun,
  useRunResults,
  useCancelRun,
  useRetryRun,
  useRunArtifacts,
  useRunLogs,
} from "@/hooks/use-runs";
import { useRunStatus } from "@/hooks/use-websocket";
import { formatDuration, formatRelativeTime, cn } from "@/lib/utils";
import type { TestRunStatus, Artifact } from "@/types/models";

// =============================================================================
// Status Badge Component
// =============================================================================

function getStatusBadge(status: TestRunStatus) {
  switch (status) {
    case "passed":
      return (
        <Badge variant="success" className="gap-1">
          <CheckCircle className="h-3 w-3" />
          Passed
        </Badge>
      );
    case "failed":
      return (
        <Badge variant="destructive" className="gap-1">
          <XCircle className="h-3 w-3" />
          Failed
        </Badge>
      );
    case "running":
      return (
        <Badge variant="default" className="gap-1">
          <RefreshCw className="h-3 w-3 animate-spin" />
          Running
        </Badge>
      );
    case "pending":
    case "queued":
      return (
        <Badge variant="outline" className="gap-1">
          <Clock className="h-3 w-3" />
          {status === "pending" ? "Pending" : "Queued"}
        </Badge>
      );
    case "cancelled":
      return (
        <Badge variant="secondary" className="gap-1">
          <XCircle className="h-3 w-3" />
          Cancelled
        </Badge>
      );
    case "timed_out":
      return (
        <Badge variant="warning" className="gap-1">
          <AlertCircle className="h-3 w-3" />
          Timed Out
        </Badge>
      );
    default:
      return <Badge variant="outline">{status}</Badge>;
  }
}

// =============================================================================
// Timeline Component
// =============================================================================

interface TimelineEvent {
  id: string;
  label: string;
  timestamp: string;
  icon: typeof Clock;
  status: "completed" | "current" | "pending";
}

function RunTimeline({ events }: { events: TimelineEvent[] }) {
  return (
    <div className="relative space-y-4 pl-6">
      {/* Vertical line */}
      <div className="absolute left-[11px] top-2 h-[calc(100%-24px)] w-0.5 bg-border" />

      {events.map((event) => {
        const Icon = event.icon;
        return (
          <div key={event.id} className="relative flex items-start gap-3">
            <div
              className={cn(
                "absolute -left-6 flex h-6 w-6 items-center justify-center rounded-full border-2 bg-background",
                event.status === "completed" && "border-success bg-success/10",
                event.status === "current" && "border-primary bg-primary/10",
                event.status === "pending" && "border-muted"
              )}
            >
              <Icon
                className={cn(
                  "h-3 w-3",
                  event.status === "completed" && "text-success",
                  event.status === "current" && "text-primary animate-pulse",
                  event.status === "pending" && "text-muted-foreground"
                )}
              />
            </div>
            <div className="flex-1 pt-0.5">
              <p className="text-sm font-medium">{event.label}</p>
              <p className="text-xs text-muted-foreground">
                {event.timestamp ? formatRelativeTime(event.timestamp) : "Pending"}
              </p>
            </div>
          </div>
        );
      })}
    </div>
  );
}

// =============================================================================
// Artifacts List Component
// =============================================================================

interface ArtifactsListProps {
  artifacts: Artifact[];
  isLoading?: boolean;
}

function ArtifactsList({ artifacts, isLoading }: ArtifactsListProps) {
  if (isLoading) {
    return (
      <div className="space-y-2">
        {[1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-14" />
        ))}
      </div>
    );
  }

  if (artifacts.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-8 text-center">
        <Download className="h-8 w-8 text-muted-foreground" />
        <p className="mt-2 text-sm text-muted-foreground">No artifacts available</p>
      </div>
    );
  }

  const formatSize = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  return (
    <div className="space-y-2">
      {artifacts.map((artifact) => (
        <div
          key={artifact.id}
          className="flex items-center justify-between rounded-lg border p-3"
        >
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded bg-muted">
              <Download className="h-5 w-5 text-muted-foreground" />
            </div>
            <div>
              <p className="font-medium">{artifact.name}</p>
              <p className="text-xs text-muted-foreground">
                {formatSize(artifact.sizeBytes)} · {artifact.contentType}
              </p>
            </div>
          </div>
          <Button variant="ghost" size="sm" asChild>
            <a href={artifact.downloadUrl} download>
              <Download className="mr-2 h-4 w-4" />
              Download
            </a>
          </Button>
        </div>
      ))}
    </div>
  );
}

// =============================================================================
// Main Page Component
// =============================================================================

export function RunDetailsPage() {
  // Get run ID from URL params
  const { runId } = useParams({ from: "/app/test-runs/$runId" });
  const [activeTab, setActiveTab] = useState("results");

  // Fetch run data
  const { data: runData, isLoading: runLoading, error: runError } = useRun(
    runId,
    {
      includeResults: false,
      includeArtifacts: false,
      refetchInterval: 5000,
    }
  );

  // Fetch test results
  const { data: resultsData, isLoading: resultsLoading } = useRunResults(
    runId,
    undefined,
    { enabled: !!runId }
  );

  // Fetch artifacts
  const { data: artifactsData, isLoading: artifactsLoading } = useRunArtifacts(
    runId,
    { enabled: !!runId }
  );

  // Fetch logs
  const { data: logsData } = useRunLogs(runId, {
    enabled: !!runId,
  });

  // Real-time status updates via WebSocket
  const { runStatus: liveStatus } = useRunStatus(runId, {
    enabled: runData?.status === "running",
  });

  // Mutations
  const cancelRun = useCancelRun();
  const retryRun = useRetryRun();

  // Determine current status (prefer live if available)
  const currentStatus = liveStatus?.status || runData?.status;
  const isInProgress =
    currentStatus === "running" ||
    currentStatus === "pending" ||
    currentStatus === "queued";

  // Build timeline events
  const timelineEvents: TimelineEvent[] = runData
    ? [
        {
          id: "created",
          label: "Run Created",
          timestamp: runData.createdAt,
          icon: Calendar,
          status: "completed",
        },
        {
          id: "started",
          label: "Execution Started",
          timestamp: runData.startedAt || "",
          icon: Play,
          status: runData.startedAt ? "completed" : "pending",
        },
        {
          id: "completed",
          label: `Run ${runData.status === "running" ? "In Progress" : "Completed"}`,
          timestamp: runData.completedAt || "",
          icon: runData.status === "passed" ? CheckCircle : Clock,
          status:
            runData.status === "running"
              ? "current"
              : runData.completedAt
              ? "completed"
              : "pending",
        },
      ]
    : [];

  // Transform results for table
  const testResults: TestResultRow[] =
    resultsData?.items?.map((r) => ({
      id: r.id,
      testName: r.testName,
      testPath: r.testPath,
      suiteName: r.suiteName,
      status: r.status,
      durationMs: r.durationMs,
      errorMessage: r.errorMessage,
      stackTrace: r.stackTrace,
      stdout: r.stdout,
      stderr: r.stderr,
      retryAttempt: r.retryAttempt,
    })) || [];

  // Loading state
  if (runLoading) {
    return (
      <div className="space-y-6">
        <div className="flex items-center gap-4">
          <Skeleton className="h-10 w-10" />
          <div className="space-y-2">
            <Skeleton className="h-6 w-64" />
            <Skeleton className="h-4 w-40" />
          </div>
        </div>
        <div className="grid gap-6 lg:grid-cols-3">
          <Skeleton className="h-48 lg:col-span-2" />
          <Skeleton className="h-48" />
        </div>
        <Skeleton className="h-96" />
      </div>
    );
  }

  // Error state
  if (runError || !runData) {
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <AlertCircle className="h-12 w-12 text-destructive" />
        <h2 className="mt-4 text-lg font-medium">Run not found</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          The test run you're looking for doesn't exist or has been deleted.
        </p>
        <Button asChild className="mt-4">
          <Link to="/test-runs">Back to Test Runs</Link>
        </Button>
      </div>
    );
  }

  const run = runData;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div className="flex items-start gap-4">
          <Button variant="ghost" size="icon" asChild>
            <Link to="/test-runs">
              <ArrowLeft className="h-4 w-4" />
            </Link>
          </Button>
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-2xl font-bold">{run.repositoryName}</h1>
              {getStatusBadge(run.status)}
            </div>
            <div className="mt-1 flex items-center gap-3 text-sm text-muted-foreground">
              <div className="flex items-center gap-1">
                <GitBranch className="h-4 w-4" />
                {run.branch}
              </div>
              <div className="flex items-center gap-1">
                <GitCommit className="h-4 w-4" />
                <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                  {run.commitSha.slice(0, 7)}
                </code>
              </div>
              {run.durationMs && (
                <div className="flex items-center gap-1">
                  <Clock className="h-4 w-4" />
                  {formatDuration(run.durationMs)}
                </div>
              )}
            </div>
          </div>
        </div>

        <div className="flex gap-2">
          {isInProgress && (
            <Button
              variant="destructive"
              onClick={() => cancelRun.mutate({ runId })}
              disabled={cancelRun.isPending}
            >
              <XCircle className="mr-2 h-4 w-4" />
              Cancel Run
            </Button>
          )}
          {!isInProgress && (
            <Button
              variant="outline"
              onClick={() => retryRun.mutate({ runId })}
              disabled={retryRun.isPending}
            >
              <RefreshCw className="mr-2 h-4 w-4" />
              Retry Run
            </Button>
          )}
        </div>
      </div>

      {/* Summary Cards */}
      <div className="grid gap-6 lg:grid-cols-3">
        {/* Test Results Summary */}
        <Card className="lg:col-span-2">
          <CardHeader className="pb-3">
            <CardTitle className="text-base">Test Results</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-4 gap-4">
              <div className="text-center">
                <p className="text-3xl font-bold">{run.totalTests}</p>
                <p className="text-xs text-muted-foreground">Total</p>
              </div>
              <div className="text-center">
                <p className="text-3xl font-bold text-success">{run.passedTests}</p>
                <p className="text-xs text-muted-foreground">Passed</p>
              </div>
              <div className="text-center">
                <p className="text-3xl font-bold text-destructive">{run.failedTests}</p>
                <p className="text-xs text-muted-foreground">Failed</p>
              </div>
              <div className="text-center">
                <p className="text-3xl font-bold text-muted-foreground">
                  {run.skippedTests}
                </p>
                <p className="text-xs text-muted-foreground">Skipped</p>
              </div>
            </div>

            {run.totalTests > 0 && (
              <div className="mt-4">
                <div className="flex h-2 overflow-hidden rounded-full bg-muted">
                  <div
                    className="bg-success"
                    style={{ width: `${(run.passedTests / run.totalTests) * 100}%` }}
                  />
                  <div
                    className="bg-destructive"
                    style={{ width: `${(run.failedTests / run.totalTests) * 100}%` }}
                  />
                  <div
                    className="bg-muted-foreground/50"
                    style={{ width: `${(run.skippedTests / run.totalTests) * 100}%` }}
                  />
                </div>
              </div>
            )}
          </CardContent>
        </Card>

        {/* Run Info */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-base">Run Information</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-sm text-muted-foreground">Agent</span>
              <span className="text-sm font-medium">{run.agentName || "—"}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm text-muted-foreground">Mode</span>
              <Badge variant="outline" className="capitalize">
                {run.executionMode}
              </Badge>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm text-muted-foreground">Started</span>
              <span className="text-sm">
                {run.startedAt ? formatRelativeTime(run.startedAt) : "—"}
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm text-muted-foreground">Duration</span>
              <span className="text-sm">
                {run.durationMs ? formatDuration(run.durationMs) : "—"}
              </span>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Error Message */}
      {run.errorMessage && (
        <Card className="border-destructive/50 bg-destructive/5">
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-base text-destructive">
              <AlertCircle className="h-4 w-4" />
              Error
            </CardTitle>
          </CardHeader>
          <CardContent>
            <pre className="whitespace-pre-wrap text-sm text-destructive">
              {run.errorMessage}
            </pre>
          </CardContent>
        </Card>
      )}

      {/* Tabs */}
      <Tabs value={activeTab} onValueChange={setActiveTab}>
        <TabsList>
          <TabsTrigger value="results">
            Test Results
            {run.failedTests > 0 && (
              <Badge variant="destructive" className="ml-2">
                {run.failedTests}
              </Badge>
            )}
          </TabsTrigger>
          <TabsTrigger value="logs">Logs</TabsTrigger>
          <TabsTrigger value="artifacts">
            Artifacts
            {artifactsData?.items && artifactsData.items.length > 0 && (
              <Badge variant="secondary" className="ml-2">
                {artifactsData.items.length}
              </Badge>
            )}
          </TabsTrigger>
          <TabsTrigger value="timeline">Timeline</TabsTrigger>
        </TabsList>

        <TabsContent value="results" className="mt-4">
          <TestResultsTable
            results={testResults}
            isLoading={resultsLoading}
            maxHeight={500}
          />
        </TabsContent>

        <TabsContent value="logs" className="mt-4">
          <LogViewer
            runId={runId}
            logs={logsData?.entries}
            isLive={run.status === "running"}
            height={500}
          />
        </TabsContent>

        <TabsContent value="artifacts" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Artifacts</CardTitle>
              <CardDescription>
                Files generated during test execution
              </CardDescription>
            </CardHeader>
            <CardContent>
              <ArtifactsList
                artifacts={artifactsData?.items || []}
                isLoading={artifactsLoading}
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="timeline" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Execution Timeline</CardTitle>
              <CardDescription>Track the progress of this test run</CardDescription>
            </CardHeader>
            <CardContent>
              <RunTimeline events={timelineEvents} />
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}

export default RunDetailsPage;
