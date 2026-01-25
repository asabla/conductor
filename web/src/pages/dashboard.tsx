import {
  Activity,
  ArrowDownRight,
  ArrowUpRight,
  Clock,
  Server,
  TrendingUp,
} from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

interface StatCardProps {
  title: string;
  value: string | number;
  description?: string;
  trend?: number;
  icon: React.ReactNode;
}

function StatCard({ title, value, description, trend, icon }: StatCardProps) {
  const isPositive = trend && trend > 0;
  const isNegative = trend && trend < 0;

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium">{title}</CardTitle>
        <div className="text-muted-foreground">{icon}</div>
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-bold">{value}</div>
        {(description || trend !== undefined) && (
          <p className="text-xs text-muted-foreground">
            {trend !== undefined && (
              <span
                className={cn(
                  "inline-flex items-center",
                  isPositive && "text-success",
                  isNegative && "text-destructive"
                )}
              >
                {isPositive ? (
                  <ArrowUpRight className="mr-1 h-3 w-3" />
                ) : isNegative ? (
                  <ArrowDownRight className="mr-1 h-3 w-3" />
                ) : null}
                {Math.abs(trend)}%
              </span>
            )}
            {description && (
              <span className="ml-1">{description}</span>
            )}
          </p>
        )}
      </CardContent>
    </Card>
  );
}

interface RecentRun {
  id: string;
  repository: string;
  branch: string;
  status: "passed" | "failed" | "running";
  duration: string;
  time: string;
}

const recentRuns: RecentRun[] = [
  {
    id: "1",
    repository: "api-gateway",
    branch: "main",
    status: "passed",
    duration: "2m 34s",
    time: "5 minutes ago",
  },
  {
    id: "2",
    repository: "user-service",
    branch: "feature/auth",
    status: "failed",
    duration: "1m 12s",
    time: "12 minutes ago",
  },
  {
    id: "3",
    repository: "payment-service",
    branch: "main",
    status: "running",
    duration: "45s",
    time: "15 minutes ago",
  },
  {
    id: "4",
    repository: "notification-service",
    branch: "develop",
    status: "passed",
    duration: "3m 45s",
    time: "23 minutes ago",
  },
  {
    id: "5",
    repository: "api-gateway",
    branch: "hotfix/security",
    status: "passed",
    duration: "2m 10s",
    time: "45 minutes ago",
  },
];

function getStatusBadge(status: RecentRun["status"]) {
  switch (status) {
    case "passed":
      return <Badge variant="success">Passed</Badge>;
    case "failed":
      return <Badge variant="destructive">Failed</Badge>;
    case "running":
      return <Badge variant="secondary">Running</Badge>;
  }
}

export function DashboardPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Dashboard</h1>
        <p className="text-muted-foreground">
          Overview of your test orchestration system
        </p>
      </div>

      {/* Stats Grid */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <StatCard
          title="Total Runs (24h)"
          value={247}
          trend={12}
          description="from yesterday"
          icon={<Activity className="h-4 w-4" />}
        />
        <StatCard
          title="Pass Rate"
          value="94.2%"
          trend={2.1}
          description="from last week"
          icon={<TrendingUp className="h-4 w-4" />}
        />
        <StatCard
          title="Active Agents"
          value="8/12"
          description="agents online"
          icon={<Server className="h-4 w-4" />}
        />
        <StatCard
          title="Avg Duration"
          value="2m 45s"
          trend={-5}
          description="faster than avg"
          icon={<Clock className="h-4 w-4" />}
        />
      </div>

      {/* Recent Runs */}
      <Card>
        <CardHeader>
          <CardTitle>Recent Test Runs</CardTitle>
          <CardDescription>
            Latest test executions across all repositories
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {recentRuns.map((run) => (
              <div
                key={run.id}
                className="flex items-center justify-between rounded-lg border p-4"
              >
                <div className="space-y-1">
                  <p className="font-medium">{run.repository}</p>
                  <p className="text-sm text-muted-foreground">
                    {run.branch} · {run.time}
                  </p>
                </div>
                <div className="flex items-center gap-4">
                  <span className="text-sm text-muted-foreground">
                    {run.duration}
                  </span>
                  {getStatusBadge(run.status)}
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
