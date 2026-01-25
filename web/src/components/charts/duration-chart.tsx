/**
 * Duration trend chart component
 * Shows run duration over time with P50/P95 percentile lines
 */

import { useMemo, useState } from "react";
import {
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
  Area,
  ComposedChart,
} from "recharts";
import { format, parseISO } from "date-fns";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";

// =============================================================================
// Types
// =============================================================================

export interface DurationDataPoint {
  date: string;
  avgDurationMs: number;
  p50DurationMs: number;
  p95DurationMs: number;
  minDurationMs?: number;
  maxDurationMs?: number;
}

export interface DurationChartProps {
  /** Duration data points */
  data: DurationDataPoint[];
  /** Chart title */
  title?: string;
  /** Height of the chart */
  height?: number;
  /** Whether data is loading */
  isLoading?: boolean;
  /** Show P50 line */
  showP50?: boolean;
  /** Show P95 line */
  showP95?: boolean;
  /** Show average line */
  showAvg?: boolean;
  /** Show range area (min-max) */
  showRange?: boolean;
  /** Date range options */
  dateRangeOptions?: { label: string; days: number }[];
  /** Currently selected date range in days */
  selectedDays?: number;
  /** Callback when date range changes */
  onDateRangeChange?: (days: number) => void;
  /** Class name for container */
  className?: string;
}

// =============================================================================
// Constants
// =============================================================================

const DEFAULT_DATE_RANGES = [
  { label: "7d", days: 7 },
  { label: "14d", days: 14 },
  { label: "30d", days: 30 },
  { label: "90d", days: 90 },
];

// =============================================================================
// Utilities
// =============================================================================

/**
 * Format duration in milliseconds to human-readable string
 */
function formatDuration(ms: number): string {
  if (ms < 1000) {
    return `${ms}ms`;
  }
  const seconds = ms / 1000;
  if (seconds < 60) {
    return `${seconds.toFixed(1)}s`;
  }
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = Math.round(seconds % 60);
  if (minutes < 60) {
    return remainingSeconds > 0 ? `${minutes}m ${remainingSeconds}s` : `${minutes}m`;
  }
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return remainingMinutes > 0 ? `${hours}h ${remainingMinutes}m` : `${hours}h`;
}

/**
 * Format duration for Y-axis (abbreviated)
 */
function formatDurationShort(ms: number): string {
  if (ms < 1000) {
    return `${ms}ms`;
  }
  const seconds = ms / 1000;
  if (seconds < 60) {
    return `${Math.round(seconds)}s`;
  }
  const minutes = seconds / 60;
  if (minutes < 60) {
    return `${Math.round(minutes)}m`;
  }
  const hours = minutes / 60;
  return `${hours.toFixed(1)}h`;
}

// =============================================================================
// Component
// =============================================================================

