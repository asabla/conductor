/**
 * Flaky Tests page
 * Shows tests that exhibit flaky behavior with management controls
 */

import { useState, useMemo } from "react";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import { format, parseISO } from "date-fns";
import {
  AlertTriangle,
  Filter,
  RefreshCw,
  ShieldAlert,
  ShieldCheck,
} from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { toast } from "@/components/ui/toast";
import {
  Table,
  TableBody,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FlakyTestRow } from "@/components/analytics/flaky-test-row";
import {
  useFlakyTests,
  useFlakyTestHistory,
  useQuarantineTest,
  useUnquarantineTest,
} from "@/hooks/use-analytics";
import { useServices } from "@/hooks/use-services";
import type { FlakyTestsParams } from "@/api/analytics";

// =============================================================================
// Types
// =============================================================================

type QuarantineFilter = "all" | "active" | "quarantined";
type SortBy = "flakinessScore" | "lastFlakyDate" | "totalRuns";

// =============================================================================
// Component
// =============================================================================

export function FlakyTestsPage() {
  // Filter state
  const [selectedService, setSelectedService] = useState<string | undefined>();
  const [quarantineFilter, setQuarantineFilter] = useState<QuarantineFilter>("all");
  const [sortBy, setSortBy] = useState<SortBy>("flakinessScore");

  // History dialog state
  const [selectedTestId, setSelectedTestId] = useState<string | null>(null);
  const [historyDialogOpen, setHistoryDialogOpen] = useState(false);
  const [quarantineDialogOpen, setQuarantineDialogOpen] = useState(false);
  const [quarantineTargetId, setQuarantineTargetId] = useState<string | null>(null);
  const [quarantineAction, setQuarantineAction] = useState<"quarantine" | "unquarantine">(
    "quarantine"
  );
  const [quarantineReason, setQuarantineReason] = useState("");

  // Build API params
  const params: FlakyTestsParams = useMemo(
    () => ({
      serviceId: selectedService,
      showQuarantined: quarantineFilter === "all" ? undefined : quarantineFilter === "quarantined",
      sortBy,
      sortOrder: "desc",
      pageSize: 100,
    }),
    [selectedService, quarantineFilter, sortBy]
  );

  // Fetch data
  const {
    data: flakyData,
    isLoading,
    isError,
    error,
    refetch,
    isFetching,
  } = useFlakyTests(params);

  const { data: servicesData } = useServices({});

  // History data for selected test
  const { data: historyData, isLoading: isHistoryLoading } = useFlakyTestHistory(
    selectedTestId ?? undefined,
    30,
    { enabled: !!selectedTestId && historyDialogOpen }
  );

  // Mutations
  const quarantineMutation = useQuarantineTest();
  const unquarantineMutation = useUnquarantineTest();

  // Filter tests based on quarantine filter
  const filteredTests = useMemo(() => {
    if (!flakyData?.tests) return [];
    if (quarantineFilter === "all") return flakyData.tests;
    if (quarantineFilter === "quarantined") {
      return flakyData.tests.filter((t) => t.isQuarantined);
    }
    return flakyData.tests.filter((t) => !t.isQuarantined);
  }, [flakyData?.tests, quarantineFilter]);

  // Calculate summary stats
  const summaryStats = useMemo(() => {
    if (!flakyData?.tests) {
      return { total: 0, quarantined: 0, highFlakiness: 0, avgFlakiness: 0 };
    }

    const tests = flakyData.tests;
    const quarantined = tests.filter((t) => t.isQuarantined).length;
    const highFlakiness = tests.filter((t) => t.flakinessScore >= 25).length;
    const avgFlakiness =
      tests.length > 0
        ? tests.reduce((sum, t) => sum + t.flakinessScore, 0) / tests.length
        : 0;

    return {
      total: tests.length,
      quarantined,
      highFlakiness,
      avgFlakiness,
    };
  }, [flakyData?.tests]);

  // Get selected test for history dialog
  const selectedTest = useMemo(
    () => flakyData?.tests.find((t) => t.testId === selectedTestId),
    [flakyData?.tests, selectedTestId]
  );

  const quarantineTarget = useMemo(
    () => flakyData?.tests.find((t) => t.testId === quarantineTargetId),
    [flakyData?.tests, quarantineTargetId]
  );

  // Handlers
  const openQuarantineDialog = (
    testId: string,
    action: "quarantine" | "unquarantine"
  ) => {
    setQuarantineTargetId(testId);
    setQuarantineAction(action);
    setQuarantineReason("");
    setQuarantineDialogOpen(true);
  };

  const handleQuarantine = (testId: string) => {
    openQuarantineDialog(testId, "quarantine");
  };

  const handleUnquarantine = (testId: string) => {
    openQuarantineDialog(testId, "unquarantine");
  };

  const handleViewHistory = (testId: string) => {
    setSelectedTestId(testId);
    setHistoryDialogOpen(true);
  };

  const handleConfirmQuarantine = () => {
    if (!quarantineTargetId) return;

    const targetName = quarantineTarget?.testName ?? "Test";

    if (quarantineAction === "quarantine") {
      quarantineMutation.mutate(
        {
          testId: quarantineTargetId,
          reason: quarantineReason.trim() || undefined,
        },
        {
          onSuccess: () => {
            setQuarantineDialogOpen(false);
            toast({
              title: "Test quarantined",
              description: `${targetName} is now quarantined.`,
              variant: "success",
            });
          },
          onError: (err) => {
            toast({
              title: "Failed to quarantine",
              description: err instanceof Error ? err.message : "Please try again.",
              variant: "destructive",
            });
          },
        }
      );
      return;
    }

    unquarantineMutation.mutate(quarantineTargetId, {
      onSuccess: () => {
        setQuarantineDialogOpen(false);
        toast({
          title: "Quarantine removed",
          description: `${targetName} is active again.`,
          variant: "success",
        });
      },
      onError: (err) => {
        toast({
          title: "Failed to remove quarantine",
          description: err instanceof Error ? err.message : "Please try again.",
          variant: "destructive",
        });
      },
    });
  };

  const handleCloseQuarantineDialog = (open: boolean) => {
    setQuarantineDialogOpen(open);
    if (!open) {
      setQuarantineTargetId(null);
      setQuarantineReason("");
    }
  };

  // Format date for chart
  const formatDate = (dateStr: string) => {
    try {
      return format(parseISO(dateStr), "MMM d");
    } catch {
      return dateStr;
    }
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Flaky Tests</h1>
          <p className="text-muted-foreground">
            Identify and manage tests that show inconsistent behavior
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => refetch()}
          disabled={isFetching}
        >
          <RefreshCw className={`mr-2 h-4 w-4 ${isFetching ? "animate-spin" : ""}`} />
          Refresh
        </Button>
      </div>

      {/* Summary Stats */}
      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Total Flaky Tests
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {isLoading ? <Skeleton className="h-8 w-16" /> : summaryStats.total}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Quarantined
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex items-baseline gap-2">
              <span className="text-2xl font-bold">
                {isLoading ? <Skeleton className="h-8 w-12" /> : summaryStats.quarantined}
              </span>
              <ShieldAlert className="h-4 w-4 text-warning" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              High Flakiness ({">"}25%)
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-destructive">
              {isLoading ? <Skeleton className="h-8 w-12" /> : summaryStats.highFlakiness}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Avg Flakiness Score
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {isLoading ? (
                <Skeleton className="h-8 w-16" />
              ) : (
                `${summaryStats.avgFlakiness.toFixed(1)}%`
              )}
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Filters */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-base">
            <Filter className="h-4 w-4" />
            Filters
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap items-center gap-4">
            {/* Service filter */}
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">Service:</span>
              <div className="flex gap-1 flex-wrap">
                <Button
                  variant={!selectedService ? "default" : "outline"}
                  size="sm"
                  onClick={() => setSelectedService(undefined)}
                >
                  All
                </Button>
                {servicesData?.items.slice(0, 5).map((service) => (
                  <Button
                    key={service.id}
                    variant={selectedService === service.id ? "default" : "outline"}
                    size="sm"
                    onClick={() => setSelectedService(service.id)}
                  >
                    {service.name}
                  </Button>
                ))}
              </div>
            </div>

            {/* Quarantine filter */}
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">Status:</span>
              <div className="flex gap-1">
                <Button
                  variant={quarantineFilter === "all" ? "default" : "outline"}
                  size="sm"
                  onClick={() => setQuarantineFilter("all")}
                >
                  All
                </Button>
                <Button
                  variant={quarantineFilter === "active" ? "default" : "outline"}
                  size="sm"
                  onClick={() => setQuarantineFilter("active")}
                >
                  <AlertTriangle className="mr-1 h-3 w-3" />
                  Active
                </Button>
                <Button
                  variant={quarantineFilter === "quarantined" ? "default" : "outline"}
                  size="sm"
                  onClick={() => setQuarantineFilter("quarantined")}
                >
                  <ShieldAlert className="mr-1 h-3 w-3" />
                  Quarantined
                </Button>
              </div>
            </div>

            {/* Sort by */}
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">Sort by:</span>
              <div className="flex gap-1">
                <Button
                  variant={sortBy === "flakinessScore" ? "default" : "outline"}
                  size="sm"
                  onClick={() => setSortBy("flakinessScore")}
                >
                  Flakiness
                </Button>
                <Button
                  variant={sortBy === "lastFlakyDate" ? "default" : "outline"}
                  size="sm"
                  onClick={() => setSortBy("lastFlakyDate")}
                >
                  Recent
                </Button>
                <Button
                  variant={sortBy === "totalRuns" ? "default" : "outline"}
                  size="sm"
                  onClick={() => setSortBy("totalRuns")}
                >
                  Runs
                </Button>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Error State */}
      {isError && (
        <Card className="border-destructive">
          <CardContent className="flex items-center gap-4 py-6">
            <AlertTriangle className="h-8 w-8 text-destructive" />
            <div>
              <h3 className="font-semibold">Failed to load flaky tests</h3>
              <p className="text-sm text-muted-foreground">
                {error instanceof Error ? error.message : "An unknown error occurred"}
              </p>
            </div>
            <Button variant="outline" onClick={() => refetch()} className="ml-auto">
              Retry
            </Button>
          </CardContent>
        </Card>
      )}

      {/* Loading State */}
      {isLoading && (
        <Card>
          <CardContent className="py-6">
            <div className="space-y-4">
              {[1, 2, 3, 4, 5].map((i) => (
                <div key={i} className="flex items-center gap-4">
                  <Skeleton className="h-12 w-full" />
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Flaky Tests Table */}
      {!isLoading && !isError && (
        <>
          {filteredTests.length === 0 ? (
            <Card>
              <CardContent className="flex flex-col items-center justify-center py-12">
                <ShieldCheck className="h-12 w-12 text-success mb-4" />
                <CardTitle className="text-lg mb-2">No flaky tests found</CardTitle>
                <CardDescription>
                  {quarantineFilter !== "all"
                    ? `No ${quarantineFilter} flaky tests`
                    : "Your test suite is looking healthy!"}
                </CardDescription>
              </CardContent>
            </Card>
          ) : (
            <Card>
              <CardHeader className="pb-3">
                <div className="flex items-center justify-between">
                  <CardTitle className="text-base">
                    Flaky Tests
                    <Badge variant="secondary" className="ml-2">
                      {filteredTests.length}
                    </Badge>
                  </CardTitle>
                </div>
              </CardHeader>
              <CardContent className="p-0">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-[40%]">Test</TableHead>
                      <TableHead>Flakiness</TableHead>
                      <TableHead>Runs</TableHead>
                      <TableHead>Last Flaky</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead className="w-[50px]" />
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredTests.map((test) => (
                      <FlakyTestRow
                        key={test.id}
                        test={test}
                        onQuarantine={handleQuarantine}
                        onUnquarantine={handleUnquarantine}
                        onViewHistory={handleViewHistory}
                        isLoading={
                          quarantineMutation.isPending || unquarantineMutation.isPending
                        }
                        isSelected={selectedTestId === test.testId}
                        onClick={() => setSelectedTestId(test.testId)}
                      />
                    ))}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
          )}
        </>
      )}

       {/* Quarantine Dialog */}
       <Dialog open={quarantineDialogOpen} onOpenChange={handleCloseQuarantineDialog}>
         <DialogContent className="max-w-lg">
           <DialogHeader>
             <DialogTitle>
               {quarantineAction === "quarantine" ? "Quarantine Test" : "Remove Quarantine"}
             </DialogTitle>
             <DialogDescription>
               {quarantineTarget
                 ? `Test: ${quarantineTarget.testName}`
                 : "Choose a test to update its quarantine status."}
             </DialogDescription>
           </DialogHeader>

           <div className="space-y-4">
             {quarantineAction === "quarantine" && (
               <div className="space-y-2">
                 <span className="text-sm font-medium">Reason (optional)</span>
                 <Input
                   value={quarantineReason}
                   onChange={(event) => setQuarantineReason(event.target.value)}
                   placeholder="e.g. Flaky in CI under load"
                 />
               </div>
             )}

             <div className="flex justify-end gap-2">
               <Button variant="outline" onClick={() => handleCloseQuarantineDialog(false)}>
                 Cancel
               </Button>
               <Button
                 variant={quarantineAction === "quarantine" ? "default" : "outline"}
                 onClick={handleConfirmQuarantine}
                 disabled={quarantineMutation.isPending || unquarantineMutation.isPending}
               >
                 {quarantineAction === "quarantine" ? "Confirm Quarantine" : "Remove Quarantine"}
               </Button>
             </div>
           </div>
         </DialogContent>
       </Dialog>

       {/* History Dialog */}
       <Dialog open={historyDialogOpen} onOpenChange={setHistoryDialogOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              Flakiness History
              {selectedTest && (
                <Badge variant="outline">{selectedTest.serviceName}</Badge>
              )}
            </DialogTitle>
            <DialogDescription>
              {selectedTest?.testName}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            {isHistoryLoading ? (
              <div className="h-64 flex items-center justify-center">
                <Skeleton className="h-full w-full" />
              </div>
            ) : historyData?.history && historyData.history.length > 0 ? (
              <div className="h-64">
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart data={historyData.history}>
                    <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
                    <XAxis
                      dataKey="date"
                      tickFormatter={formatDate}
                      tick={{ fontSize: 12 }}
                      tickLine={false}
                      axisLine={false}
                    />
                    <YAxis
                      domain={[0, 100]}
                      tick={{ fontSize: 12 }}
                      tickLine={false}
                      axisLine={false}
                      tickFormatter={(v) => `${v}%`}
                      width={45}
                    />
                    <Tooltip
                      content={({ active, payload, label }) => {
                        if (!active || !payload?.[0]) return null;
                        const data = payload[0].payload as {
                          flakinessScore: number;
                          flakyRuns: number;
                          totalRuns: number;
                        };
                        return (
                          <div className="rounded-lg border bg-background p-3 shadow-md">
                            <p className="mb-1 font-medium">{formatDate(String(label ?? ""))}</p>
                            <p className="text-sm">
                              Flakiness: <span className="font-medium">{data.flakinessScore.toFixed(1)}%</span>
                            </p>
                            <p className="text-xs text-muted-foreground">
                              {data.flakyRuns}/{data.totalRuns} flaky runs
                            </p>
                          </div>
                        );
                      }}
                    />
                    <Line
                      type="monotone"
                      dataKey="flakinessScore"
                      stroke="hsl(var(--warning))"
                      strokeWidth={2}
                      dot={false}
                      activeDot={{ r: 4 }}
                    />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            ) : (
              <div className="h-64 flex items-center justify-center text-muted-foreground">
                No history data available
              </div>
            )}

            {/* Current stats */}
            {selectedTest && (
              <div className="grid grid-cols-3 gap-4 pt-4 border-t">
                <div>
                  <p className="text-sm text-muted-foreground">Current Flakiness</p>
                  <p className="text-lg font-bold">{selectedTest.flakinessScore.toFixed(1)}%</p>
                </div>
                <div>
                  <p className="text-sm text-muted-foreground">Total Runs</p>
                  <p className="text-lg font-bold">{selectedTest.totalRuns}</p>
                </div>
                <div>
                  <p className="text-sm text-muted-foreground">Flaky Runs</p>
                  <p className="text-lg font-bold">{selectedTest.flakyRuns}</p>
                </div>
              </div>
            )}

            {/* Actions */}
            <div className="flex justify-end gap-2 pt-4 border-t">
              <Button variant="outline" onClick={() => setHistoryDialogOpen(false)}>
                Close
              </Button>
              {selectedTest && (
                <Button
                  variant={selectedTest.isQuarantined ? "outline" : "default"}
                  onClick={() => {
                    const action = selectedTest.isQuarantined
                      ? "unquarantine"
                      : "quarantine";
                    setHistoryDialogOpen(false);
                    openQuarantineDialog(selectedTest.testId, action);
                  }}
                  disabled={quarantineMutation.isPending || unquarantineMutation.isPending}
                >
                  {selectedTest.isQuarantined ? (
                    <>
                      <ShieldCheck className="mr-2 h-4 w-4" />
                      Remove Quarantine
                    </>
                  ) : (
                    <>
                      <ShieldAlert className="mr-2 h-4 w-4" />
                      Quarantine Test
                    </>
                  )}
                </Button>
              )}
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

export default FlakyTestsPage;
