/**
 * Failure Analysis page
 * Groups and analyzes test failures by error signature
 */

import { useState, useMemo } from "react";
import { AlertCircle, Filter, RefreshCw } from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { FailureGroupCard } from "@/components/analytics/failure-group-card";
import { useFailureGroups } from "@/hooks/use-analytics";
import { useServices } from "@/hooks/use-services";
import type { FailureGroupsParams } from "@/api/analytics";

// =============================================================================
// Types
// =============================================================================

type TimeRange = "24h" | "7d" | "30d";

// =============================================================================
// Constants
// =============================================================================

const TIME_RANGES: { value: TimeRange; label: string }[] = [
  { value: "24h", label: "24 hours" },
  { value: "7d", label: "7 days" },
  { value: "30d", label: "30 days" },
];

// =============================================================================
// Component
// =============================================================================

export function FailureAnalysisPage() {
  // Filter state
  const [timeRange, setTimeRange] = useState<TimeRange>("7d");
  const [selectedService, setSelectedService] = useState<string | undefined>();
  const [searchQuery, setSearchQuery] = useState("");

  // Build API params
  const params: FailureGroupsParams = useMemo(
    () => ({
      timeRange,
      serviceId: selectedService,
      pageSize: 50,
    }),
    [timeRange, selectedService]
  );

  // Fetch data
  const {
    data: failureData,
    isLoading,
    isError,
    error,
    refetch,
    isFetching,
  } = useFailureGroups(params);

  const { data: servicesData } = useServices({});

  // Filter groups by search query
  const filteredGroups = useMemo(() => {
    if (!failureData?.groups) return [];
    if (!searchQuery) return failureData.groups;

    const query = searchQuery.toLowerCase();
    return failureData.groups.filter(
      (group) =>
        group.errorSignature.toLowerCase().includes(query) ||
        group.errorMessage.toLowerCase().includes(query) ||
        group.affectedTests.some((test) =>
          test.testName.toLowerCase().includes(query)
        )
    );
  }, [failureData?.groups, searchQuery]);

  // Calculate summary stats
  const summaryStats = useMemo(() => {
    if (!failureData?.groups) {
      return { totalGroups: 0, totalOccurrences: 0, affectedServices: 0 };
    }

    const allServices = new Set<string>();
    let totalOccurrences = 0;

    failureData.groups.forEach((group) => {
      totalOccurrences += group.count;
      group.services.forEach((s) => allServices.add(s));
    });

    return {
      totalGroups: failureData.groups.length,
      totalOccurrences,
      affectedServices: allServices.size,
    };
  }, [failureData?.groups]);

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Failure Analysis</h1>
          <p className="text-muted-foreground">
            Analyze and group test failures by error signature
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => refetch()}
          disabled={isFetching}
        >
          <RefreshCw className={`mr-2 h-4 w-4 ${isFetching ? "animate-spin" : ""}`} />
          Refresh
        </Button>
      </div>

      {/* Summary Stats */}
      <div className="grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Failure Groups
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {isLoading ? (
                <Skeleton className="h-8 w-16" />
              ) : (
                summaryStats.totalGroups
              )}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Total Occurrences
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {isLoading ? (
                <Skeleton className="h-8 w-20" />
              ) : (
                summaryStats.totalOccurrences.toLocaleString()
              )}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Affected Services
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {isLoading ? (
                <Skeleton className="h-8 w-12" />
              ) : (
                summaryStats.affectedServices
              )}
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Filters */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-base">
            <Filter className="h-4 w-4" />
            Filters
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap items-center gap-4">
            {/* Time range filter */}
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">Time range:</span>
              <div className="flex gap-1">
                {TIME_RANGES.map((range) => (
                  <Button
                    key={range.value}
                    variant={timeRange === range.value ? "default" : "outline"}
                    size="sm"
                    onClick={() => setTimeRange(range.value)}
                  >
                    {range.label}
                  </Button>
                ))}
              </div>
            </div>

            {/* Service filter */}
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">Service:</span>
              <div className="flex gap-1 flex-wrap">
                <Button
                  variant={!selectedService ? "default" : "outline"}
                  size="sm"
                  onClick={() => setSelectedService(undefined)}
                >
                  All
                </Button>
                {servicesData?.items.slice(0, 5).map((service) => (
                  <Button
                    key={service.id}
                    variant={selectedService === service.id ? "default" : "outline"}
                    size="sm"
                    onClick={() => setSelectedService(service.id)}
                  >
                    {service.name}
                  </Button>
                ))}
              </div>
            </div>

            {/* Search */}
            <div className="flex-1 min-w-[200px]">
              <Input
                placeholder="Search error messages..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="max-w-sm"
              />
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Error State */}
      {isError && (
        <Card className="border-destructive">
          <CardContent className="flex items-center gap-4 py-6">
            <AlertCircle className="h-8 w-8 text-destructive" />
            <div>
              <h3 className="font-semibold">Failed to load failure data</h3>
              <p className="text-sm text-muted-foreground">
                {error instanceof Error ? error.message : "An unknown error occurred"}
              </p>
            </div>
            <Button variant="outline" onClick={() => refetch()} className="ml-auto">
              Retry
            </Button>
          </CardContent>
        </Card>
      )}

      {/* Loading State */}
      {isLoading && (
        <div className="space-y-4">
          {[1, 2, 3].map((i) => (
            <Card key={i}>
              <CardHeader>
                <Skeleton className="h-5 w-3/4" />
                <Skeleton className="h-4 w-1/2" />
              </CardHeader>
              <CardContent>
                <Skeleton className="h-20 w-full" />
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {/* Failure Groups */}
      {!isLoading && !isError && (
        <>
          {filteredGroups.length === 0 ? (
            <Card>
              <CardContent className="flex flex-col items-center justify-center py-12">
                <AlertCircle className="h-12 w-12 text-muted-foreground mb-4" />
                <CardTitle className="text-lg mb-2">No failures found</CardTitle>
                <CardDescription>
                  {searchQuery
                    ? "No failures match your search criteria"
                    : `No failures in the last ${timeRange}`}
                </CardDescription>
              </CardContent>
            </Card>
          ) : (
            <div className="space-y-4">
              {/* Results count */}
              <div className="flex items-center gap-2">
                <Badge variant="secondary">{filteredGroups.length} groups</Badge>
                {searchQuery && (
                  <span className="text-sm text-muted-foreground">
                    matching "{searchQuery}"
                  </span>
                )}
              </div>

              {/* Failure group cards */}
              {filteredGroups.map((group) => (
                <FailureGroupCard key={group.id} group={group} />
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}

export default FailureAnalysisPage;
