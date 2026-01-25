/**
 * Reusable trend card component
 * Shows a metric value with trend indicator and optional sparkline
 */

import { useMemo } from "react";
import {
  LineChart,
  Line,
  ResponsiveContainer,
  YAxis,
} from "recharts";
import { ArrowUpRight, ArrowDownRight, Minus } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

// =============================================================================
// Types
// =============================================================================

export interface TrendDataPoint {
  date: string;
  value: number;
}

export interface TrendCardProps {
  /** Card title */
  title: string;
  /** Optional subtitle/description */
  subtitle?: string;
  /** Main metric value to display */
  value: string | number;
  /** Percentage change (positive = up, negative = down) */
  change?: number;
  /** Optional sparkline data */
  sparklineData?: TrendDataPoint[];
  /** Whether positive change is good (default: true) */
  positiveIsGood?: boolean;
  /** Time period options for selector */
  timePeriods?: { label: string; value: string }[];
  /** Currently selected time period */
  selectedPeriod?: string;
  /** Callback when time period changes */
  onPeriodChange?: (period: string) => void;
  /** Loading state */
  isLoading?: boolean;
  /** Optional icon */
  icon?: React.ReactNode;
  /** Additional class names */
  className?: string;
}

// =============================================================================
// Component
// =============================================================================

export function TrendCard({
  title,
  subtitle,
  value,
  change,
  sparklineData,
  positiveIsGood = true,
  timePeriods,
  selectedPeriod,
  onPeriodChange,
  isLoading = false,
  icon,
  className,
}: TrendCardProps) {
  // Determine if change is positive/negative from a presentation standpoint
  const isPositiveChange = change !== undefined && change > 0;
  const isNegativeChange = change !== undefined && change < 0;
  
  // Determine if the change is "good" or "bad" for coloring
  const isGoodChange = positiveIsGood ? isPositiveChange : isNegativeChange;
  const isBadChange = positiveIsGood ? isNegativeChange : isPositiveChange;

  // Sparkline color based on trend
  const sparklineColor = useMemo(() => {
    if (sparklineData && sparklineData.length >= 2) {
      const firstValue = sparklineData[0]?.value ?? 0;
      const lastValue = sparklineData[sparklineData.length - 1]?.value ?? 0;
      const isUp = lastValue > firstValue;
      if (positiveIsGood) {
        return isUp ? "hsl(var(--success))" : "hsl(var(--destructive))";
      }
      return isUp ? "hsl(var(--destructive))" : "hsl(var(--success))";
    }
    return "hsl(var(--primary))";
  }, [sparklineData, positiveIsGood]);

  // Calculate Y domain for sparkline
  const yDomain = useMemo(() => {
    if (!sparklineData || sparklineData.length === 0) return [0, 100];
    const values = sparklineData.map((d) => d.value);
    const min = Math.min(...values);
    const max = Math.max(...values);
    const padding = (max - min) * 0.1 || 1;
    return [min - padding, max + padding];
  }, [sparklineData]);

  if (isLoading) {
    return (
      <Card className={className}>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <Skeleton className="h-4 w-24" />
          {icon && <Skeleton className="h-4 w-4" />}
        </CardHeader>
        <CardContent>
          <Skeleton className="mb-2 h-8 w-20" />
          <Skeleton className="h-4 w-32" />
          {sparklineData !== undefined && (
            <Skeleton className="mt-4 h-12 w-full" />
          )}
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className={className}>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <div className="flex flex-col">
          <CardTitle className="text-sm font-medium">{title}</CardTitle>
          {subtitle && (
            <p className="text-xs text-muted-foreground">{subtitle}</p>
          )}
        </div>
        <div className="flex items-center gap-2">
          {timePeriods && onPeriodChange && (
            <div className="flex gap-0.5">
              {timePeriods.map((period) => (
                <Button
                  key={period.value}
                  variant={selectedPeriod === period.value ? "default" : "ghost"}
                  size="sm"
                  className="h-6 px-2 text-xs"
                  onClick={() => onPeriodChange(period.value)}
                >
                  {period.label}
                </Button>
              ))}
            </div>
          )}
          {icon && (
            <div className="text-muted-foreground">{icon}</div>
          )}
        </div>
      </CardHeader>
      <CardContent>
        <div className="flex items-baseline gap-2">
          <div className="text-2xl font-bold">{value}</div>
          {change !== undefined && (
            <div
              className={cn(
                "flex items-center text-sm",
                isGoodChange && "text-success",
                isBadChange && "text-destructive",
                !isGoodChange && !isBadChange && "text-muted-foreground"
              )}
            >
              {isPositiveChange ? (
                <ArrowUpRight className="mr-0.5 h-4 w-4" />
              ) : isNegativeChange ? (
                <ArrowDownRight className="mr-0.5 h-4 w-4" />
              ) : (
                <Minus className="mr-0.5 h-4 w-4" />
              )}
              <span>{Math.abs(change).toFixed(1)}%</span>
            </div>
          )}
        </div>

        {sparklineData && sparklineData.length > 1 && (
          <div className="mt-4 h-12">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={sparklineData}>
                <YAxis domain={yDomain} hide />
                <Line
                  type="monotone"
                  dataKey="value"
                  stroke={sparklineColor}
                  strokeWidth={2}
                  dot={false}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

export default TrendCard;
