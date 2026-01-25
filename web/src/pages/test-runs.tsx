import { useState } from "react";
import {
  ExternalLink,
  Filter,
  GitBranch,
  MoreVertical,
  RefreshCw,
  Search,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
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
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { TestRunStatus } from "@/types/models";

interface TestRunRow {
  id: string;
  repository: string;
  branch: string;
  commit: string;
  status: TestRunStatus;
  agent: string;
  duration: string;
  tests: {
    passed: number;
    failed: number;
    skipped: number;
  };
  startedAt: string;
}

const mockTestRuns: TestRunRow[] = [
  {
    id: "run_1",
    repository: "api-gateway",
    branch: "main",
    commit: "a1b2c3d",
    status: "passed",
    agent: "agent-1",
    duration: "2m 34s",
    tests: { passed: 142, failed: 0, skipped: 3 },
    startedAt: "5 minutes ago",
  },
  {
    id: "run_2",
    repository: "user-service",
    branch: "feature/auth",
    commit: "e4f5g6h",
    status: "failed",
    agent: "agent-2",
    duration: "1m 12s",
    tests: { passed: 85, failed: 3, skipped: 0 },
    startedAt: "12 minutes ago",
  },
  {
    id: "run_3",
    repository: "payment-service",
    branch: "main",
    commit: "i7j8k9l",
    status: "running",
    agent: "agent-3",
    duration: "45s",
    tests: { passed: 23, failed: 0, skipped: 0 },
    startedAt: "15 minutes ago",
  },
  {
    id: "run_4",
    repository: "notification-service",
    branch: "develop",
    commit: "m0n1o2p",
    status: "passed",
    agent: "agent-1",
    duration: "3m 45s",
    tests: { passed: 67, failed: 0, skipped: 2 },
    startedAt: "23 minutes ago",
  },
  {
    id: "run_5",
    repository: "api-gateway",
    branch: "hotfix/security",
    commit: "q3r4s5t",
    status: "passed",
    agent: "agent-4",
    duration: "2m 10s",
    tests: { passed: 142, failed: 0, skipped: 3 },
    startedAt: "45 minutes ago",
  },
  {
    id: "run_6",
    repository: "inventory-service",
    branch: "main",
    commit: "u6v7w8x",
    status: "cancelled",
    agent: "agent-2",
    duration: "30s",
    tests: { passed: 12, failed: 0, skipped: 45 },
    startedAt: "1 hour ago",
  },
  {
    id: "run_7",
    repository: "order-service",
    branch: "feature/bulk-orders",
    commit: "y9z0a1b",
    status: "timed_out",
    agent: "agent-5",
    duration: "30m 0s",
    tests: { passed: 89, failed: 0, skipped: 0 },
    startedAt: "2 hours ago",
  },
];

function getStatusBadge(status: TestRunStatus) {
  switch (status) {
    case "passed":
      return <Badge variant="success">Passed</Badge>;
    case "failed":
      return <Badge variant="destructive">Failed</Badge>;
    case "running":
      return (
        <Badge variant="secondary">
          <RefreshCw className="mr-1 h-3 w-3 animate-spin" />
          Running
        </Badge>
      );
    case "pending":
      return <Badge variant="outline">Pending</Badge>;
    case "queued":
      return <Badge variant="outline">Queued</Badge>;
    case "cancelled":
      return <Badge variant="secondary">Cancelled</Badge>;
    case "timed_out":
      return <Badge variant="warning">Timed Out</Badge>;
  }
}

export function TestRunsPage() {
  const [searchQuery, setSearchQuery] = useState("");

  const filteredRuns = mockTestRuns.filter(
    (run) =>
      run.repository.toLowerCase().includes(searchQuery.toLowerCase()) ||
      run.branch.toLowerCase().includes(searchQuery.toLowerCase())
  );

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Test Runs</h1>
          <p className="text-muted-foreground">
            View and manage test executions across all repositories
          </p>
        </div>
        <Button>
          <RefreshCw className="mr-2 h-4 w-4" />
          Trigger Run
        </Button>
      </div>

      <Tabs defaultValue="all" className="space-y-4">
        <div className="flex items-center justify-between">
          <TabsList>
            <TabsTrigger value="all">All</TabsTrigger>
            <TabsTrigger value="running">Running</TabsTrigger>
            <TabsTrigger value="passed">Passed</TabsTrigger>
            <TabsTrigger value="failed">Failed</TabsTrigger>
          </TabsList>

          <div className="flex items-center gap-2">
            <div className="relative">
              <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder="Search runs..."
                className="w-64 pl-8"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
            </div>
            <Button variant="outline" size="icon">
              <Filter className="h-4 w-4" />
            </Button>
          </div>
        </div>

        <TabsContent value="all" className="space-y-4">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle>Test Runs</CardTitle>
              <CardDescription>
                {filteredRuns.length} runs found
              </CardDescription>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Repository</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Tests</TableHead>
                    <TableHead>Agent</TableHead>
                    <TableHead>Duration</TableHead>
                    <TableHead>Started</TableHead>
                    <TableHead className="w-[50px]"></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredRuns.map((run) => (
                    <TableRow key={run.id}>
                      <TableCell>
                        <div className="space-y-1">
                          <div className="font-medium">{run.repository}</div>
                          <div className="flex items-center gap-1 text-xs text-muted-foreground">
                            <GitBranch className="h-3 w-3" />
                            {run.branch}
                            <span className="text-muted-foreground/50">·</span>
                            <code className="rounded bg-muted px-1">
                              {run.commit}
                            </code>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell>{getStatusBadge(run.status)}</TableCell>
                      <TableCell>
                        <div className="flex items-center gap-2 text-sm">
                          <span className="text-success">{run.tests.passed}</span>
                          <span className="text-muted-foreground">/</span>
                          <span className="text-destructive">{run.tests.failed}</span>
                          <span className="text-muted-foreground">/</span>
                          <span className="text-muted-foreground">{run.tests.skipped}</span>
                        </div>
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {run.agent}
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {run.duration}
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {run.startedAt}
                      </TableCell>
                      <TableCell>
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button variant="ghost" size="icon">
                              <MoreVertical className="h-4 w-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            <DropdownMenuItem>
                              <ExternalLink className="mr-2 h-4 w-4" />
                              View Details
                            </DropdownMenuItem>
                            <DropdownMenuItem>View Logs</DropdownMenuItem>
                            <DropdownMenuItem>Download Artifacts</DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem>Re-run</DropdownMenuItem>
                            {run.status === "running" && (
                              <DropdownMenuItem className="text-destructive">
                                Cancel
                              </DropdownMenuItem>
                            )}
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="running">
          <Card>
            <CardHeader>
              <CardTitle>Running Tests</CardTitle>
              <CardDescription>Currently executing test runs</CardDescription>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground">
                Filter by running status to see active runs.
              </p>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="passed">
          <Card>
            <CardHeader>
              <CardTitle>Passed Tests</CardTitle>
              <CardDescription>Successfully completed test runs</CardDescription>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground">
                Filter by passed status to see successful runs.
              </p>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="failed">
          <Card>
            <CardHeader>
              <CardTitle>Failed Tests</CardTitle>
              <CardDescription>Test runs with failures</CardDescription>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground">
                Filter by failed status to see runs with failures.
              </p>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