export function DurationChart({
  data,
  title = "Duration Trend",
  height = 300,
  isLoading = false,
  showP50 = true,
  showP95 = true,
  showAvg = true,
  showRange = false,
  dateRangeOptions = DEFAULT_DATE_RANGES,
  selectedDays = 14,
  onDateRangeChange,
  className,
}: DurationChartProps) {
  const [internalSelectedDays, setInternalSelectedDays] = useState(selectedDays);
  const activeDays = onDateRangeChange ? selectedDays : internalSelectedDays;

  // Process data for the chart
  const chartData = useMemo(() => {
    return data.map((point) => ({
      ...point,
      // Convert to seconds for display
      avgSeconds: point.avgDurationMs / 1000,
      p50Seconds: point.p50DurationMs / 1000,
      p95Seconds: point.p95DurationMs / 1000,
      minSeconds: point.minDurationMs ? point.minDurationMs / 1000 : undefined,
      maxSeconds: point.maxDurationMs ? point.maxDurationMs / 1000 : undefined,
    }));
  }, [data]);

  // Format date for display
  const formatDate = (dateStr: string) => {
    try {
      return format(parseISO(dateStr), "MMM d");
    } catch {
      return dateStr;
    }
  };

  // Custom tooltip
  interface DurationDataPayload {
    avgDurationMs: number;
    p50DurationMs: number;
    p95DurationMs: number;
    minDurationMs?: number;
    maxDurationMs?: number;
  }
  
  interface TooltipPayloadEntry {
    payload?: DurationDataPayload;
  }
  
  interface CustomTooltipProps {
    active?: boolean;
    payload?: TooltipPayloadEntry[];
    label?: string;
  }
  
  const CustomTooltip = ({ active, payload, label }: CustomTooltipProps) => {
    if (!active || !payload || !payload.length) return null;

    const dataPoint = payload[0]?.payload;
    if (!dataPoint) return null;

    return (
      <div className="rounded-lg border bg-background p-3 shadow-md">
        <p className="mb-2 font-medium">{formatDate(label || "")}</p>
        <div className="space-y-1 text-sm">
          {showAvg && (
            <div className="flex items-center gap-2">
              <div className="h-2 w-2 rounded-full bg-primary" />
              <span className="text-muted-foreground">Average:</span>
              <span className="font-medium">
                {formatDuration(dataPoint.avgDurationMs)}
              </span>
            </div>
          )}
          {showP50 && (
            <div className="flex items-center gap-2">
              <div className="h-2 w-2 rounded-full" style={{ backgroundColor: "hsl(var(--chart-2))" }} />
              <span className="text-muted-foreground">P50:</span>
              <span className="font-medium">
                {formatDuration(dataPoint.p50DurationMs)}
              </span>
            </div>
          )}
          {showP95 && (
            <div className="flex items-center gap-2">
              <div className="h-2 w-2 rounded-full" style={{ backgroundColor: "hsl(var(--chart-3))" }} />
              <span className="text-muted-foreground">P95:</span>
              <span className="font-medium">
                {formatDuration(dataPoint.p95DurationMs)}
              </span>
            </div>
          )}
          {showRange && dataPoint.minDurationMs && dataPoint.maxDurationMs && (
            <div className="mt-2 border-t pt-2 text-xs text-muted-foreground">
              Range: {formatDuration(dataPoint.minDurationMs)} -{" "}
              {formatDuration(dataPoint.maxDurationMs)}
            </div>
          )}
        </div>
      </div>
    );
  };

  const handleDateRangeChange = (days: number) => {
    if (onDateRangeChange) {
      onDateRangeChange(days);
    } else {
      setInternalSelectedDays(days);
    }
  };

  if (isLoading) {
    return (
      <Card className={className}>
        <CardHeader className="flex flex-row items-center justify-between pb-2">
          <CardTitle className="text-base font-medium">{title}</CardTitle>
        </CardHeader>
        <CardContent>
          <div
            className="flex items-center justify-center"
            style={{ height }}
          >
            <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className={className}>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="text-base font-medium">{title}</CardTitle>
        <div className="flex gap-1">
          {dateRangeOptions.map((option) => (
            <Button
              key={option.days}
              variant={activeDays === option.days ? "default" : "ghost"}
              size="sm"
              className="h-7 px-2 text-xs"
              onClick={() => handleDateRangeChange(option.days)}
            >
              {option.label}
            </Button>
          ))}
        </div>
      </CardHeader>
      <CardContent>
        {chartData.length === 0 ? (
          <div
            className="flex items-center justify-center text-muted-foreground"
            style={{ height }}
          >
            No data available
          </div>
        ) : (
          <ResponsiveContainer width="100%" height={height}>
            <ComposedChart
              data={chartData}
              margin={{ top: 5, right: 10, left: 0, bottom: 5 }}
            >
              <CartesianGrid
                strokeDasharray="3 3"
                className="stroke-muted"
                vertical={false}
              />
              <XAxis
                dataKey="date"
                tickFormatter={formatDate}
                tick={{ fontSize: 12 }}
                tickLine={false}
                axisLine={false}
                className="text-muted-foreground"
              />
              <YAxis
                tick={{ fontSize: 12 }}
                tickLine={false}
                axisLine={false}
                tickFormatter={(value) => formatDurationShort(value * 1000)}
                className="text-muted-foreground"
                width={45}
              />
              <Tooltip content={<CustomTooltip />} />
              <Legend
                formatter={(value: string) => {
                  const labels: Record<string, string> = {
                    avgSeconds: "Average",
                    p50Seconds: "P50",
                    p95Seconds: "P95",
                  };
                  return labels[value] || value;
                }}
              />

              {showRange && (
                <Area
                  type="monotone"
                  dataKey="maxSeconds"
                  stroke="none"
                  fill="hsl(var(--muted))"
                  fillOpacity={0.3}
                />
              )}

              {showP95 && (
                <Line
                  type="monotone"
                  dataKey="p95Seconds"
                  name="p95Seconds"
                  stroke="hsl(var(--chart-3))"
                  strokeWidth={2}
                  strokeDasharray="5 5"
                  dot={false}
                  activeDot={{ r: 4 }}
                />
              )}

              {showP50 && (
                <Line
                  type="monotone"
                  dataKey="p50Seconds"
                  name="p50Seconds"
                  stroke="hsl(var(--chart-2))"
                  strokeWidth={2}
                  dot={false}
                  activeDot={{ r: 4 }}
                />
              )}

              {showAvg && (
                <Line
                  type="monotone"
                  dataKey="avgSeconds"
                  name="avgSeconds"
                  stroke="hsl(var(--primary))"
                  strokeWidth={2}
                  dot={false}
                  activeDot={{ r: 4 }}
                />
              )}
            </ComposedChart>
          </ResponsiveContainer>
        )}
      </CardContent>
    </Card>
  );
}

export default DurationChart;
