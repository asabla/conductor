/**
 * Services List Page
 * Shows all registered services with health indicators and stats
 */

import { useState, useMemo } from "react";
import { Link } from "@tanstack/react-router";
import {
  Search,
  GitBranch,
  ExternalLink,
  MoreVertical,
  RefreshCw,
  Plus,
  Users,
  Globe,
  TrendingUp,
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
import { Skeleton } from "@/components/ui/skeleton";
import { useServices, type Service, type RecentRunSummary } from "@/hooks/use-services";
import { formatRelativeTime } from "@/lib/utils";
import type { TestRunStatus } from "@/types/models";

// =============================================================================
// Types
// =============================================================================

interface ServiceWithMeta extends Service {
  recentRuns?: RecentRunSummary[];
  passRate?: number;
}

// =============================================================================
// Status Helpers
// =============================================================================

function getRunStatusBadge(status: TestRunStatus) {
  switch (status) {
    case "passed":
      return <div className="h-2 w-2 rounded-full bg-success" title="Passed" />;
    case "failed":
      return <div className="h-2 w-2 rounded-full bg-destructive" title="Failed" />;
    case "running":
      return (
        <div
          className="h-2 w-2 animate-pulse rounded-full bg-primary"
          title="Running"
        />
      );
    default:
      return <div className="h-2 w-2 rounded-full bg-muted" title={status} />;
  }
}

function getPassRateBadge(passRate: number | undefined) {
  if (passRate === undefined || passRate === null) {
    return <Badge variant="outline">No data</Badge>;
  }

  if (passRate >= 95) {
    return (
      <Badge variant="success" className="gap-1">
        <TrendingUp className="h-3 w-3" />
        {passRate.toFixed(1)}%
      </Badge>
    );
  }
  if (passRate >= 80) {
    return (
      <Badge variant="warning" className="gap-1">
        {passRate.toFixed(1)}%
      </Badge>
    );
  }
  return (
    <Badge variant="destructive" className="gap-1">
      {passRate.toFixed(1)}%
    </Badge>
  );
}

function getHealthIndicator(recentRuns?: RecentRunSummary[]) {
  if (!recentRuns || recentRuns.length === 0) {
    return (
      <div className="flex items-center gap-1">
        <div className="h-3 w-3 rounded-full bg-muted" />
        <span className="text-xs text-muted-foreground">No runs</span>
      </div>
    );
  }

  const lastRun = recentRuns[0]!;
  const recentFailures = recentRuns.filter((r) => r.status === "failed").length;

  if (lastRun.status === "passed" && recentFailures === 0) {
    return (
      <div className="flex items-center gap-1">
        <div className="h-3 w-3 rounded-full bg-success" />
        <span className="text-xs text-success">Healthy</span>
      </div>
    );
  }

  if (lastRun.status === "failed" || recentFailures > 2) {
    return (
      <div className="flex items-center gap-1">
        <div className="h-3 w-3 rounded-full bg-destructive" />
        <span className="text-xs text-destructive">Failing</span>
      </div>
    );
  }

  return (
    <div className="flex items-center gap-1">
      <div className="h-3 w-3 rounded-full bg-warning" />
      <span className="text-xs text-warning">Unstable</span>
    </div>
  );
}

// =============================================================================
// Service Card Component
// =============================================================================

interface ServiceCardProps {
  service: ServiceWithMeta;
  onSync?: () => void;
}

function ServiceCard({ service, onSync }: ServiceCardProps) {
  const recentRuns = service.recentRuns?.slice(0, 5) || [];

  return (
    <Card className="transition-shadow hover:shadow-md">
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between">
          <div className="flex-1">
            <Link to="/services/$serviceId" params={{ serviceId: service.id }}>
              <CardTitle className="flex items-center gap-2 text-lg hover:text-primary">
                {service.name}
                <ExternalLink className="h-4 w-4 opacity-0 transition-opacity group-hover:opacity-100" />
              </CardTitle>
            </Link>
            <CardDescription className="mt-1 flex items-center gap-2">
              <Users className="h-3 w-3" />
              {service.owner}
            </CardDescription>
          </div>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon" className="h-8 w-8">
                <MoreVertical className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem asChild>
                <Link to="/services/$serviceId" params={{ serviceId: service.id }}>View Details</Link>
              </DropdownMenuItem>
              <DropdownMenuItem onClick={onSync}>
                <RefreshCw className="mr-2 h-4 w-4" />
                Sync Tests
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem asChild>
                <a href={service.gitUrl} target="_blank" rel="noopener noreferrer">
                  <GitBranch className="mr-2 h-4 w-4" />
                  Open Repository
                </a>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Health and Pass Rate */}
        <div className="flex items-center justify-between">
          {getHealthIndicator(recentRuns)}
          {getPassRateBadge(service.passRate)}
        </div>

        {/* Recent Runs */}
        <div>
          <p className="mb-2 text-xs font-medium text-muted-foreground">
            Recent Runs
          </p>
          <div className="flex items-center gap-1">
            {recentRuns.length > 0 ? (
              recentRuns.map((run) => (
                <Link key={run.id} to="/test-runs/$runId" params={{ runId: run.id }}>
                  {getRunStatusBadge(run.status)}
                </Link>
              ))
            ) : (
              <span className="text-xs text-muted-foreground">No recent runs</span>
            )}
          </div>
        </div>

        {/* Network Zones */}
        {service.networkZones && service.networkZones.length > 0 && (
          <div className="flex flex-wrap gap-1">
            {service.networkZones.map((zone) => (
              <Badge key={zone} variant="outline" className="text-xs">
                <Globe className="mr-1 h-3 w-3" />
                {zone}
              </Badge>
            ))}
          </div>
        )}

        {/* Test Count */}
        <div className="flex items-center justify-between text-sm">
          <span className="text-muted-foreground">Tests</span>
          <span className="font-medium">{service.testCount}</span>
        </div>
      </CardContent>
    </Card>
  );
}

