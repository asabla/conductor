/**
 * Failure group card component
 * Displays a group of related failures with expandable affected tests list
 */

import { useState } from "react";
import { Link } from "@tanstack/react-router";
import {
  LineChart,
  Line,
  ResponsiveContainer,
  Tooltip,
} from "recharts";
import {
  ChevronDown,
  ChevronRight,
  AlertCircle,
  ExternalLink,
  Calendar,
  Hash,
} from "lucide-react";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { formatRelativeTime } from "@/lib/utils";
import type { FailureGroup, AffectedTest, TrendPoint } from "@/api/analytics";

// =============================================================================
// Types
// =============================================================================

export interface FailureGroupCardProps {
  /** Failure group data */
  group: FailureGroup;
  /** Initial expanded state */
  defaultExpanded?: boolean;
  /** Maximum number of affected tests to show when collapsed */
  previewCount?: number;
  /** Additional class names */
  className?: string;
}

// =============================================================================
// Subcomponents
// =============================================================================

interface SparklineTooltipProps {
  active?: boolean;
  payload?: Array<{
    value: number;
    payload: TrendPoint;
  }>;
}

function SparklineTooltip({ active, payload }: SparklineTooltipProps) {
  if (!active || !payload?.[0]) return null;

  const data = payload[0].payload;
  return (
    <div className="rounded border bg-background px-2 py-1 text-xs shadow">
      <div className="font-medium">{data.count} occurrences</div>
      <div className="text-muted-foreground">{data.date}</div>
    </div>
  );
}

interface AffectedTestItemProps {
  test: AffectedTest;
}

function AffectedTestItem({ test }: AffectedTestItemProps) {
  return (
    <div className="flex items-center justify-between rounded-md border px-3 py-2 text-sm hover:bg-muted/50">
      <div className="flex flex-col gap-0.5">
        <div className="flex items-center gap-2">
          <span className="font-medium">{test.testName}</span>
          <Badge variant="outline" className="text-xs">
            {test.serviceName}
          </Badge>
        </div>
        <span className="text-xs text-muted-foreground">{test.testPath}</span>
      </div>
      <div className="flex items-center gap-2">
        <span className="text-xs text-muted-foreground">
          {formatRelativeTime(test.occurredAt)}
        </span>
        <Link
          to="/test-runs/$runId"
          params={{ runId: test.runId }}
          className="text-muted-foreground hover:text-foreground"
        >
          <ExternalLink className="h-4 w-4" />
        </Link>
      </div>
    </div>
  );
}

// =============================================================================
// Main Component
// =============================================================================

export function FailureGroupCard({
  group,
  defaultExpanded = false,
  previewCount = 3,
  className,
}: FailureGroupCardProps) {
  const [isExpanded, setIsExpanded] = useState(defaultExpanded);
  
  const hasMoreTests = group.affectedTests.length > previewCount;
  const displayedTests = isExpanded
    ? group.affectedTests
    : group.affectedTests.slice(0, previewCount);

  // Truncate error signature for display
  const truncatedSignature = group.errorSignature.length > 80
    ? group.errorSignature.slice(0, 80) + "..."
    : group.errorSignature;

  return (
    <Card className={cn("overflow-hidden", className)}>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-4">
          <div className="flex-1 min-w-0">
            {/* Error signature header */}
            <div className="flex items-center gap-2 mb-2">
              <AlertCircle className="h-5 w-5 text-destructive flex-shrink-0" />
              <h3 className="font-mono text-sm font-medium truncate" title={group.errorSignature}>
                {truncatedSignature}
              </h3>
            </div>
            
            {/* Error message */}
            <p className="text-sm text-muted-foreground line-clamp-2" title={group.errorMessage}>
              {group.errorMessage}
            </p>

            {/* Metadata row */}
            <div className="mt-3 flex flex-wrap items-center gap-4 text-xs text-muted-foreground">
              <div className="flex items-center gap-1">
                <Hash className="h-3 w-3" />
                <span>{group.count} occurrences</span>
              </div>
              <div className="flex items-center gap-1">
                <Calendar className="h-3 w-3" />
                <span>First: {formatRelativeTime(group.firstOccurrence)}</span>
              </div>
              <div className="flex items-center gap-1">
                <Calendar className="h-3 w-3" />
                <span>Last: {formatRelativeTime(group.lastOccurrence)}</span>
              </div>
            </div>

            {/* Service badges */}
            {group.services.length > 0 && (
              <div className="mt-2 flex flex-wrap gap-1">
                {group.services.map((service) => (
                  <Badge key={service} variant="secondary" className="text-xs">
                    {service}
                  </Badge>
                ))}
              </div>
            )}
          </div>

          {/* Right side: count badge and sparkline */}
          <div className="flex flex-col items-end gap-2">
            <Badge variant="destructive" className="text-lg px-3 py-1">
              {group.count}
            </Badge>
            
            {/* Sparkline */}
            {group.trendData.length > 1 && (
              <div className="h-10 w-24">
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart data={group.trendData}>
                    <Tooltip content={<SparklineTooltip />} />
                    <Line
                      type="monotone"
                      dataKey="count"
                      stroke="hsl(var(--destructive))"
                      strokeWidth={2}
                      dot={false}
                    />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            )}
          </div>
        </div>
      </CardHeader>

      <CardContent className="pt-0">
        {/* Affected tests section */}
        <div className="border-t pt-4">
          <Button
            variant="ghost"
            size="sm"
            className="mb-3 -ml-2 text-muted-foreground hover:text-foreground"
            onClick={() => setIsExpanded(!isExpanded)}
          >
            {isExpanded ? (
              <ChevronDown className="mr-1 h-4 w-4" />
            ) : (
              <ChevronRight className="mr-1 h-4 w-4" />
            )}
            Affected Tests ({group.affectedTests.length})
          </Button>

          <div className="space-y-2">
            {displayedTests.map((test, index) => (
              <AffectedTestItem
                key={`${test.testId}-${test.runId}-${index}`}
                test={test}
              />
            ))}
          </div>

          {hasMoreTests && !isExpanded && (
            <Button
              variant="ghost"
              size="sm"
              className="mt-2 w-full text-muted-foreground"
              onClick={() => setIsExpanded(true)}
            >
              Show {group.affectedTests.length - previewCount} more tests
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

export default FailureGroupCard;
