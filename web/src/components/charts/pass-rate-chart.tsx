/**
 * Pass rate trend chart component
 * Shows pass rate over time with optional service comparison
 */

import { useMemo, useState } from "react";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
  ReferenceLine,
} from "recharts";
import { format, parseISO } from "date-fns";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";


// =============================================================================
// Types
// =============================================================================

export interface PassRateDataPoint {
  date: string;
  passRate: number;
  total: number;
  passed: number;
  failed: number;
}

export interface ServicePassRateData {
  serviceId: string;
  serviceName: string;
  color: string;
  data: PassRateDataPoint[];
}

export interface PassRateChartProps {
  /** Single service data */
  data?: PassRateDataPoint[];
  /** Multiple services for comparison */
  servicesData?: ServicePassRateData[];
  /** Chart title */
  title?: string;
  /** Show target line */
  targetPassRate?: number;
  /** Height of the chart */
  height?: number;
  /** Whether data is loading */
  isLoading?: boolean;
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

const SERVICE_COLORS = [
  "hsl(var(--primary))",
  "hsl(var(--chart-2))",
  "hsl(var(--chart-3))",
  "hsl(var(--chart-4))",
  "hsl(var(--chart-5))",
];

// =============================================================================
// Component
// =============================================================================

export function PassRateChart({
  data,
  servicesData,
  title = "Pass Rate Trend",
  targetPassRate,
  height = 300,
  isLoading = false,
  dateRangeOptions = DEFAULT_DATE_RANGES,
  selectedDays = 14,
  onDateRangeChange,
  className,
}: PassRateChartProps) {
  const [internalSelectedDays, setInternalSelectedDays] = useState(selectedDays);
  const activeDays = onDateRangeChange ? selectedDays : internalSelectedDays;

  // Process data for the chart
  const chartData = useMemo(() => {
    if (servicesData && servicesData.length > 0) {
      // Merge multiple services data by date
      const dateMap = new Map<string, Record<string, number | string>>();
      
      servicesData.forEach((service) => {
        service.data.forEach((point) => {
          const existing = dateMap.get(point.date) || { date: point.date };
          existing[service.serviceId] = point.passRate;
          dateMap.set(point.date, existing);
        });
      });

      return Array.from(dateMap.values()).sort(
        (a, b) => new Date(a.date as string).getTime() - new Date(b.date as string).getTime()
      );
    }

    if (data) {
      return data.map((point) => ({
        date: point.date,
        passRate: point.passRate,
        total: point.total,
        passed: point.passed,
        failed: point.failed,
      }));
    }

    return [];
  }, [data, servicesData]);

  // Format date for display
  const formatDate = (dateStr: string) => {
    try {
      return format(parseISO(dateStr), "MMM d");
    } catch {
      return dateStr;
    }
  };

  // Custom tooltip
  interface TooltipPayloadEntry {
    color: string;
    dataKey: string;
    value?: number;
    payload?: {
      passed: number;
      total: number;
    };
  }
  
  interface CustomTooltipProps {
    active?: boolean;
    payload?: TooltipPayloadEntry[];
    label?: string;
  }
  
  const CustomTooltip = ({ active, payload, label }: CustomTooltipProps) => {
    if (!active || !payload) return null;

    return (
      <div className="rounded-lg border bg-background p-3 shadow-md">
        <p className="mb-2 font-medium">{formatDate(label || "")}</p>
        {payload.map((entry, index: number) => (
          <div key={index} className="flex items-center gap-2 text-sm">
            <div
              className="h-2 w-2 rounded-full"
              style={{ backgroundColor: entry.color }}
            />
            <span className="text-muted-foreground">
              {servicesData
                ? servicesData.find((s) => s.serviceId === entry.dataKey)
                    ?.serviceName || entry.dataKey
                : "Pass Rate"}
              :
            </span>
            <span className="font-medium">{entry.value?.toFixed(1)}%</span>
          </div>
        ))}
        {!servicesData && payload[0]?.payload && (
          <div className="mt-2 border-t pt-2 text-xs text-muted-foreground">
            {payload[0].payload.passed}/{payload[0].payload.total} tests passed
          </div>
        )}
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
            <LineChart
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
                domain={[0, 100]}
                tick={{ fontSize: 12 }}
                tickLine={false}
                axisLine={false}
                tickFormatter={(value) => `${value}%`}
                className="text-muted-foreground"
                width={45}
              />
              <Tooltip content={<CustomTooltip />} />
              
              {targetPassRate && (
                <ReferenceLine
                  y={targetPassRate}
                  stroke="hsl(var(--warning))"
                  strokeDasharray="5 5"
                  label={{
                    value: `Target: ${targetPassRate}%`,
                    position: "right",
                    fontSize: 11,
                    fill: "hsl(var(--warning))",
                  }}
                />
              )}

              {servicesData ? (
                <>
                  <Legend
                    formatter={(value: string) =>
                      servicesData.find((s) => s.serviceId === value)
                        ?.serviceName || value
                    }
                  />
                  {servicesData.map((service, index) => (
                    <Line
                      key={service.serviceId}
                      type="monotone"
                      dataKey={service.serviceId}
                      stroke={service.color || SERVICE_COLORS[index % SERVICE_COLORS.length]}
                      strokeWidth={2}
                      dot={false}
                      activeDot={{ r: 4 }}
                    />
                  ))}
                </>
              ) : (
                <Line
                  type="monotone"
                  dataKey="passRate"
                  stroke="hsl(var(--primary))"
                  strokeWidth={2}
                  dot={false}
                  activeDot={{ r: 4 }}
                />
              )}
            </LineChart>
          </ResponsiveContainer>
        )}
      </CardContent>
    </Card>
  );
}

export default PassRateChart;