// =============================================================================
// Main Page Component
// =============================================================================

export function ServicesPage() {
  const [searchQuery, setSearchQuery] = useState("");
  const [ownerFilter, setOwnerFilter] = useState<string>("");
  const [networkZoneFilter, setNetworkZoneFilter] = useState<string>("");
  const [viewMode, setViewMode] = useState<"cards" | "table">("cards");

  // Fetch services
  const { data: servicesData, isLoading } = useServices({
    query: searchQuery || undefined,
    owner: ownerFilter || undefined,
    networkZone: networkZoneFilter || undefined,
  });

  // Extract unique owners and network zones for filters
  const { owners, networkZones } = useMemo(() => {
    const services = servicesData?.items || [];
    const ownersSet = new Set<string>();
    const zonesSet = new Set<string>();

    services.forEach((service) => {
      if (service.owner) ownersSet.add(service.owner);
      service.networkZones?.forEach((zone) => zonesSet.add(zone));
    });

    return {
      owners: Array.from(ownersSet).sort(),
      networkZones: Array.from(zonesSet).sort(),
    };
  }, [servicesData?.items]);

  const services = servicesData?.items || [];

  // Loading state
  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <Skeleton className="h-8 w-32" />
            <Skeleton className="mt-2 h-4 w-64" />
          </div>
          <Skeleton className="h-10 w-32" />
        </div>
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {[1, 2, 3, 4, 5, 6].map((i) => (
            <Skeleton key={i} className="h-52" />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Services</h1>
          <p className="text-muted-foreground">
            Manage and monitor your test services
          </p>
        </div>
        <Button>
          <Plus className="mr-2 h-4 w-4" />
          Add Service
        </Button>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
          <Input
            placeholder="Search services..."
            className="pl-8"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
        </div>

        <select
          value={ownerFilter}
          onChange={(e) => setOwnerFilter(e.target.value)}
          className="flex h-10 rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <option value="">All Owners</option>
          {owners.map((owner) => (
            <option key={owner} value={owner}>
              {owner}
            </option>
          ))}
        </select>

        <select
          value={networkZoneFilter}
          onChange={(e) => setNetworkZoneFilter(e.target.value)}
          className="flex h-10 rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <option value="">All Zones</option>
          {networkZones.map((zone) => (
            <option key={zone} value={zone}>
              {zone}
            </option>
          ))}
        </select>

        <div className="ml-auto flex gap-1">
          <Button
            variant={viewMode === "cards" ? "default" : "outline"}
            size="sm"
            onClick={() => setViewMode("cards")}
          >
            Cards
          </Button>
          <Button
            variant={viewMode === "table" ? "default" : "outline"}
            size="sm"
            onClick={() => setViewMode("table")}
          >
            Table
          </Button>
        </div>
      </div>

      {/* Services Grid/Table */}
      {services.length === 0 ? (
        <Card>
          <CardContent className="flex flex-col items-center justify-center py-12">
            <GitBranch className="h-12 w-12 text-muted-foreground" />
            <h3 className="mt-4 text-lg font-medium">No services found</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              {searchQuery || ownerFilter || networkZoneFilter
                ? "Try adjusting your filters"
                : "Get started by adding your first service"}
            </p>
            {!searchQuery && !ownerFilter && !networkZoneFilter && (
              <Button className="mt-4">
                <Plus className="mr-2 h-4 w-4" />
                Add Service
              </Button>
            )}
          </CardContent>
        </Card>
      ) : viewMode === "cards" ? (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {services.map((service) => (
            <ServiceCard key={service.id} service={service as ServiceWithMeta} />
          ))}
        </div>
      ) : (
        <Card>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Service</TableHead>
                <TableHead>Owner</TableHead>
                <TableHead>Health</TableHead>
                <TableHead>Pass Rate</TableHead>
                <TableHead>Tests</TableHead>
                <TableHead>Last Updated</TableHead>
                <TableHead className="w-[50px]"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {services.map((service) => (
                <TableRow key={service.id}>
                  <TableCell>
                    <Link
                      to="/services/$serviceId"
                      params={{ serviceId: service.id }}
                      className="font-medium hover:text-primary"
                    >
                      {service.name}
                    </Link>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {service.owner}
                  </TableCell>
                  <TableCell>
                    {getHealthIndicator((service as ServiceWithMeta).recentRuns)}
                  </TableCell>
                  <TableCell>
                    {getPassRateBadge((service as ServiceWithMeta).passRate)}
                  </TableCell>
                  <TableCell>{service.testCount}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatRelativeTime(service.updatedAt)}
                  </TableCell>
                  <TableCell>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon">
                          <MoreVertical className="h-4 w-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem asChild>
                          <Link to="/services/$serviceId" params={{ serviceId: service.id }}>View Details</Link>
                        </DropdownMenuItem>
                        <DropdownMenuItem>
                          <RefreshCw className="mr-2 h-4 w-4" />
                          Sync Tests
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem asChild>
                          <a
                            href={service.gitUrl}
                            target="_blank"
                            rel="noopener noreferrer"
                          >
                            <GitBranch className="mr-2 h-4 w-4" />
                            Open Repository
                          </a>
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}
    </div>
  );
}

export default ServicesPage;
