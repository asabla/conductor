/**
 * Service Details Page
 * Shows comprehensive information about a single service
 */

import { useState } from "react";
import { useParams, Link } from "@tanstack/react-router";
import {
  ArrowLeft,
  GitBranch,
  ExternalLink,
  RefreshCw,
  Play,
  Clock,
  Users,
  Mail,
  MessageSquare,
  Globe,
  CheckCircle,
  XCircle,
  AlertCircle,
  FileCode,
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
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { PassRateChart, type PassRateDataPoint } from "@/components/charts/pass-rate-chart";
import { DurationChart, type DurationDataPoint } from "@/components/charts/duration-chart";
import {
  useService,
  useServiceStats,
  useSyncService,
  type TestDefinition,
  type RecentRunSummary,
} from "@/hooks/use-services";
import { useCreateRun } from "@/hooks/use-runs";
import { formatDuration, formatRelativeTime, cn } from "@/lib/utils";
import type { TestRunStatus } from "@/types/models";

// =============================================================================
// Status Helpers
// =============================================================================

function getRunStatusBadge(status: TestRunStatus, size: "sm" | "default" = "default") {
  const className = size === "sm" ? "text-xs" : "";
  switch (status) {
    case "passed":
      return (
        <Badge variant="success" className={className}>
          <CheckCircle className="mr-1 h-3 w-3" />
          Passed
        </Badge>
      );
    case "failed":
      return (
        <Badge variant="destructive" className={className}>
          <XCircle className="mr-1 h-3 w-3" />
          Failed
        </Badge>
      );
    case "running":
      return (
        <Badge variant="default" className={className}>
          <RefreshCw className="mr-1 h-3 w-3 animate-spin" />
          Running
        </Badge>
      );
    default:
      return (
        <Badge variant="outline" className={cn("capitalize", className)}>
          {status}
        </Badge>
      );
  }
}

function getTestTypeBadge(type: TestDefinition["type"]) {
  const colors: Record<TestDefinition["type"], string> = {
    unit: "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
    integration: "bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200",
    e2e: "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200",
    performance: "bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200",
    security: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200",
    smoke: "bg-gray-100 text-gray-800 dark:bg-gray-900 dark:text-gray-200",
  };

  return (
    <Badge variant="outline" className={cn("capitalize", colors[type])}>
      {type}
    </Badge>
  );
}

// =============================================================================
// Test Definitions Table
// =============================================================================

interface TestDefinitionsTableProps {
  tests: TestDefinition[];
  isLoading?: boolean;
}

function TestDefinitionsTable({ tests, isLoading }: TestDefinitionsTableProps) {
  if (isLoading) {
    return (
      <div className="space-y-2">
        {[1, 2, 3, 4, 5].map((i) => (
          <Skeleton key={i} className="h-12" />
        ))}
      </div>
    );
  }

  if (tests.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12 text-center">
        <FileCode className="h-12 w-12 text-muted-foreground" />
        <h3 className="mt-4 text-lg font-medium">No test definitions</h3>
        <p className="mt-2 text-sm text-muted-foreground">
          Sync this service to discover tests from the repository
        </p>
      </div>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Test Name</TableHead>
          <TableHead>Type</TableHead>
          <TableHead>Tags</TableHead>
          <TableHead>Timeout</TableHead>
          <TableHead>Flakiness</TableHead>
          <TableHead>Status</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {tests.map((test) => (
          <TableRow key={test.id}>
            <TableCell>
              <div>
                <p className="font-medium">{test.name}</p>
                <p className="text-xs text-muted-foreground">{test.path}</p>
              </div>
            </TableCell>
            <TableCell>{getTestTypeBadge(test.type)}</TableCell>
            <TableCell>
              <div className="flex flex-wrap gap-1">
                {test.tags.slice(0, 3).map((tag) => (
                  <Badge key={tag} variant="secondary" className="text-xs">
                    {tag}
                  </Badge>
                ))}
                {test.tags.length > 3 && (
                  <Badge variant="secondary" className="text-xs">
                    +{test.tags.length - 3}
                  </Badge>
                )}
              </div>
            </TableCell>
            <TableCell className="text-muted-foreground">
              {test.timeout ? formatDuration(test.timeout * 1000) : "Default"}
            </TableCell>
            <TableCell>
              {test.flakinessRate !== undefined ? (
                <span
                  className={cn(
                    "text-sm",
                    test.flakinessRate > 0.1
                      ? "text-warning"
                      : "text-muted-foreground"
                  )}
                >
                  {(test.flakinessRate * 100).toFixed(1)}%
                </span>
              ) : (
                <span className="text-muted-foreground">—</span>
              )}
            </TableCell>
            <TableCell>
              {test.enabled ? (
                <Badge variant="success" className="text-xs">
                  Enabled
                </Badge>
              ) : (
                <Badge variant="secondary" className="text-xs">
                  Disabled
                </Badge>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

// =============================================================================
// Recent Runs List
// =============================================================================

interface RecentRunsListProps {
  runs: RecentRunSummary[];
  isLoading?: boolean;
}

function RecentRunsList({ runs, isLoading }: RecentRunsListProps) {
  if (isLoading) {
    return (
      <div className="space-y-2">
        {[1, 2, 3, 4, 5].map((i) => (
          <Skeleton key={i} className="h-16" />
        ))}
      </div>
    );
  }

  if (runs.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12 text-center">
        <Clock className="h-12 w-12 text-muted-foreground" />
        <h3 className="mt-4 text-lg font-medium">No recent runs</h3>
        <p className="mt-2 text-sm text-muted-foreground">
          Trigger a test run to see results here
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {runs.map((run) => (
        <Link
          key={run.id}
          to="/test-runs/$runId"
          params={{ runId: run.id }}
          className="block rounded-lg border p-4 transition-colors hover:bg-muted/50"
        >
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              {getRunStatusBadge(run.status, "sm")}
              <div>
                <div className="flex items-center gap-2">
                  <GitBranch className="h-3 w-3 text-muted-foreground" />
                  <span className="text-sm font-medium">{run.branch}</span>
                  <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                    {run.commitSha.slice(0, 7)}
                  </code>
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  {formatRelativeTime(run.createdAt)}
                </p>
              </div>
            </div>
            <div className="text-right">
              <div className="flex items-center gap-2 text-sm">
                <span className="text-success">{run.passed}</span>
                <span className="text-muted-foreground">/</span>
                <span className="text-destructive">{run.failed}</span>
                <span className="text-muted-foreground">/</span>
                <span className="text-muted-foreground">{run.total}</span>
              </div>
              {run.durationMs && (
                <p className="mt-1 text-xs text-muted-foreground">
                  {formatDuration(run.durationMs)}
                </p>
              )}
            </div>
          </div>
        </Link>
      ))}
    </div>
  );
}

// =============================================================================
// Trigger Run Dialog
// =============================================================================

interface TriggerRunDialogProps {
  serviceId: string;
  serviceName: string;
  defaultBranch: string;
  onTriggered?: () => void;
}

function TriggerRunDialog({
  serviceId,
  serviceName,
  defaultBranch,
  onTriggered,
}: TriggerRunDialogProps) {
  const [open, setOpen] = useState(false);
  const [branch, setBranch] = useState(defaultBranch);
  const createRun = useCreateRun();

  const handleTrigger = async () => {
    try {
      await createRun.mutateAsync({
        serviceId,
        gitRef: { branch },
      });
      setOpen(false);
      onTriggered?.();
    } catch (error) {
      console.error("Failed to trigger run:", error);
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button>
          <Play className="mr-2 h-4 w-4" />
          Trigger Run
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Trigger Test Run</DialogTitle>
          <DialogDescription>
            Start a new test run for {serviceName}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <label className="text-sm font-medium">Branch</label>
            <input
              type="text"
              value={branch}
              onChange={(e) => setBranch(e.target.value)}
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              placeholder={defaultBranch}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button onClick={handleTrigger} disabled={createRun.isPending}>
            {createRun.isPending ? (
              <>
                <RefreshCw className="mr-2 h-4 w-4 animate-spin" />
                Triggering...
              </>
            ) : (
              <>
                <Play className="mr-2 h-4 w-4" />
                Trigger Run
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// =============================================================================
// Main Page Component
// =============================================================================

export function ServiceDetailsPage() {
  const { serviceId } = useParams({ from: "/app/services/$serviceId" });
  const [statsDays, setStatsDays] = useState(30);

  // Fetch service data
  const { data: serviceData, isLoading: serviceLoading } = useService(
    serviceId,
    {
      includeTests: true,
      includeRecentRuns: true,
    }
  );

  // Fetch service stats
  const { data: statsData, isLoading: statsLoading } = useServiceStats(
    serviceId,
    { days: statsDays }
  );

  // Sync mutation
  const syncService = useSyncService();

  const handleSync = () => {
    syncService.mutate({ id: serviceId });
  };

  // Loading state
  if (serviceLoading) {
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
          <Skeleton className="h-96 lg:col-span-2" />
          <Skeleton className="h-96" />
        </div>
      </div>
    );
  }

  if (!serviceData?.service) {
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <AlertCircle className="h-12 w-12 text-destructive" />
        <h2 className="mt-4 text-lg font-medium">Service not found</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          The service you're looking for doesn't exist or has been deleted.
        </p>
        <Button asChild className="mt-4">
          <Link to="/services">Back to Services</Link>
        </Button>
      </div>
    );
  }

  const { service, tests = [], recentRuns = [] } = serviceData;

  // Transform stats for charts
  const passRateData: PassRateDataPoint[] =
    statsData?.passRateTrend?.map((p) => ({
      date: p.date,
      passRate: p.passRate,
      total: p.total,
      passed: p.passed,
      failed: p.failed,
    })) || [];

  const durationData: DurationDataPoint[] =
    statsData?.durationTrend?.map((d) => ({
      date: d.date,
      avgDurationMs: d.avgDurationMs,
      p50DurationMs: d.p50DurationMs,
      p95DurationMs: d.p95DurationMs,
    })) || [];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div className="flex items-start gap-4">
          <Button variant="ghost" size="icon" asChild>
            <Link to="/services">
              <ArrowLeft className="h-4 w-4" />
            </Link>
          </Button>
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-2xl font-bold">{service.name}</h1>
              {service.active ? (
                <Badge variant="success">Active</Badge>
              ) : (
                <Badge variant="secondary">Inactive</Badge>
              )}
            </div>
            <div className="mt-1 flex items-center gap-3 text-sm text-muted-foreground">
              <div className="flex items-center gap-1">
                <Users className="h-4 w-4" />
                {service.owner}
              </div>
              <a
                href={service.gitUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center gap-1 hover:text-primary"
              >
                <GitBranch className="h-4 w-4" />
                {service.defaultBranch}
                <ExternalLink className="h-3 w-3" />
              </a>
            </div>
          </div>
        </div>

        <div className="flex gap-2">
          <Button
            variant="outline"
            onClick={handleSync}
            disabled={syncService.isPending}
          >
            <RefreshCw
              className={cn(
                "mr-2 h-4 w-4",
                syncService.isPending && "animate-spin"
              )}
            />
            Sync Tests
          </Button>
          <TriggerRunDialog
            serviceId={service.id}
            serviceName={service.name}
            defaultBranch={service.defaultBranch}
          />
        </div>
      </div>

      {/* Quick Stats */}
      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Test Definitions
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-bold">{service.testCount}</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Pass Rate ({statsDays}d)
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-bold">
              {statsData?.passRate?.toFixed(1) || "—"}%
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Total Runs ({statsDays}d)
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-bold">{statsData?.totalRuns || 0}</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Avg Duration
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-bold">
              {statsData?.avgDurationMs
                ? formatDuration(statsData.avgDurationMs)
                : "—"}
            </p>
          </CardContent>
        </Card>
      </div>

      {/* Main Content */}
      <div className="grid gap-6 lg:grid-cols-3">
        {/* Left Column - Charts */}
        <div className="space-y-6 lg:col-span-2">
          <PassRateChart
            data={passRateData}
            title="Pass Rate Trend"
            targetPassRate={95}
            isLoading={statsLoading}
            selectedDays={statsDays}
            onDateRangeChange={setStatsDays}
          />
          <DurationChart
            data={durationData}
            title="Duration Trend"
            isLoading={statsLoading}
            selectedDays={statsDays}
            onDateRangeChange={setStatsDays}
          />
        </div>

        {/* Right Column - Service Info */}
        <div className="space-y-6">
          {/* Service Info Card */}
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Service Information</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div>
                <p className="text-xs font-medium text-muted-foreground">
                  Git Repository
                </p>
                <a
                  href={service.gitUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center gap-1 text-sm hover:text-primary"
                >
                  {service.gitUrl}
                  <ExternalLink className="h-3 w-3" />
                </a>
              </div>

              {service.contact && (
                <div>
                  <p className="text-xs font-medium text-muted-foreground">
                    Contact
                  </p>
                  <div className="mt-1 space-y-1">
                    {service.contact.name && (
                      <div className="flex items-center gap-2 text-sm">
                        <Users className="h-3 w-3 text-muted-foreground" />
                        {service.contact.name}
                      </div>
                    )}
                    {service.contact.email && (
                      <div className="flex items-center gap-2 text-sm">
                        <Mail className="h-3 w-3 text-muted-foreground" />
                        <a
                          href={`mailto:${service.contact.email}`}
                          className="hover:text-primary"
                        >
                          {service.contact.email}
                        </a>
                      </div>
                    )}
                    {service.contact.slack && (
                      <div className="flex items-center gap-2 text-sm">
                        <MessageSquare className="h-3 w-3 text-muted-foreground" />
                        {service.contact.slack}
                      </div>
                    )}
                  </div>
                </div>
              )}

              <div>
                <p className="text-xs font-medium text-muted-foreground">
                  Execution Mode
                </p>
                <Badge variant="outline" className="mt-1 capitalize">
                  {service.defaultExecutionType}
                </Badge>
              </div>

              {service.networkZones && service.networkZones.length > 0 && (
                <div>
                  <p className="text-xs font-medium text-muted-foreground">
                    Network Zones
                  </p>
                  <div className="mt-1 flex flex-wrap gap-1">
                    {service.networkZones.map((zone) => (
                      <Badge key={zone} variant="secondary" className="text-xs">
                        <Globe className="mr-1 h-3 w-3" />
                        {zone}
                      </Badge>
                    ))}
                  </div>
                </div>
              )}

              <div>
                <p className="text-xs font-medium text-muted-foreground">
                  Last Synced
                </p>
                <p className="text-sm">
                  {service.lastSyncedAt
                    ? formatRelativeTime(service.lastSyncedAt)
                    : "Never"}
                </p>
              </div>
            </CardContent>
          </Card>

          {/* Recent Runs Card */}
          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-3">
              <CardTitle className="text-base">Recent Runs</CardTitle>
              <Link
                to="/test-runs"
                className="text-xs text-muted-foreground hover:text-primary"
              >
                View all
              </Link>
            </CardHeader>
            <CardContent>
              <RecentRunsList runs={recentRuns.slice(0, 5)} />
            </CardContent>
          </Card>
        </div>
      </div>

      {/* Test Definitions */}
      <Card>
        <CardHeader>
          <CardTitle>Test Definitions</CardTitle>
          <CardDescription>
            {tests.length} test{tests.length !== 1 ? "s" : ""} defined for this
            service
          </CardDescription>
        </CardHeader>
        <CardContent>
          <TestDefinitionsTable tests={tests} />
        </CardContent>
      </Card>
    </div>
  );
}

export default ServiceDetailsPage;
