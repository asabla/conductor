import { useState } from "react";
import {
  AlertCircle,
  CheckCircle,
  Clock,
  Cpu,
  HardDrive,
  MoreVertical,
  Plus,
  RefreshCw,
  Search,
  Server,
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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { AgentStatus } from "@/types/models";
import { cn } from "@/lib/utils";

interface AgentRow {
  id: string;
  name: string;
  status: AgentStatus;
  hostname: string;
  labels: string[];
  cpuCores: number;
  memoryGb: number;
  currentRun?: string;
  version: string;
  lastSeen: string;
}

const mockAgents: AgentRow[] = [
  {
    id: "agent_1",
    name: "agent-prod-1",
    status: "online",
    hostname: "worker-1.prod.internal",
    labels: ["production", "docker", "linux"],
    cpuCores: 8,
    memoryGb: 32,
    version: "1.2.0",
    lastSeen: "Just now",
  },
  {
    id: "agent_2",
    name: "agent-prod-2",
    status: "busy",
    hostname: "worker-2.prod.internal",
    labels: ["production", "docker", "linux"],
    cpuCores: 8,
    memoryGb: 32,
    currentRun: "api-gateway/main",
    version: "1.2.0",
    lastSeen: "Just now",
  },
  {
    id: "agent_3",
    name: "agent-prod-3",
    status: "busy",
    hostname: "worker-3.prod.internal",
    labels: ["production", "subprocess", "linux"],
    cpuCores: 4,
    memoryGb: 16,
    currentRun: "user-service/feature/auth",
    version: "1.2.0",
    lastSeen: "Just now",
  },
  {
    id: "agent_4",
    name: "agent-staging-1",
    status: "online",
    hostname: "worker-1.staging.internal",
    labels: ["staging", "docker", "linux"],
    cpuCores: 4,
    memoryGb: 16,
    version: "1.2.0",
    lastSeen: "2 seconds ago",
  },
  {
    id: "agent_5",
    name: "agent-staging-2",
    status: "draining",
    hostname: "worker-2.staging.internal",
    labels: ["staging", "subprocess", "linux"],
    cpuCores: 4,
    memoryGb: 16,
    version: "1.1.5",
    lastSeen: "5 seconds ago",
  },
  {
    id: "agent_6",
    name: "agent-dev-1",
    status: "offline",
    hostname: "worker-1.dev.internal",
    labels: ["development", "docker", "linux"],
    cpuCores: 2,
    memoryGb: 8,
    version: "1.1.0",
    lastSeen: "15 minutes ago",
  },
];

function getStatusIndicator(status: AgentStatus) {
  switch (status) {
    case "online":
      return (
        <div className="flex items-center gap-2">
          <div className="h-2 w-2 rounded-full bg-success" />
          <span className="text-sm">Online</span>
        </div>
      );
    case "busy":
      return (
        <div className="flex items-center gap-2">
          <div className="h-2 w-2 animate-pulse rounded-full bg-primary" />
          <span className="text-sm">Busy</span>
        </div>
      );
    case "draining":
      return (
        <div className="flex items-center gap-2">
          <div className="h-2 w-2 rounded-full bg-warning" />
          <span className="text-sm">Draining</span>
        </div>
      );
    case "offline":
      return (
        <div className="flex items-center gap-2">
          <div className="h-2 w-2 rounded-full bg-muted-foreground" />
          <span className="text-sm text-muted-foreground">Offline</span>
        </div>
      );
  }
}

export function AgentsPage() {
  const [searchQuery, setSearchQuery] = useState("");

  const filteredAgents = mockAgents.filter(
    (agent) =>
      agent.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      agent.hostname.toLowerCase().includes(searchQuery.toLowerCase()) ||
      agent.labels.some((label) =>
        label.toLowerCase().includes(searchQuery.toLowerCase())
      )
  );

  const onlineCount = mockAgents.filter(
    (a) => a.status === "online" || a.status === "busy"
  ).length;
  const busyCount = mockAgents.filter((a) => a.status === "busy").length;
  const offlineCount = mockAgents.filter((a) => a.status === "offline").length;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Agents</h1>
          <p className="text-muted-foreground">
            Manage and monitor test execution agents
          </p>
        </div>
        <Button>
          <Plus className="mr-2 h-4 w-4" />
          Register Agent
        </Button>
      </div>

      {/* Summary Cards */}
      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Total Agents</CardTitle>
            <Server className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{mockAgents.length}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Online</CardTitle>
            <CheckCircle className="h-4 w-4 text-success" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{onlineCount}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Busy</CardTitle>
            <RefreshCw className="h-4 w-4 text-primary" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{busyCount}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Offline</CardTitle>
            <AlertCircle className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{offlineCount}</div>
          </CardContent>
        </Card>
      </div>

      {/* Search */}
      <div className="flex items-center gap-2">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
          <Input
            placeholder="Search agents..."
            className="pl-8"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
        </div>
      </div>

      {/* Agent Cards */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        {filteredAgents.map((agent) => (
          <Card
            key={agent.id}
            className={cn(
              "transition-colors",
              agent.status === "offline" && "opacity-60"
            )}
          >
            <CardHeader className="pb-3">
              <div className="flex items-start justify-between">
                <div className="space-y-1">
                  <CardTitle className="text-lg">{agent.name}</CardTitle>
                  <CardDescription>{agent.hostname}</CardDescription>
                </div>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" size="icon">
                      <MoreVertical className="h-4 w-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem>View Details</DropdownMenuItem>
                    <DropdownMenuItem>View Logs</DropdownMenuItem>
                    <DropdownMenuSeparator />
                    {agent.status !== "draining" && agent.status !== "offline" && (
                      <DropdownMenuItem>Drain</DropdownMenuItem>
                    )}
                    <DropdownMenuItem className="text-destructive">
                      Remove
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              {/* Status */}
              <div className="flex items-center justify-between">
                {getStatusIndicator(agent.status)}
                <Badge variant="outline" className="font-mono text-xs">
                  v{agent.version}
                </Badge>
              </div>

              {/* Current Run */}
              {agent.currentRun && (
                <div className="rounded-md bg-primary/10 p-2 text-sm">
                  <div className="flex items-center gap-2">
                    <RefreshCw className="h-3 w-3 animate-spin text-primary" />
                    <span className="font-medium">Running:</span>
                    <span className="text-muted-foreground">{agent.currentRun}</span>
                  </div>
                </div>
              )}

              {/* Resources */}
              <div className="flex items-center gap-4 text-sm text-muted-foreground">
                <div className="flex items-center gap-1">
                  <Cpu className="h-4 w-4" />
                  {agent.cpuCores} cores
                </div>
                <div className="flex items-center gap-1">
                  <HardDrive className="h-4 w-4" />
                  {agent.memoryGb} GB
                </div>
              </div>

              {/* Labels */}
              <div className="flex flex-wrap gap-1">
                {agent.labels.map((label) => (
                  <Badge key={label} variant="secondary" className="text-xs">
                    {label}
                  </Badge>
                ))}
              </div>

              {/* Last Seen */}
              <div className="flex items-center gap-1 text-xs text-muted-foreground">
                <Clock className="h-3 w-3" />
                Last seen {agent.lastSeen}
              </div>
            </CardContent>
          </Card>
        ))}
      </div>

      {filteredAgents.length === 0 && (
        <Card>
          <CardContent className="flex flex-col items-center justify-center py-12">
            <Server className="h-12 w-12 text-muted-foreground" />
            <h3 className="mt-4 text-lg font-medium">No agents found</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              No agents match your search criteria.
            </p>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
