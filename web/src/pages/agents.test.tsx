import { describe, it, expect } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { AgentsPage } from "./agents";

describe("AgentsPage", () => {
  it("renders page title and description", () => {
    render(<AgentsPage />);

    expect(
      screen.getByRole("heading", { level: 1, name: "Agents" })
    ).toBeInTheDocument();
    expect(
      screen.getByText("Manage and monitor test execution agents")
    ).toBeInTheDocument();
  });

  it("shows correct counts in summary cards", () => {
    render(<AgentsPage />);

    // Get all summary cards by their container structure
    const summaryCards = screen
      .getAllByRole("heading", { level: 3 })
      .map((heading) => heading.closest(".rounded-lg"));

    // Total Agents: 6
    expect(screen.getByText("Total Agents")).toBeInTheDocument();

    // Find the count values in summary cards (they have text-2xl font-bold class)
    // The counts appear in order: Total (6), Online (4), Busy (2), Offline (1)
    const countElements = screen.getAllByText(/^[0-9]+$/).filter((el) => {
      return el.className.includes("text-2xl");
    });

    expect(countElements).toHaveLength(4);
    expect(countElements[0]).toHaveTextContent("6"); // Total
    expect(countElements[1]).toHaveTextContent("4"); // Online (online + busy = 2 + 2)
    expect(countElements[2]).toHaveTextContent("2"); // Busy
    expect(countElements[3]).toHaveTextContent("1"); // Offline

    // Verify the card titles exist
    expect(screen.getByText("Total Agents")).toBeInTheDocument();
    // Use getAllByText for "Online", "Busy", "Offline" since they appear multiple times
    expect(screen.getAllByText("Online").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("Busy").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("Offline").length).toBeGreaterThanOrEqual(1);
  });

  it("renders all agent cards with correct names", () => {
    render(<AgentsPage />);

    expect(screen.getByText("agent-prod-1")).toBeInTheDocument();
    expect(screen.getByText("agent-prod-2")).toBeInTheDocument();
    expect(screen.getByText("agent-prod-3")).toBeInTheDocument();
    expect(screen.getByText("agent-staging-1")).toBeInTheDocument();
    expect(screen.getByText("agent-staging-2")).toBeInTheDocument();
    expect(screen.getByText("agent-dev-1")).toBeInTheDocument();
  });

  it("shows correct status indicators for each status type", () => {
    render(<AgentsPage />);

    // Check for status text indicators
    // Online status (agent-prod-1, agent-staging-1) - 2 agents
    const onlineStatuses = screen.getAllByText("Online");
    // One is the card title, others are status indicators
    expect(onlineStatuses.length).toBeGreaterThanOrEqual(2);

    // Busy status (agent-prod-2, agent-prod-3) - 2 agents
    const busyStatuses = screen.getAllByText("Busy");
    expect(busyStatuses.length).toBeGreaterThanOrEqual(2);

    // Draining status (agent-staging-2) - 1 agent
    expect(screen.getByText("Draining")).toBeInTheDocument();

    // Offline status (agent-dev-1) - 1 agent
    const offlineStatuses = screen.getAllByText("Offline");
    expect(offlineStatuses.length).toBeGreaterThanOrEqual(1);
  });

  it("filters agents by name", () => {
    render(<AgentsPage />);

    const searchInput = screen.getByPlaceholderText("Search agents...");
    fireEvent.change(searchInput, { target: { value: "prod" } });

    // Should show prod agents
    expect(screen.getByText("agent-prod-1")).toBeInTheDocument();
    expect(screen.getByText("agent-prod-2")).toBeInTheDocument();
    expect(screen.getByText("agent-prod-3")).toBeInTheDocument();

    // Should not show staging or dev agents
    expect(screen.queryByText("agent-staging-1")).not.toBeInTheDocument();
    expect(screen.queryByText("agent-staging-2")).not.toBeInTheDocument();
    expect(screen.queryByText("agent-dev-1")).not.toBeInTheDocument();
  });

  it("filters agents by hostname", () => {
    render(<AgentsPage />);

    const searchInput = screen.getByPlaceholderText("Search agents...");
    fireEvent.change(searchInput, { target: { value: "staging.internal" } });

    // Should show staging agents
    expect(screen.getByText("agent-staging-1")).toBeInTheDocument();
    expect(screen.getByText("agent-staging-2")).toBeInTheDocument();

    // Should not show prod or dev agents
    expect(screen.queryByText("agent-prod-1")).not.toBeInTheDocument();
    expect(screen.queryByText("agent-prod-2")).not.toBeInTheDocument();
    expect(screen.queryByText("agent-prod-3")).not.toBeInTheDocument();
    expect(screen.queryByText("agent-dev-1")).not.toBeInTheDocument();
  });

  it("filters agents by label", () => {
    render(<AgentsPage />);

    const searchInput = screen.getByPlaceholderText("Search agents...");
    fireEvent.change(searchInput, { target: { value: "development" } });

    // Should show only dev agent (has "development" label)
    expect(screen.getByText("agent-dev-1")).toBeInTheDocument();

    // Should not show prod or staging agents
    expect(screen.queryByText("agent-prod-1")).not.toBeInTheDocument();
    expect(screen.queryByText("agent-prod-2")).not.toBeInTheDocument();
    expect(screen.queryByText("agent-prod-3")).not.toBeInTheDocument();
    expect(screen.queryByText("agent-staging-1")).not.toBeInTheDocument();
    expect(screen.queryByText("agent-staging-2")).not.toBeInTheDocument();
  });

  it("shows 'No agents found' when search has no matches", () => {
    render(<AgentsPage />);

    const searchInput = screen.getByPlaceholderText("Search agents...");
    fireEvent.change(searchInput, { target: { value: "nonexistent-agent" } });

    expect(screen.getByText("No agents found")).toBeInTheDocument();
    expect(
      screen.getByText("No agents match your search criteria.")
    ).toBeInTheDocument();

    // Should not show any agent cards
    expect(screen.queryByText("agent-prod-1")).not.toBeInTheDocument();
    expect(screen.queryByText("agent-staging-1")).not.toBeInTheDocument();
    expect(screen.queryByText("agent-dev-1")).not.toBeInTheDocument();
  });

  it("shows current run info for busy agents", () => {
    render(<AgentsPage />);

    // agent-prod-2 is busy with "api-gateway/main"
    expect(screen.getByText("api-gateway/main")).toBeInTheDocument();

    // agent-prod-3 is busy with "user-service/feature/auth"
    expect(screen.getByText("user-service/feature/auth")).toBeInTheDocument();

    // Both should have "Running:" labels
    const runningLabels = screen.getAllByText("Running:");
    expect(runningLabels).toHaveLength(2);
  });

  it("shows Register Agent button", () => {
    render(<AgentsPage />);

    const registerButton = screen.getByRole("button", {
      name: /register agent/i,
    });
    expect(registerButton).toBeInTheDocument();
  });
});
