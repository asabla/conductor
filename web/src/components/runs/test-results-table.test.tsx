/**
 * Tests for TestResultsTable component
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TestResultsTable, TestResultRow, TestStatus } from "./test-results-table";

// =============================================================================
// Mock Data Factory
// =============================================================================

function createTestResult(overrides: Partial<TestResultRow> = {}): TestResultRow {
  return {
    id: `test-${Math.random().toString(36).substring(7)}`,
    testName: "Test Example",
    status: "passed",
    durationMs: 150,
    ...overrides,
  };
}

function createMockResults(): TestResultRow[] {
  return [
    createTestResult({
      id: "1",
      testName: "should handle user login",
      suiteName: "AuthSuite",
      status: "passed",
      durationMs: 150,
    }),
    createTestResult({
      id: "2",
      testName: "should display dashboard",
      suiteName: "DashboardSuite",
      status: "passed",
      durationMs: 320,
    }),
    createTestResult({
      id: "3",
      testName: "should validate form inputs",
      status: "failed",
      durationMs: 450,
      errorMessage: "Expected true but got false",
      stackTrace: "at FormValidator.validate (form.ts:42)\nat test (form.test.ts:15)",
    }),
    createTestResult({
      id: "4",
      testName: "should connect to database",
      status: "error",
      durationMs: 5200,
      errorMessage: "Connection timeout",
      stderr: "Error: ECONNREFUSED 127.0.0.1:5432",
    }),
    createTestResult({
      id: "5",
      testName: "should handle edge case",
      status: "skipped",
      durationMs: 0,
    }),
  ];
}

// =============================================================================
// Test Suite
// =============================================================================

describe("TestResultsTable", () => {
  // ===========================================================================
  // 1. Loading State
  // ===========================================================================
  describe("Loading State", () => {
    it("shows spinner when isLoading is true", () => {
      render(<TestResultsTable results={[]} isLoading={true} />);
      
      // The loading spinner has animate-spin class
      const spinner = document.querySelector(".animate-spin");
      expect(spinner).toBeInTheDocument();
    });

    it("does not show table when loading", () => {
      render(<TestResultsTable results={createMockResults()} isLoading={true} />);
      
      expect(screen.queryByRole("table")).not.toBeInTheDocument();
    });
  });

  // ===========================================================================
  // 2. Empty State
  // ===========================================================================
  describe("Empty State", () => {
    it("shows 'No test results found' when results array is empty", () => {
      render(<TestResultsTable results={[]} />);
      
      expect(screen.getByText("No test results found")).toBeInTheDocument();
    });

    it("shows 'No test results found' when search yields no results", () => {
      render(<TestResultsTable results={createMockResults()} />);
      
      const searchInput = screen.getByPlaceholderText("Search tests...");
      fireEvent.change(searchInput, { target: { value: "nonexistent-test-name-xyz" } });
      
      expect(screen.getByText("No test results found")).toBeInTheDocument();
    });
  });

  // ===========================================================================
  // 3. Rendering Results
  // ===========================================================================
  describe("Rendering Results", () => {
    it("renders test names correctly", () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      expect(screen.getByText("should handle user login")).toBeInTheDocument();
      expect(screen.getByText("should display dashboard")).toBeInTheDocument();
      expect(screen.getByText("should validate form inputs")).toBeInTheDocument();
      expect(screen.getByText("should connect to database")).toBeInTheDocument();
      expect(screen.getByText("should handle edge case")).toBeInTheDocument();
    });

    it("renders suite names when provided", () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      expect(screen.getByText("AuthSuite")).toBeInTheDocument();
      expect(screen.getByText("DashboardSuite")).toBeInTheDocument();
    });

    it("renders status badges with correct labels", () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      // Check for status badge labels in table rows
      const passedBadges = screen.getAllByText("Passed");
      expect(passedBadges.length).toBeGreaterThanOrEqual(2); // 2 passed tests
      
      expect(screen.getByText("Failed")).toBeInTheDocument();
      expect(screen.getByText("Error")).toBeInTheDocument();
      expect(screen.getByText("Skipped")).toBeInTheDocument();
    });

    it("renders formatted durations", () => {
      const results = [
        createTestResult({ id: "1", testName: "Fast test", durationMs: 150 }),
        createTestResult({ id: "2", testName: "Medium test", durationMs: 2500 }),
        createTestResult({ id: "3", testName: "Slow test", durationMs: 65000 }),
      ];
      render(<TestResultsTable results={results} />);
      
      // formatDuration: 150ms -> "150ms", 2500ms -> "2s", 65000ms -> "1m 5s"
      expect(screen.getByText("150ms")).toBeInTheDocument();
      expect(screen.getByText("2s")).toBeInTheDocument();
      expect(screen.getByText("1m 5s")).toBeInTheDocument();
    });

    it("applies custom className", () => {
      const { container } = render(
        <TestResultsTable results={createMockResults()} className="custom-class" />
      );
      
      expect(container.firstChild).toHaveClass("custom-class");
    });
  });

  // ===========================================================================
  // 4. Summary Counts
  // ===========================================================================
  describe("Summary Counts", () => {
    it("displays correct total count", () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      expect(screen.getByText(/5 of 5 tests/)).toBeInTheDocument();
    });

    it("displays correct passed count in summary badge", () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      // Find summary badges - they're in the flex gap-2 container
      // The summary shows icon + count, we look for the success badge with "2"
      const successBadges = document.querySelectorAll('[class*="bg-success"]');
      expect(successBadges.length).toBeGreaterThan(0);
      
      // Check the first success badge in summary (should contain "2")
      const summarySuccessBadge = successBadges[0];
      expect(summarySuccessBadge.textContent).toContain("2");
    });

    it("displays correct failed count", () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      // Find the destructive badge in summary (should contain "1")
      const destructiveBadges = document.querySelectorAll('[class*="bg-destructive"]');
      expect(destructiveBadges.length).toBeGreaterThan(0);
      
      const summaryDestructiveBadge = destructiveBadges[0];
      expect(summaryDestructiveBadge.textContent).toContain("1");
    });

    it("displays error count when errors exist", () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      // Find the warning badge in summary (should contain "1")
      const warningBadges = document.querySelectorAll('[class*="bg-warning"]');
      expect(warningBadges.length).toBeGreaterThan(0);
      
      const summaryWarningBadge = warningBadges[0];
      expect(summaryWarningBadge.textContent).toContain("1");
    });

    it("displays skipped count when skipped tests exist", () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      // Find the secondary badge in summary - need to be more specific
      // The summary badges are in the first flex gap-2 div
      const summarySection = screen.getByText(/of 5 tests/).parentElement?.parentElement;
      const secondaryBadges = summarySection?.querySelectorAll('[class*="bg-secondary"]');
      
      expect(secondaryBadges).toBeDefined();
      expect(secondaryBadges!.length).toBeGreaterThan(0);
      expect(secondaryBadges![0].textContent).toContain("1");
    });

    it("updates filtered count when search is applied", () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      const searchInput = screen.getByPlaceholderText("Search tests...");
      fireEvent.change(searchInput, { target: { value: "login" } });
      
      // Should show "1 of 5 tests"
      expect(screen.getByText(/1 of 5 tests/)).toBeInTheDocument();
    });
  });

  // ===========================================================================
  // 5. Search Filtering
  // ===========================================================================
  describe("Search Filtering", () => {
    it("filters results by test name", () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      const searchInput = screen.getByPlaceholderText("Search tests...");
      fireEvent.change(searchInput, { target: { value: "validate" } });
      
      expect(screen.getByText("should validate form inputs")).toBeInTheDocument();
      expect(screen.queryByText("should handle user login")).not.toBeInTheDocument();
    });

    it("filters results by suite name", () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      const searchInput = screen.getByPlaceholderText("Search tests...");
      fireEvent.change(searchInput, { target: { value: "AuthSuite" } });
      
      expect(screen.getByText("should handle user login")).toBeInTheDocument();
      expect(screen.queryByText("should display dashboard")).not.toBeInTheDocument();
    });

    it("is case insensitive", () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      const searchInput = screen.getByPlaceholderText("Search tests...");
      fireEvent.change(searchInput, { target: { value: "LOGIN" } });
      
      expect(screen.getByText("should handle user login")).toBeInTheDocument();
    });

    it("clears filter when search is cleared", () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      const searchInput = screen.getByPlaceholderText("Search tests...");
      fireEvent.change(searchInput, { target: { value: "login" } });
      expect(screen.getByText(/1 of 5 tests/)).toBeInTheDocument();
      
      fireEvent.change(searchInput, { target: { value: "" } });
      expect(screen.getByText(/5 of 5 tests/)).toBeInTheDocument();
    });
  });

  // ===========================================================================
  // 6. Status Filter Dropdown
  // ===========================================================================
  describe("Status Filter Dropdown", () => {
    it("opens filter dropdown when clicking filter button", async () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      const filterButton = screen.getByRole("button", { name: /filter/i });
      await userEvent.click(filterButton);
      
      // Dropdown renders in a portal, wait for it to appear
      await waitFor(() => {
        expect(screen.getByRole("menuitemcheckbox", { name: /passed/i })).toBeInTheDocument();
      });
      
      expect(screen.getByRole("menuitemcheckbox", { name: /failed/i })).toBeInTheDocument();
      expect(screen.getByRole("menuitemcheckbox", { name: /skipped/i })).toBeInTheDocument();
      expect(screen.getByRole("menuitemcheckbox", { name: /error/i })).toBeInTheDocument();
    });

    it("filters by single status", async () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      const filterButton = screen.getByRole("button", { name: /filter/i });
      await userEvent.click(filterButton);
      
      await waitFor(() => {
        expect(screen.getByRole("menuitemcheckbox", { name: /failed/i })).toBeInTheDocument();
      });
      
      const failedOption = screen.getByRole("menuitemcheckbox", { name: /failed/i });
      await userEvent.click(failedOption);
      
      // Should only show failed test
      await waitFor(() => {
        expect(screen.getByText("should validate form inputs")).toBeInTheDocument();
      });
      expect(screen.queryByText("should handle user login")).not.toBeInTheDocument();
    });

    it("filters by multiple statuses", async () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      const filterButton = screen.getByRole("button", { name: /filter/i });
      
      // Select failed
      await userEvent.click(filterButton);
      await waitFor(() => {
        expect(screen.getByRole("menuitemcheckbox", { name: /failed/i })).toBeInTheDocument();
      });
      await userEvent.click(screen.getByRole("menuitemcheckbox", { name: /failed/i }));
      
      // Re-open dropdown and select error
      await userEvent.click(filterButton);
      await waitFor(() => {
        expect(screen.getByRole("menuitemcheckbox", { name: /error/i })).toBeInTheDocument();
      });
      await userEvent.click(screen.getByRole("menuitemcheckbox", { name: /error/i }));
      
      // Should show both failed and error tests
      await waitFor(() => {
        expect(screen.getByText("should validate form inputs")).toBeInTheDocument();
        expect(screen.getByText("should connect to database")).toBeInTheDocument();
      });
      expect(screen.queryByText("should handle user login")).not.toBeInTheDocument();
    });

    it("shows badge count when filters are active", async () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      const filterButton = screen.getByRole("button", { name: /filter/i });
      await userEvent.click(filterButton);
      
      await waitFor(() => {
        expect(screen.getByRole("menuitemcheckbox", { name: /failed/i })).toBeInTheDocument();
      });
      
      await userEvent.click(screen.getByRole("menuitemcheckbox", { name: /failed/i }));
      
      // Filter button should now show count badge
      await waitFor(() => {
        const updatedFilterButton = screen.getByRole("button", { name: /filter/i });
        expect(within(updatedFilterButton).getByText("1")).toBeInTheDocument();
      });
    });

    it("removes filter when status is unselected", async () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      const filterButton = screen.getByRole("button", { name: /filter/i });
      
      // Apply filter
      await userEvent.click(filterButton);
      await waitFor(() => {
        expect(screen.getByRole("menuitemcheckbox", { name: /failed/i })).toBeInTheDocument();
      });
      await userEvent.click(screen.getByRole("menuitemcheckbox", { name: /failed/i }));
      
      await waitFor(() => {
        expect(screen.getByText(/1 of 5 tests/)).toBeInTheDocument();
      });
      
      // Remove filter
      await userEvent.click(filterButton);
      await waitFor(() => {
        expect(screen.getByRole("menuitemcheckbox", { name: /failed/i })).toBeInTheDocument();
      });
      await userEvent.click(screen.getByRole("menuitemcheckbox", { name: /failed/i }));
      
      await waitFor(() => {
        expect(screen.getByText(/5 of 5 tests/)).toBeInTheDocument();
      });
    });
  });

  // ===========================================================================
  // 7. Sorting
  // ===========================================================================
  describe("Sorting", () => {
    it("sorts by test name ascending by default", () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      const rows = screen.getAllByRole("row");
      // First row is header, subsequent rows are data
      // With name sort ascending: connect, dashboard, edge case, login, validate
      const cells = rows.slice(1).map(row => within(row).queryByText(/should/));
      const testNames = cells.filter(Boolean).map(c => c?.textContent);
      
      // Verify alphabetical order
      expect(testNames[0]).toBe("should connect to database");
      expect(testNames[1]).toBe("should display dashboard");
    });

    it("sorts by name descending when header clicked twice", () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      const nameHeader = screen.getByText("Test Name");
      // First click on "name" column: since it's already sorted by name asc, this toggles to desc
      fireEvent.click(nameHeader);
      
      const rows = screen.getAllByRole("row");
      const cells = rows.slice(1).map(row => within(row).queryByText(/should/));
      const testNames = cells.filter(Boolean).map(c => c?.textContent);
      
      // Reverse alphabetical order - "validate" comes last alphabetically, so first in desc
      expect(testNames[0]).toBe("should validate form inputs");
    });

    it("sorts by status when status header clicked", () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      const statusHeader = screen.getByText("Status");
      fireEvent.click(statusHeader);
      
      // Status order: failed, error, passed, skipped (ascending)
      const rows = screen.getAllByRole("row");
      const firstDataRow = rows[1];
      
      // First row should be failed
      expect(within(firstDataRow).getByText("Failed")).toBeInTheDocument();
    });

    it("sorts by duration when duration header clicked", () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      const durationHeader = screen.getByText("Duration");
      fireEvent.click(durationHeader);
      
      // Ascending duration order: 0ms (skipped), 150ms, 320ms, 450ms, 5200ms
      const rows = screen.getAllByRole("row");
      const firstDataRow = rows[1];
      
      // Skipped test has 0ms duration
      expect(within(firstDataRow).getByText("should handle edge case")).toBeInTheDocument();
    });

    it("toggles sort direction on repeated clicks", () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      const durationHeader = screen.getByText("Duration");
      
      // First click: ascending
      fireEvent.click(durationHeader);
      let rows = screen.getAllByRole("row");
      expect(within(rows[1]).getByText("should handle edge case")).toBeInTheDocument();
      
      // Second click: descending
      fireEvent.click(durationHeader);
      rows = screen.getAllByRole("row");
      expect(within(rows[1]).getByText("should connect to database")).toBeInTheDocument();
    });
  });

  // ===========================================================================
  // 8. Row Expansion
  // ===========================================================================
  describe("Row Expansion", () => {
    it("shows expand button for rows with details", () => {
      const results = [
        createTestResult({
          id: "1",
          testName: "Test with error",
          status: "failed",
          errorMessage: "Test failed",
        }),
        createTestResult({
          id: "2",
          testName: "Test without details",
          status: "passed",
        }),
      ];
      render(<TestResultsTable results={results} />);
      
      // Find rows in table body
      const tbody = document.querySelector("tbody");
      const rows = tbody?.querySelectorAll("tr");
      
      // The failed test row should have an expand button
      const failedTestRow = Array.from(rows || []).find(row => 
        row.textContent?.includes("Test with error")
      );
      expect(failedTestRow?.querySelector("button")).toBeInTheDocument();
      
      // The passing test row should not have an expand button
      const passingTestRow = Array.from(rows || []).find(row => 
        row.textContent?.includes("Test without details")
      );
      const expandButton = passingTestRow?.querySelector("button");
      expect(expandButton).toBeNull();
    });

    it("expands row when clicked", () => {
      const results = [
        createTestResult({
          id: "1",
          testName: "Test with error",
          status: "failed",
          errorMessage: "Expected value to be truthy",
          stackTrace: "at test.ts:15",
        }),
      ];
      render(<TestResultsTable results={results} />);
      
      const row = screen.getByText("Test with error").closest("tr");
      expect(row).toBeInTheDocument();
      
      fireEvent.click(row!);
      
      // Error message should now be visible
      expect(screen.getByText("Expected value to be truthy")).toBeInTheDocument();
    });

    it("collapses row when clicked again", () => {
      const results = [
        createTestResult({
          id: "1",
          testName: "Test with error",
          status: "failed",
          errorMessage: "Expected value to be truthy",
        }),
      ];
      render(<TestResultsTable results={results} />);
      
      const row = screen.getByText("Test with error").closest("tr");
      
      // Expand
      fireEvent.click(row!);
      expect(screen.getByText("Expected value to be truthy")).toBeInTheDocument();
      
      // Collapse - click the main row again (not the expanded content row)
      fireEvent.click(row!);
      expect(screen.queryByText("Expected value to be truthy")).not.toBeInTheDocument();
    });

    it("toggles expansion via expand button", () => {
      const results = [
        createTestResult({
          id: "1",
          testName: "Test with error",
          status: "failed",
          errorMessage: "Error occurred",
        }),
      ];
      render(<TestResultsTable results={results} />);
      
      // Find the expand button in the row
      const row = screen.getByText("Test with error").closest("tr");
      const expandButton = row?.querySelector("button");
      expect(expandButton).toBeInTheDocument();
      
      fireEvent.click(expandButton!);
      
      expect(screen.getByText("Error occurred")).toBeInTheDocument();
    });

    it("does not expand rows without details", () => {
      const results = [
        createTestResult({
          id: "1",
          testName: "Simple passing test",
          status: "passed",
          // No error, stdout, stderr, or stackTrace
        }),
      ];
      render(<TestResultsTable results={results} />);
      
      const row = screen.getByText("Simple passing test").closest("tr");
      fireEvent.click(row!);
      
      // No expanded content should appear
      expect(screen.queryByText("Error Message")).not.toBeInTheDocument();
      expect(screen.queryByText("No additional details available")).not.toBeInTheDocument();
    });
  });

  // ===========================================================================
  // 9. onTestClick Callback
  // ===========================================================================
  describe("onTestClick Callback", () => {
    it("calls onTestClick when row is clicked", () => {
      const mockOnTestClick = vi.fn();
      const results = createMockResults();
      render(<TestResultsTable results={results} onTestClick={mockOnTestClick} />);
      
      const row = screen.getByText("should handle user login").closest("tr");
      fireEvent.click(row!);
      
      expect(mockOnTestClick).toHaveBeenCalledTimes(1);
      expect(mockOnTestClick).toHaveBeenCalledWith(
        expect.objectContaining({
          id: "1",
          testName: "should handle user login",
          status: "passed",
        })
      );
    });

    it("calls onTestClick with correct test data for each row", () => {
      const mockOnTestClick = vi.fn();
      const results = createMockResults();
      render(<TestResultsTable results={results} onTestClick={mockOnTestClick} />);
      
      const failedRow = screen.getByText("should validate form inputs").closest("tr");
      fireEvent.click(failedRow!);
      
      expect(mockOnTestClick).toHaveBeenCalledWith(
        expect.objectContaining({
          id: "3",
          testName: "should validate form inputs",
          status: "failed",
          errorMessage: "Expected true but got false",
        })
      );
    });

    it("calls onTestClick even when expanding row with details", () => {
      const mockOnTestClick = vi.fn();
      const results = [
        createTestResult({
          id: "1",
          testName: "Test with error",
          status: "failed",
          errorMessage: "Error",
        }),
      ];
      render(<TestResultsTable results={results} onTestClick={mockOnTestClick} />);
      
      const row = screen.getByText("Test with error").closest("tr");
      fireEvent.click(row!);
      
      expect(mockOnTestClick).toHaveBeenCalledTimes(1);
    });

    it("does not throw when onTestClick is not provided", () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      const row = screen.getByText("should handle user login").closest("tr");
      
      // Should not throw
      expect(() => fireEvent.click(row!)).not.toThrow();
    });
  });

  // ===========================================================================
  // 10. Expanded Row Content (Tabs)
  // ===========================================================================
  describe("Expanded Row Content", () => {
    it("shows error message when error tab is active", () => {
      const results = [
        createTestResult({
          id: "1",
          testName: "Test with error",
          status: "failed",
          errorMessage: "Assertion failed: expected 5 but got 3",
        }),
      ];
      render(<TestResultsTable results={results} />);
      
      const row = screen.getByText("Test with error").closest("tr");
      fireEvent.click(row!);
      
      expect(screen.getByText("Error Message")).toBeInTheDocument();
      expect(screen.getByText("Assertion failed: expected 5 but got 3")).toBeInTheDocument();
    });

    it("shows stack trace in error tab", () => {
      const results = [
        createTestResult({
          id: "1",
          testName: "Test with stack trace",
          status: "failed",
          errorMessage: "Error occurred",
          stackTrace: "at Function.execute (test.ts:42)\nat Runner.run (runner.ts:100)",
        }),
      ];
      render(<TestResultsTable results={results} />);
      
      const row = screen.getByText("Test with stack trace").closest("tr");
      fireEvent.click(row!);
      
      expect(screen.getByText("Stack Trace")).toBeInTheDocument();
      expect(screen.getByText(/at Function.execute/)).toBeInTheDocument();
    });

    it("shows stdout tab when stdout is available", () => {
      const results = [
        createTestResult({
          id: "1",
          testName: "Test with output",
          status: "passed",
          stdout: "Test output: Hello World",
        }),
      ];
      render(<TestResultsTable results={results} />);
      
      const row = screen.getByText("Test with output").closest("tr");
      fireEvent.click(row!);
      
      // Stdout tab should be visible
      const stdoutTab = screen.getByRole("button", { name: "Stdout" });
      expect(stdoutTab).toBeInTheDocument();
      
      // Since there's no error, stdout should be active by default
      expect(screen.getByText("Test output: Hello World")).toBeInTheDocument();
    });

    it("switches to stdout tab when clicked", () => {
      const results = [
        createTestResult({
          id: "1",
          testName: "Test with error and output",
          status: "failed",
          errorMessage: "Test failed",
          stdout: "Console log output here",
        }),
      ];
      render(<TestResultsTable results={results} />);
      
      const row = screen.getByText("Test with error and output").closest("tr");
      fireEvent.click(row!);
      
      // Initially error is shown (default when error exists)
      expect(screen.getByText("Error Message")).toBeInTheDocument();
      
      // Click stdout tab
      const stdoutTab = screen.getByRole("button", { name: "Stdout" });
      fireEvent.click(stdoutTab);
      
      expect(screen.getByText("Console log output here")).toBeInTheDocument();
    });

    it("shows stderr tab when stderr is available", () => {
      const results = [
        createTestResult({
          id: "1",
          testName: "Test with stderr",
          status: "error",
          errorMessage: "Process exited with code 1",
          stderr: "FATAL: Unable to connect to server",
        }),
      ];
      render(<TestResultsTable results={results} />);
      
      const row = screen.getByText("Test with stderr").closest("tr");
      fireEvent.click(row!);
      
      // Stderr tab should be visible
      const stderrTab = screen.getByRole("button", { name: "Stderr" });
      expect(stderrTab).toBeInTheDocument();
      
      // Click stderr tab
      fireEvent.click(stderrTab);
      
      expect(screen.getByText("FATAL: Unable to connect to server")).toBeInTheDocument();
    });

    it("only shows relevant tabs based on available data", () => {
      const results = [
        createTestResult({
          id: "1",
          testName: "Test with only error",
          status: "failed",
          errorMessage: "Only error, no stdout/stderr",
        }),
      ];
      render(<TestResultsTable results={results} />);
      
      const row = screen.getByText("Test with only error").closest("tr");
      fireEvent.click(row!);
      
      // Only Error tab should exist
      expect(screen.getByRole("button", { name: "Error" })).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Stdout" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Stderr" })).not.toBeInTheDocument();
    });

    it("shows all tabs when all data is available", () => {
      const results = [
        createTestResult({
          id: "1",
          testName: "Test with all output",
          status: "failed",
          errorMessage: "Error message",
          stackTrace: "Stack trace here",
          stdout: "Stdout content",
          stderr: "Stderr content",
        }),
      ];
      render(<TestResultsTable results={results} />);
      
      const row = screen.getByText("Test with all output").closest("tr");
      fireEvent.click(row!);
      
      // All tabs should be visible
      expect(screen.getByRole("button", { name: "Error" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Stdout" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Stderr" })).toBeInTheDocument();
    });
  });

  // ===========================================================================
  // Combined Scenarios
  // ===========================================================================
  describe("Combined Scenarios", () => {
    it("applies both search and status filter together", async () => {
      const results = [
        createTestResult({ id: "1", testName: "auth login test", status: "passed" }),
        createTestResult({ id: "2", testName: "auth logout test", status: "failed", errorMessage: "e" }),
        createTestResult({ id: "3", testName: "dashboard test", status: "failed", errorMessage: "e" }),
      ];
      render(<TestResultsTable results={results} />);
      
      // Apply search filter
      const searchInput = screen.getByPlaceholderText("Search tests...");
      fireEvent.change(searchInput, { target: { value: "auth" } });
      
      // Apply status filter
      const filterButton = screen.getByRole("button", { name: /filter/i });
      await userEvent.click(filterButton);
      
      await waitFor(() => {
        expect(screen.getByRole("menuitemcheckbox", { name: /failed/i })).toBeInTheDocument();
      });
      
      await userEvent.click(screen.getByRole("menuitemcheckbox", { name: /failed/i }));
      
      // Should only show "auth logout test" (matches both search and status)
      await waitFor(() => {
        expect(screen.getByText("auth logout test")).toBeInTheDocument();
      });
      expect(screen.queryByText("auth login test")).not.toBeInTheDocument();
      expect(screen.queryByText("dashboard test")).not.toBeInTheDocument();
    });

    it("maintains sort when filter is applied", async () => {
      const results = createMockResults();
      render(<TestResultsTable results={results} />);
      
      // Sort by duration descending
      const durationHeader = screen.getByText("Duration");
      fireEvent.click(durationHeader);
      fireEvent.click(durationHeader); // descending
      
      // Apply passed filter
      const filterButton = screen.getByRole("button", { name: /filter/i });
      await userEvent.click(filterButton);
      
      await waitFor(() => {
        expect(screen.getByRole("menuitemcheckbox", { name: /passed/i })).toBeInTheDocument();
      });
      
      await userEvent.click(screen.getByRole("menuitemcheckbox", { name: /passed/i }));
      
      // Should show passed tests sorted by duration descending
      await waitFor(() => {
        const rows = screen.getAllByRole("row");
        const firstDataRow = rows[1];
        // "should display dashboard" has 320ms, "should handle user login" has 150ms
        expect(within(firstDataRow).getByText("should display dashboard")).toBeInTheDocument();
      });
    });
  });
});
