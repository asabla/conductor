/**
 * API error response type
 */
export interface ApiError {
  message: string;
  code: string;
  status: number;
  details?: Record<string, unknown>;
}

/**
 * Paginated response wrapper
 */
export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
  totalPages: number;
}

/**
 * API request options
 */
export interface RequestOptions {
  signal?: AbortSignal;
}

/**
 * Pagination parameters
 */
export interface PaginationParams {
  page?: number;
  pageSize?: number;
}

/**
 * Sort parameters
 */
export interface SortParams {
  sortBy?: string;
  sortOrder?: "asc" | "desc";
}

/**
 * Filter parameters for test runs
 */
export interface TestRunFilterParams extends PaginationParams, SortParams {
  status?: string;
  agentId?: string;
  repositoryId?: string;
  branch?: string;
  startDate?: string;
  endDate?: string;
}

/**
 * Filter parameters for agents
 */
export interface AgentFilterParams extends PaginationParams, SortParams {
  status?: string;
  label?: string;
}
