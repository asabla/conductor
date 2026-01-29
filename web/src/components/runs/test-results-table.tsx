/**
 * Test results table component
 * Features: sortable columns, expandable rows, status filtering, search
 */

import { useState, useMemo, useCallback, Fragment } from "react";
import {
  ChevronDown,
  ChevronRight,
  ChevronUp,
  Search,
  Filter,
  CheckCircle,
  XCircle,
  MinusCircle,
  AlertCircle,
  Clock,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuCheckboxItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";
import { formatDuration } from "@/lib/utils";

// =============================================================================
// Types
// =============================================================================

export type TestStatus = "passed" | "failed" | "skipped" | "error";

export interface TestResultRow {
  id: string;
  testName: string;
  testPath?: string;
  suiteName?: string;
  status: TestStatus;
  durationMs: number;
  errorMessage?: string;
  stackTrace?: string;
  stdout?: string;
  stderr?: string;
  retryAttempt?: number;
}

export interface TestResultsTableProps {
  /** Test results data */
  results: TestResultRow[];
  /** Whether data is loading */
  isLoading?: boolean;
  /** Maximum height for the table */
  maxHeight?: number | string;
  /** Callback when a test row is clicked */
  onTestClick?: (test: TestResultRow) => void;
  /** Class name for container */
  className?: string;
}

type SortField = "name" | "status" | "duration";
type SortOrder = "asc" | "desc";

// =============================================================================
// Status Helpers
// =============================================================================

const STATUS_CONFIG: Record<
  TestStatus,
  { label: string; icon: typeof CheckCircle; variant: "success" | "destructive" | "secondary" | "warning" }
> = {
  passed: { label: "Passed", icon: CheckCircle, variant: "success" },
  failed: { label: "Failed", icon: XCircle, variant: "destructive" },
  skipped: { label: "Skipped", icon: MinusCircle, variant: "secondary" },
  error: { label: "Error", icon: AlertCircle, variant: "warning" },
};

function getStatusBadge(status: TestStatus) {
  const config = STATUS_CONFIG[status];
  const Icon = config.icon;
  return (
    <Badge variant={config.variant} className="gap-1">
      <Icon className="h-3 w-3" />
      {config.label}
    </Badge>
  );
}

// =============================================================================
// Expandable Row Component
// =============================================================================

interface ExpandedRowContentProps {
  test: TestResultRow;
}

function ExpandedRowContent({ test }: ExpandedRowContentProps) {
  const [activeTab, setActiveTab] = useState<"error" | "stdout" | "stderr">(
    test.errorMessage ? "error" : "stdout"
  );

  const hasContent =
    test.errorMessage || test.stackTrace || test.stdout || test.stderr;

  if (!hasContent) {
    return (
      <div className="px-4 py-3 text-sm text-muted-foreground">
        No additional details available
      </div>
    );
  }

  return (
    <div className="space-y-3 px-4 py-3">
      {/* Tab buttons */}
      <div className="flex gap-1">
        {test.errorMessage && (
          <Button
            variant={activeTab === "error" ? "default" : "ghost"}
            size="sm"
            className="h-7 text-xs"
            onClick={() => setActiveTab("error")}
          >
            Error
          </Button>
        )}
        {test.stdout && (
          <Button
            variant={activeTab === "stdout" ? "default" : "ghost"}
            size="sm"
            className="h-7 text-xs"
            onClick={() => setActiveTab("stdout")}
          >
            Stdout
          </Button>
        )}
        {test.stderr && (
          <Button
            variant={activeTab === "stderr" ? "default" : "ghost"}
            size="sm"
            className="h-7 text-xs"
            onClick={() => setActiveTab("stderr")}
          >
            Stderr
          </Button>
        )}
      </div>

      {/* Content */}
      <div className="rounded-md bg-muted/50 p-3">
        {activeTab === "error" && (
          <div className="space-y-2">
            {test.errorMessage && (
              <div>
                <p className="mb-1 text-xs font-medium text-destructive">
                  Error Message
                </p>
                <pre className="whitespace-pre-wrap text-xs text-foreground">
                  {test.errorMessage}
                </pre>
              </div>
            )}
            {test.stackTrace && (
              <div>
                <p className="mb-1 text-xs font-medium text-muted-foreground">
                  Stack Trace
                </p>
                <pre className="max-h-64 overflow-auto whitespace-pre-wrap font-mono text-xs text-muted-foreground">
                  {test.stackTrace}
                </pre>
              </div>
            )}
          </div>
        )}
        {activeTab === "stdout" && test.stdout && (
          <pre className="max-h-64 overflow-auto whitespace-pre-wrap font-mono text-xs">
            {test.stdout}
          </pre>
        )}
        {activeTab === "stderr" && test.stderr && (
          <pre className="max-h-64 overflow-auto whitespace-pre-wrap font-mono text-xs text-destructive/80">
            {test.stderr}
          </pre>
        )}
      </div>
    </div>
  );
}

// =============================================================================
// Main Component
// =============================================================================

export function TestResultsTable({
  results,
  isLoading = false,
  maxHeight = 600,
  onTestClick,
  className,
}: TestResultsTableProps) {
  const [searchQuery, setSearchQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState<TestStatus[]>([]);
  const [sortField, setSortField] = useState<SortField>("name");
  const [sortOrder, setSortOrder] = useState<SortOrder>("asc");
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set());

  // Filter and sort results
  const processedResults = useMemo(() => {
    let filtered = results;

    // Apply search filter
    if (searchQuery) {
      const query = searchQuery.toLowerCase();
      filtered = filtered.filter(
        (r) =>
          r.testName.toLowerCase().includes(query) ||
          r.suiteName?.toLowerCase().includes(query) ||
          r.testPath?.toLowerCase().includes(query)
      );
    }

    // Apply status filter
    if (statusFilter.length > 0) {
      filtered = filtered.filter((r) => statusFilter.includes(r.status));
    }

    // Apply sorting
    const statusOrder: TestStatus[] = ["failed", "error", "passed", "skipped"];

    const sorted = [...filtered].sort((a, b) => {
      let comparison = 0;

      switch (sortField) {
        case "name":
          comparison = a.testName.localeCompare(b.testName);
          break;
        case "status":
          comparison =
            statusOrder.indexOf(a.status) - statusOrder.indexOf(b.status);
          break;
        case "duration":
          comparison = a.durationMs - b.durationMs;
          break;
      }

      return sortOrder === "asc" ? comparison : -comparison;
    });

    return sorted;
  }, [results, searchQuery, statusFilter, sortField, sortOrder]);

  // Summary counts
  const summary = useMemo(() => {
    return {
      total: results.length,
      passed: results.filter((r) => r.status === "passed").length,
      failed: results.filter((r) => r.status === "failed").length,
      skipped: results.filter((r) => r.status === "skipped").length,
      error: results.filter((r) => r.status === "error").length,
    };
  }, [results]);

  // Handle sort
  const handleSort = useCallback(
    (field: SortField) => {
      if (sortField === field) {
        setSortOrder(sortOrder === "asc" ? "desc" : "asc");
      } else {
        setSortField(field);
        setSortOrder("asc");
      }
    },
    [sortField, sortOrder]
  );

  // Toggle row expansion
  const toggleExpanded = useCallback((id: string) => {
    setExpandedRows((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }, []);

  // Toggle status filter
  const toggleStatusFilter = useCallback((status: TestStatus) => {
    setStatusFilter((prev) =>
      prev.includes(status)
        ? prev.filter((s) => s !== status)
        : [...prev, status]
    );
  }, []);

  // Sort icon
  const SortIcon = ({ field }: { field: SortField }) => {
    if (sortField !== field) return null;
    return sortOrder === "asc" ? (
      <ChevronUp className="ml-1 h-3 w-3" />
    ) : (
      <ChevronDown className="ml-1 h-3 w-3" />
    );
  };

  if (isLoading) {
    return (
      <div
        className={cn(
          "flex items-center justify-center rounded-lg border",
          className
        )}
        style={{ height: maxHeight }}
      >
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
      </div>
    );
  }

  return (
    <div className={cn("space-y-3", className)}>
      {/* Summary */}
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-sm font-medium">
          {processedResults.length} of {summary.total} tests
        </span>
        <div className="flex gap-2">
          <Badge variant="success" className="gap-1">
            <CheckCircle className="h-3 w-3" />
            {summary.passed}
          </Badge>
          <Badge variant="destructive" className="gap-1">
            <XCircle className="h-3 w-3" />
            {summary.failed}
          </Badge>
          {summary.error > 0 && (
            <Badge variant="warning" className="gap-1">
              <AlertCircle className="h-3 w-3" />
              {summary.error}
            </Badge>
          )}
          {summary.skipped > 0 && (
            <Badge variant="secondary" className="gap-1">
              <MinusCircle className="h-3 w-3" />
              {summary.skipped}
            </Badge>
          )}
        </div>
      </div>

      {/* Toolbar */}
      <div className="flex items-center gap-2">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
          <Input
            placeholder="Search tests..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-8"
          />
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="sm" className="gap-1">
              <Filter className="h-4 w-4" />
              Filter
              {statusFilter.length > 0 && (
                <Badge variant="secondary" className="ml-1 h-5 px-1">
                  {statusFilter.length}
                </Badge>
              )}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {(Object.keys(STATUS_CONFIG) as TestStatus[]).map((status) => (
              <DropdownMenuCheckboxItem
                key={status}
                checked={statusFilter.includes(status)}
                onCheckedChange={() => toggleStatusFilter(status)}
              >
                {getStatusBadge(status)}
              </DropdownMenuCheckboxItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* Table */}
      <div className="rounded-lg border">
        <ScrollArea style={{ maxHeight }}>
          <Table>
            <TableHeader className="sticky top-0 bg-background">
              <TableRow>
                <TableHead className="w-8"></TableHead>
                <TableHead
                  className="cursor-pointer select-none"
                  onClick={() => handleSort("name")}
                >
                  <div className="flex items-center">
                    Test Name
                    <SortIcon field="name" />
                  </div>
                </TableHead>
                <TableHead
                  className="w-28 cursor-pointer select-none"
                  onClick={() => handleSort("status")}
                >
                  <div className="flex items-center">
                    Status
                    <SortIcon field="status" />
                  </div>
                </TableHead>
                <TableHead
                  className="w-24 cursor-pointer select-none text-right"
                  onClick={() => handleSort("duration")}
                >
                  <div className="flex items-center justify-end">
                    Duration
                    <SortIcon field="duration" />
                  </div>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {processedResults.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={4} className="h-24 text-center">
                    No test results found
                  </TableCell>
                </TableRow>
              ) : (
                processedResults.map((result) => {
                  const isExpanded = expandedRows.has(result.id);
                  const hasDetails =
                    result.errorMessage ||
                    result.stackTrace ||
                    result.stdout ||
                    result.stderr;

                  return (
                    <Fragment key={result.id}>
                      <TableRow
                        className={cn(
                          "cursor-pointer",
                          isExpanded && "bg-muted/50"
                        )}
                        onClick={() => {
                          if (hasDetails) {
                            toggleExpanded(result.id);
                          }
                          onTestClick?.(result);
                        }}
                      >
                        <TableCell className="w-8 px-2">
                          {hasDetails && (
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-6 w-6"
                              onClick={(e) => {
                                e.stopPropagation();
                                toggleExpanded(result.id);
                              }}
                            >
                              {isExpanded ? (
                                <ChevronDown className="h-4 w-4" />
                              ) : (
                                <ChevronRight className="h-4 w-4" />
                              )}
                            </Button>
                          )}
                        </TableCell>
                        <TableCell>
                          <div>
                            <p className="font-medium">{result.testName}</p>
                            {result.suiteName && (
                              <p className="text-xs text-muted-foreground">
                                {result.suiteName}
                              </p>
                            )}
                          </div>
                        </TableCell>
                        <TableCell>{getStatusBadge(result.status)}</TableCell>
                        <TableCell className="text-right">
                          <div className="flex items-center justify-end gap-1 text-muted-foreground">
                            <Clock className="h-3 w-3" />
                            {formatDuration(result.durationMs)}
                          </div>
                        </TableCell>
                      </TableRow>
                      {isExpanded && (
                        <TableRow>
                          <TableCell colSpan={4} className="p-0">
                            <ExpandedRowContent test={result} />
                          </TableCell>
                        </TableRow>
                      )}
                    </Fragment>
                  );
                })
              )}
            </TableBody>
          </Table>
        </ScrollArea>
      </div>
    </div>
  );
}

export default TestResultsTable;
