import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { Badge, badgeVariants } from "@/components/ui/badge";

describe("Badge", () => {
  describe("rendering", () => {
    it("renders with default props", () => {
      render(<Badge>Test Badge</Badge>);

      expect(screen.getByText("Test Badge")).toBeInTheDocument();
    });

    it("renders children correctly", () => {
      render(
        <Badge>
          <span data-testid="child-element">Child Content</span>
        </Badge>
      );

      expect(screen.getByTestId("child-element")).toBeInTheDocument();
      expect(screen.getByText("Child Content")).toBeInTheDocument();
    });

    it("applies custom className", () => {
      render(<Badge className="custom-class">Badge</Badge>);

      const badge = screen.getByText("Badge");
      expect(badge).toHaveClass("custom-class");
    });

    it("spreads additional props to the element", () => {
      render(<Badge data-testid="test-badge">Badge</Badge>);

      expect(screen.getByTestId("test-badge")).toBeInTheDocument();
    });
  });

  describe("variants", () => {
    it("applies default variant classes", () => {
      render(<Badge>Default</Badge>);

      const badge = screen.getByText("Default");
      expect(badge).toHaveClass("bg-primary");
      expect(badge).toHaveClass("text-primary-foreground");
      expect(badge).toHaveClass("border-transparent");
    });

    it("applies secondary variant classes", () => {
      render(<Badge variant="secondary">Secondary</Badge>);

      const badge = screen.getByText("Secondary");
      expect(badge).toHaveClass("bg-secondary");
      expect(badge).toHaveClass("text-secondary-foreground");
      expect(badge).toHaveClass("border-transparent");
    });

    it("applies destructive variant classes", () => {
      render(<Badge variant="destructive">Destructive</Badge>);

      const badge = screen.getByText("Destructive");
      expect(badge).toHaveClass("bg-destructive");
      expect(badge).toHaveClass("text-destructive-foreground");
      expect(badge).toHaveClass("border-transparent");
    });

    it("applies outline variant classes", () => {
      render(<Badge variant="outline">Outline</Badge>);

      const badge = screen.getByText("Outline");
      expect(badge).toHaveClass("text-foreground");
      expect(badge).not.toHaveClass("bg-primary");
    });

    it("applies success variant classes", () => {
      render(<Badge variant="success">Success</Badge>);

      const badge = screen.getByText("Success");
      expect(badge).toHaveClass("bg-success");
      expect(badge).toHaveClass("text-success-foreground");
      expect(badge).toHaveClass("border-transparent");
    });

    it("applies warning variant classes", () => {
      render(<Badge variant="warning">Warning</Badge>);

      const badge = screen.getByText("Warning");
      expect(badge).toHaveClass("bg-warning");
      expect(badge).toHaveClass("text-warning-foreground");
      expect(badge).toHaveClass("border-transparent");
    });
  });

  describe("base styles", () => {
    it("applies base badge styles", () => {
      render(<Badge>Styled</Badge>);

      const badge = screen.getByText("Styled");
      expect(badge).toHaveClass("inline-flex");
      expect(badge).toHaveClass("items-center");
      expect(badge).toHaveClass("rounded-full");
      expect(badge).toHaveClass("border");
      expect(badge).toHaveClass("px-2.5");
      expect(badge).toHaveClass("py-0.5");
      expect(badge).toHaveClass("text-xs");
      expect(badge).toHaveClass("font-semibold");
    });
  });

  describe("badgeVariants", () => {
    it("returns correct class string for default variant", () => {
      const classes = badgeVariants({ variant: "default" });

      expect(classes).toContain("bg-primary");
      expect(classes).toContain("text-primary-foreground");
    });

    it("returns correct class string for secondary variant", () => {
      const classes = badgeVariants({ variant: "secondary" });

      expect(classes).toContain("bg-secondary");
      expect(classes).toContain("text-secondary-foreground");
    });

    it("returns correct class string for destructive variant", () => {
      const classes = badgeVariants({ variant: "destructive" });

      expect(classes).toContain("bg-destructive");
      expect(classes).toContain("text-destructive-foreground");
    });

    it("returns correct class string for outline variant", () => {
      const classes = badgeVariants({ variant: "outline" });

      expect(classes).toContain("text-foreground");
    });

    it("returns correct class string for success variant", () => {
      const classes = badgeVariants({ variant: "success" });

      expect(classes).toContain("bg-success");
      expect(classes).toContain("text-success-foreground");
    });

    it("returns correct class string for warning variant", () => {
      const classes = badgeVariants({ variant: "warning" });

      expect(classes).toContain("bg-warning");
      expect(classes).toContain("text-warning-foreground");
    });

    it("uses default variant when no variant is specified", () => {
      const classes = badgeVariants({});

      expect(classes).toContain("bg-primary");
      expect(classes).toContain("text-primary-foreground");
    });
  });
});
