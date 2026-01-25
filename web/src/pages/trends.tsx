/**
 * Trends and Analytics page
 * Comprehensive view of test metrics and trends over time
 */

import { useState, useMemo } from "react";
import { Link } from "@tanstack/react-router";
import {
  LineChart,
  Line,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from "recharts";
import { format, parseISO } from "date-fns";
import {
  TrendingUp,
  Clock,
  AlertCircle,
  Activity,
  ExternalLink,
  Calendar,
  ArrowUpRight,
  ArrowDownRight,
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { TrendCard } from "@/components/analytics/trend-card";
import {
  usePassRateTrend,
  useDurationTrend,
  useDailyStats,
  useTopFailingTests,
  useTopSlowestTests,
  useServiceComparison,
} from "@/hooks/use-analytics";
import { useServices } from "@/hooks/use-services";
import { formatDuration } from "@/lib/utils";
import { cn } from "@/lib/utils";

// =============================================================================
// Types
// =============================================================================

type DateRange = 7 | 14 | 30 | 90;

// =============================================================================
// Constants
// =============================================================================

const DATE_RANGES: { value: DateRange; label: string }[] = [
  { value: 7, label: "7 days" },
  { value: 14, label: "14 days" },
  { value: 30, label: "30 days" },
  { value: 90, label: "90 days" },
];

// =============================================================================
// Helpers
// =============================================================================

const formatDate = (dateStr: string) => {
  try {
    return format(parseISO(dateStr), "MMM d");
  } catch {
    return dateStr;
  }
};

const formatShortDate = (dateStr: string) => {
  try {
    return format(parseISO(dateStr), "M/d");
  } catch {
    return dateStr;
  }
};

// =============================================================================
// Component
// =============================================================================

export function TrendsPage() {
  // State
  const [selectedDays, setSelectedDays] = useState<DateRange>(30);
  const [selectedService, setSelectedService] = useState<string | undefined>();

  // Fetch data
  const { data: passRateData, isLoading: isPassRateLoading } = usePassRateTrend(
    selectedService,
    selectedDays
  );

  const { data: durationData, isLoading: isDurationLoading } = useDurationTrend(
    selectedService,
    selectedDays
  );

  const { data: dailyStats, isLoading: isDailyStatsLoading } = useDailyStats(
    selectedService,
    selectedDays
  );

  const { data: topFailingData, isLoading: isTopFailingLoading } = useTopFailingTests(
    10,
    selectedService
  );

  const { data: topSlowestData, isLoading: isTopSlowestLoading } = useTopSlowestTests(
    10,
    selectedService
  );

  const { data: serviceComparisonData, isLoading: isServiceComparisonLoading } =
    useServiceComparison(selectedDays);

  const { data: servicesData } = useServices({});

  // Calculate summary metrics
  const summaryMetrics = useMemo(() => {
    if (!passRateData?.data || passRateData.data.length < 2) {
      return {
        currentPassRate: 0,
        passRateChange: 0,
        totalTests: 0,
        testsChange: 0,
      };
    }

    const data = passRateData.data;
    const current = data[data.length - 1];
    const previous = data[Math.max(0, data.length - 8)]; // Compare to ~week ago

    if (!current || !previous) {
      return {
        currentPassRate: 0,
        passRateChange: 0,
        totalTests: 0,
        testsChange: 0,
      };
    }

    const currentPassRate = current.passRate;
    const passRateChange = currentPassRate - previous.passRate;
    const totalTests = current.total;
    const testsChange = previous.total > 0 
      ? ((current.total - previous.total) / previous.total) * 100 
      : 0;

    return {
      currentPassRate,
      passRateChange,
      totalTests,
      testsChange,
    };
  }, [passRateData]);

  const durationMetrics = useMemo(() => {
    if (!durationData?.data || durationData.data.length < 2) {
      return { avgDuration: 0, durationChange: 0 };
    }

    const data = durationData.data;
    const current = data[data.length - 1];
    const previous = data[Math.max(0, data.length - 8)];

    if (!current || !previous) {
      return { avgDuration: 0, durationChange: 0 };
    }

    const avgDuration = current.avgDurationMs;
    const durationChange = previous.avgDurationMs > 0
      ? ((current.avgDurationMs - previous.avgDurationMs) / previous.avgDurationMs) * 100
      : 0;

    return { avgDuration, durationChange };
  }, [durationData]);

  // Prepare sparkline data for trend cards
  const passRateSparkline = useMemo(
    () =>
      passRateData?.data.map((d) => ({
        date: d.date,
        value: d.passRate,
      })) ?? [],
    [passRateData]
  );

  const durationSparkline = useMemo(
    () =>
      durationData?.data.map((d) => ({
        date: d.date,
        value: d.avgDurationMs / 1000, // Convert to seconds for display
      })) ?? [],
    [durationData]
  );

  // Chart tooltip
  interface TooltipPayloadEntry {
    color?: string;
    dataKey: string;
    name?: string;
    value?: number;
    payload?: Record<string, number | string>;
  }

  interface CustomTooltipProps {
    active?: boolean;
    payload?: TooltipPayloadEntry[];
    label?: string;
  }

  const PassRateTooltip = ({ active, payload, label }: CustomTooltipProps) => {
    if (!active || !payload?.[0]) return null;
    const data = payload[0].payload;
    return (
      <div className="rounded-lg border bg-background p-3 shadow-md">
        <p className="mb-1 font-medium">{formatDate(label || "")}</p>
        <p className="text-sm">
          Pass Rate: <span className="font-medium">{(data?.passRate as number)?.toFixed(1)}%</span>
        </p>
        <p className="text-xs text-muted-foreground">
          {data?.passed}/{data?.total} tests passed
        </p>
      </div>
    );
  };

  const DurationTooltip = ({ active, payload, label }: CustomTooltipProps) => {
    if (!active || !payload?.[0]) return null;
    const data = payload[0].payload;
    return (
      <div className="rounded-lg border bg-background p-3 shadow-md">
        <p className="mb-1 font-medium">{formatDate(label || "")}</p>
        <p className="text-sm">
          Avg: <span className="font-medium">{formatDuration(data?.avgDurationMs as number)}</span>
        </p>
        <p className="text-xs text-muted-foreground">
          P50: {formatDuration(data?.p50DurationMs as number)} | P95: {formatDuration(data?.p95DurationMs as number)}
        </p>
      </div>
    );
  };

  const DailyStatsTooltip = ({ active, payload, label }: CustomTooltipProps) => {
    if (!active || !payload) return null;
    return (
      <div className="rounded-lg border bg-background p-3 shadow-md">
        <p className="mb-2 font-medium">{formatDate(label || "")}</p>
        {payload.map((entry, index) => (
          <p key={index} className="text-sm" style={{ color: entry.color }}>
            {entry.name}: <span className="font-medium">{entry.value}</span>
          </p>
        ))}
      </div>
    );
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Trends & Analytics</h1>
          <p className="text-muted-foreground">
            Comprehensive view of test performance over time
          </p>
        </div>
        <div className="flex items-center gap-4">
          {/* Date range selector */}
          <div className="flex items-center gap-2">
            <Calendar className="h-4 w-4 text-muted-foreground" />
            <div className="flex gap-1">
              {DATE_RANGES.map((range) => (
                <Button
                  key={range.value}
                  variant={selectedDays === range.value ? "default" : "outline"}
                  size="sm"
                  onClick={() => setSelectedDays(range.value)}
                >
                  {range.label}
                </Button>
              ))}
            </div>
          </div>
        </div>
      </div>

      {/* Service filter */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Filter by Service</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex gap-2 flex-wrap">
            <Button
              variant={!selectedService ? "default" : "outline"}
              size="sm"
              onClick={() => setSelectedService(undefined)}
            >
              All Services
            </Button>
            {servicesData?.items.map((service) => (
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
        </CardContent>
      </Card>

      {/* Summary Trend Cards */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <TrendCard
          title="Pass Rate"
          value={`${summaryMetrics.currentPassRate.toFixed(1)}%`}
          change={summaryMetrics.passRateChange}
          sparklineData={passRateSparkline}
          positiveIsGood={true}
          isLoading={isPassRateLoading}
          icon={<TrendingUp className="h-4 w-4" />}
        />

        <TrendCard
          title="Avg Duration"
          value={formatDuration(durationMetrics.avgDuration)}
          change={durationMetrics.durationChange}
          sparklineData={durationSparkline}
          positiveIsGood={false}
          isLoading={isDurationLoading}
          icon={<Clock className="h-4 w-4" />}
        />

        <TrendCard
          title="Total Tests/Day"
          value={summaryMetrics.totalTests.toLocaleString()}
          change={summaryMetrics.testsChange}
          positiveIsGood={true}
          isLoading={isPassRateLoading}
          icon={<Activity className="h-4 w-4" />}
        />

        <TrendCard
          title="Services"
          value={serviceComparisonData?.services.length ?? 0}
          isLoading={isServiceComparisonLoading}
          icon={<Activity className="h-4 w-4" />}
        />
      </div>

      {/* Charts Row */}
      <div className="grid gap-6 lg:grid-cols-2">
        {/* Pass Rate Trend Chart */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Pass Rate Trend</CardTitle>
            <CardDescription>
              Overall test pass rate over the last {selectedDays} days
            </CardDescription>
          </CardHeader>
          <CardContent>
            {isPassRateLoading ? (
              <Skeleton className="h-64 w-full" />
            ) : passRateData?.data && passRateData.data.length > 0 ? (
              <ResponsiveContainer width="100%" height={256}>
                <LineChart data={passRateData.data}>
                  <CartesianGrid strokeDasharray="3 3" className="stroke-muted" vertical={false} />
                  <XAxis
                    dataKey="date"
                    tickFormatter={formatShortDate}
                    tick={{ fontSize: 11 }}
                    tickLine={false}
                    axisLine={false}
                  />
                  <YAxis
                    domain={[0, 100]}
                    tick={{ fontSize: 11 }}
                    tickLine={false}
                    axisLine={false}
                    tickFormatter={(v) => `${v}%`}
                    width={40}
                  />
                  <Tooltip content={<PassRateTooltip />} />
                  <Line
                    type="monotone"
                    dataKey="passRate"
                    stroke="hsl(var(--primary))"
                    strokeWidth={2}
                    dot={false}
                    activeDot={{ r: 4 }}
                  />
                </LineChart>
              </ResponsiveContainer>
            ) : (
              <div className="h-64 flex items-center justify-center text-muted-foreground">
                No data available
              </div>
            )}
          </CardContent>
        </Card>

        {/* Duration Trend Chart */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Duration Trend</CardTitle>
            <CardDescription>
              Average, P50, and P95 test duration over time
            </CardDescription>
          </CardHeader>
          <CardContent>
            {isDurationLoading ? (
              <Skeleton className="h-64 w-full" />
            ) : durationData?.data && durationData.data.length > 0 ? (
              <ResponsiveContainer width="100%" height={256}>
                <LineChart data={durationData.data}>
                  <CartesianGrid strokeDasharray="3 3" className="stroke-muted" vertical={false} />
                  <XAxis
                    dataKey="date"
                    tickFormatter={formatShortDate}
                    tick={{ fontSize: 11 }}
                    tickLine={false}
                    axisLine={false}
                  />
                  <YAxis
                    tick={{ fontSize: 11 }}
                    tickLine={false}
                    axisLine={false}
                    tickFormatter={(v: number) => formatDuration(v)}
                    width={50}
                  />
                  <Tooltip content={<DurationTooltip />} />
                  <Legend />
                  <Line
                    type="monotone"
                    dataKey="avgDurationMs"
                    name="Avg"
                    stroke="hsl(var(--primary))"
                    strokeWidth={2}
                    dot={false}
                  />
                  <Line
                    type="monotone"
                    dataKey="p50DurationMs"
                    name="P50"
                    stroke="hsl(var(--chart-2))"
                    strokeWidth={1}
                    strokeDasharray="5 5"
                    dot={false}
                  />
                  <Line
                    type="monotone"
                    dataKey="p95DurationMs"
                    name="P95"
                    stroke="hsl(var(--chart-3))"
                    strokeWidth={1}
                    strokeDasharray="5 5"
                    dot={false}
                  />
                </LineChart>
              </ResponsiveContainer>
            ) : (
              <div className="h-64 flex items-center justify-center text-muted-foreground">
                No data available
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Tests Per Day Chart */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Tests Per Day</CardTitle>
          <CardDescription>
            Daily test execution volume broken down by result
          </CardDescription>
        </CardHeader>
        <CardContent>
          {isDailyStatsLoading ? (
            <Skeleton className="h-64 w-full" />
          ) : dailyStats?.data && dailyStats.data.length > 0 ? (
            <ResponsiveContainer width="100%" height={256}>
              <BarChart data={dailyStats.data}>
                <CartesianGrid strokeDasharray="3 3" className="stroke-muted" vertical={false} />
                <XAxis
                  dataKey="date"
                  tickFormatter={formatShortDate}
                  tick={{ fontSize: 11 }}
                  tickLine={false}
                  axisLine={false}
                />
                <YAxis
                  tick={{ fontSize: 11 }}
                  tickLine={false}
                  axisLine={false}
                  width={50}
                />
                <Tooltip content={<DailyStatsTooltip />} />
                <Legend />
                <Bar
                  dataKey="passedRuns"
                  name="Passed"
                  stackId="a"
                  fill="hsl(var(--success))"
                />
                <Bar
                  dataKey="failedRuns"
                  name="Failed"
                  stackId="a"
                  fill="hsl(var(--destructive))"
                />
              </BarChart>
            </ResponsiveContainer>
          ) : (
            <div className="h-64 flex items-center justify-center text-muted-foreground">
              No data available
            </div>
          )}
        </CardContent>
      </Card>

      {/* Top Tests Tables */}
      <div className="grid gap-6 lg:grid-cols-2">
        {/* Top Failing Tests */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <AlertCircle className="h-4 w-4 text-destructive" />
              Top 10 Most Failing Tests
            </CardTitle>
            <CardDescription>
              Tests with the highest failure count in the selected period
            </CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            {isTopFailingLoading ? (
              <div className="p-4 space-y-2">
                {[1, 2, 3, 4, 5].map((i) => (
                  <Skeleton key={i} className="h-12 w-full" />
                ))}
              </div>
            ) : topFailingData?.tests && topFailingData.tests.length > 0 ? (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Test</TableHead>
                    <TableHead className="text-right">Failures</TableHead>
                    <TableHead className="text-right">Trend</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {topFailingData.tests.map((test) => (
                    <TableRow key={test.testId}>
                      <TableCell>
                        <div className="flex flex-col">
                          <span className="font-medium truncate max-w-[200px]">
                            {test.testName}
                          </span>
                          <Badge variant="outline" className="w-fit text-xs">
                            {test.serviceName}
                          </Badge>
                        </div>
                      </TableCell>
                      <TableCell className="text-right font-mono">
                        {test.value}
                      </TableCell>
                      <TableCell className="text-right">
                        <div
                          className={cn(
                            "flex items-center justify-end text-sm",
                            test.trend > 0 ? "text-destructive" : "text-success"
                          )}
                        >
                          {test.trend > 0 ? (
                            <ArrowUpRight className="h-4 w-4" />
                          ) : test.trend < 0 ? (
                            <ArrowDownRight className="h-4 w-4" />
                          ) : null}
                          {Math.abs(test.trend).toFixed(0)}%
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            ) : (
              <div className="p-8 text-center text-muted-foreground">
                No failing tests found
              </div>
            )}
          </CardContent>
        </Card>

        {/* Top Slowest Tests */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Clock className="h-4 w-4 text-warning" />
              Top 10 Slowest Tests
            </CardTitle>
            <CardDescription>
              Tests with the longest average execution time
            </CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            {isTopSlowestLoading ? (
              <div className="p-4 space-y-2">
                {[1, 2, 3, 4, 5].map((i) => (
                  <Skeleton key={i} className="h-12 w-full" />
                ))}
              </div>
            ) : topSlowestData?.tests && topSlowestData.tests.length > 0 ? (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Test</TableHead>
                    <TableHead className="text-right">Avg Duration</TableHead>
                    <TableHead className="text-right">Trend</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {topSlowestData.tests.map((test) => (
                    <TableRow key={test.testId}>
                      <TableCell>
                        <div className="flex flex-col">
                          <span className="font-medium truncate max-w-[200px]">
                            {test.testName}
                          </span>
                          <Badge variant="outline" className="w-fit text-xs">
                            {test.serviceName}
                          </Badge>
                        </div>
                      </TableCell>
                      <TableCell className="text-right font-mono">
                        {formatDuration(test.value)}
                      </TableCell>
                      <TableCell className="text-right">
                        <div
                          className={cn(
                            "flex items-center justify-end text-sm",
                            test.trend > 0 ? "text-destructive" : "text-success"
                          )}
                        >
                          {test.trend > 0 ? (
                            <ArrowUpRight className="h-4 w-4" />
                          ) : test.trend < 0 ? (
                            <ArrowDownRight className="h-4 w-4" />
                          ) : null}
                          {Math.abs(test.trend).toFixed(0)}%
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            ) : (
              <div className="p-8 text-center text-muted-foreground">
                No test data available
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Service Comparison */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Service Comparison</CardTitle>
          <CardDescription>
            Compare metrics across all services
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {isServiceComparisonLoading ? (
            <div className="p-4 space-y-2">
              {[1, 2, 3].map((i) => (
                <Skeleton key={i} className="h-16 w-full" />
              ))}
            </div>
          ) : serviceComparisonData?.services && serviceComparisonData.services.length > 0 ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Service</TableHead>
                  <TableHead className="text-right">Pass Rate</TableHead>
                  <TableHead className="text-right">Avg Duration</TableHead>
                  <TableHead className="text-right">Total Runs</TableHead>
                  <TableHead className="text-right">Flaky Tests</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {serviceComparisonData.services.map((service) => (
                  <TableRow key={service.serviceId}>
                    <TableCell className="font-medium">
                      {service.serviceName}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-2">
                        <span className="font-mono">{service.passRate.toFixed(1)}%</span>
                        <div
                          className={cn(
                            "flex items-center text-xs",
                            service.passRateTrend > 0 ? "text-success" : "text-destructive"
                          )}
                        >
                          {service.passRateTrend > 0 ? (
                            <ArrowUpRight className="h-3 w-3" />
                          ) : (
                            <ArrowDownRight className="h-3 w-3" />
                          )}
                          {Math.abs(service.passRateTrend).toFixed(1)}%
                        </div>
                      </div>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-2">
                        <span className="font-mono">{formatDuration(service.avgDurationMs)}</span>
                        <div
                          className={cn(
                            "flex items-center text-xs",
                            service.durationTrend < 0 ? "text-success" : "text-destructive"
                          )}
                        >
                          {service.durationTrend > 0 ? (
                            <ArrowUpRight className="h-3 w-3" />
                          ) : (
                            <ArrowDownRight className="h-3 w-3" />
                          )}
                          {Math.abs(service.durationTrend).toFixed(1)}%
                        </div>
                      </div>
                    </TableCell>
                    <TableCell className="text-right font-mono">
                      {service.totalRuns.toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right">
                      {service.flakyTestCount > 0 ? (
                        <Badge variant="warning">{service.flakyTestCount}</Badge>
                      ) : (
                        <Badge variant="secondary">0</Badge>
                      )}
                    </TableCell>
                    <TableCell>
                      <Link
                        to="/services/$serviceId"
                        params={{ serviceId: service.serviceId }}
                      >
                        <Button variant="ghost" size="sm">
                          <ExternalLink className="h-4 w-4" />
                        </Button>
                      </Link>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <div className="p-8 text-center text-muted-foreground">
              No service data available
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

export default TrendsPage;
