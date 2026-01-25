/**
 * Flaky test row component
 * Displays a flaky test with flakiness score, status, and action controls
 */

import { Link } from "@tanstack/react-router";
import {
  MoreHorizontal,
  ShieldAlert,
  ShieldCheck,
  History,
  ExternalLink,
  AlertTriangle,
} from "lucide-react";
import {
  TableCell,
  TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import { formatRelativeTime } from "@/lib/utils";
import type { FlakyTest } from "@/api/analytics";

// =============================================================================
// Types
// =============================================================================

export interface FlakyTestRowProps {
  /** Flaky test data */
  test: FlakyTest;
  /** Callback when quarantine action is triggered */
  onQuarantine?: (testId: string) => void;
  /** Callback when unquarantine action is triggered */
  onUnquarantine?: (testId: string) => void;
  /** Callback when view history is triggered */
  onViewHistory?: (testId: string) => void;
  /** Whether actions are loading */
  isLoading?: boolean;
  /** Whether this row is selected */
  isSelected?: boolean;
  /** Callback when row is clicked */
  onClick?: () => void;
}

// =============================================================================
// Helper Components
// =============================================================================

interface FlakinessScoreBarProps {
  score: number;
}

function FlakinessScoreBar({ score }: FlakinessScoreBarProps) {
  // Determine color based on flakiness score
  const getBarColor = () => {
    if (score >= 50) return "bg-destructive";
    if (score >= 25) return "bg-warning";
    if (score >= 10) return "bg-yellow-500";
    return "bg-success";
  };

  return (
    <div className="flex items-center gap-2">
      <div className="relative h-2 w-20 overflow-hidden rounded-full bg-muted">
        <div
          className={cn("absolute left-0 top-0 h-full transition-all", getBarColor())}
          style={{ width: `${Math.min(score, 100)}%` }}
        />
      </div>
      <span className="text-sm font-medium tabular-nums">
        {score.toFixed(1)}%
      </span>
    </div>
  );
}

// =============================================================================
// Main Component
// =============================================================================

export function FlakyTestRow({
  test,
  onQuarantine,
  onUnquarantine,
  onViewHistory,
  isLoading = false,
  isSelected = false,
  onClick,
}: FlakyTestRowProps) {
  const handleQuarantineToggle = () => {
    if (test.isQuarantined) {
      onUnquarantine?.(test.testId);
    } else {
      onQuarantine?.(test.testId);
    }
  };

  return (
    <TableRow
      className={cn(
        "cursor-pointer",
        isSelected && "bg-muted/50",
        test.isQuarantined && "opacity-60"
      )}
      onClick={onClick}
    >
      {/* Test name and service */}
      <TableCell>
        <div className="flex flex-col gap-1">
          <div className="flex items-center gap-2">
            {test.isQuarantined && (
              <ShieldAlert className="h-4 w-4 text-warning" />
            )}
            <span className="font-medium">{test.testName}</span>
          </div>
          <div className="flex items-center gap-2">
            <Badge variant="outline" className="text-xs">
              {test.serviceName}
            </Badge>
            <span className="text-xs text-muted-foreground truncate max-w-[200px]">
              {test.testPath}
            </span>
          </div>
        </div>
      </TableCell>

      {/* Flakiness score */}
      <TableCell>
        <FlakinessScoreBar score={test.flakinessScore} />
      </TableCell>

      {/* Run stats */}
      <TableCell>
        <div className="flex items-center gap-1 text-sm">
          <span className="text-muted-foreground">{test.flakyRuns}</span>
          <span className="text-muted-foreground">/</span>
          <span>{test.totalRuns}</span>
          <span className="text-xs text-muted-foreground">runs</span>
        </div>
      </TableCell>

      {/* Last flaky */}
      <TableCell>
        <span className="text-sm text-muted-foreground">
          {formatRelativeTime(test.lastFlakyDate)}
        </span>
      </TableCell>

      {/* Status */}
      <TableCell>
        {test.isQuarantined ? (
          <Badge variant="warning" className="gap-1">
            <ShieldAlert className="h-3 w-3" />
            Quarantined
          </Badge>
        ) : (
          <Badge variant="secondary" className="gap-1">
            <AlertTriangle className="h-3 w-3" />
            Active
          </Badge>
        )}
      </TableCell>

      {/* Actions */}
      <TableCell>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className="h-8 w-8 p-0"
              disabled={isLoading}
              onClick={(e) => e.stopPropagation()}
            >
              <MoreHorizontal className="h-4 w-4" />
              <span className="sr-only">Open menu</span>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" onClick={(e) => e.stopPropagation()}>
            <DropdownMenuItem onClick={() => onViewHistory?.(test.testId)}>
              <History className="mr-2 h-4 w-4" />
              View History
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={handleQuarantineToggle}>
              {test.isQuarantined ? (
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
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link
                to="/services/$serviceId"
                params={{ serviceId: test.serviceId }}
              >
                <ExternalLink className="mr-2 h-4 w-4" />
                View Service
              </Link>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </TableCell>
    </TableRow>
  );
}

export default FlakyTestRow;
