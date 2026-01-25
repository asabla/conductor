import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { Button, buttonVariants } from "@/components/ui/button";

describe("Button", () => {
  describe("rendering", () => {
    it("renders with default props", () => {
      render(<Button>Click me</Button>);

      expect(
        screen.getByRole("button", { name: "Click me" })
      ).toBeInTheDocument();
    });

    it("renders children correctly", () => {
      render(
        <Button>
          <span data-testid="child-element">Child Content</span>
        </Button>
      );

      expect(screen.getByTestId("child-element")).toBeInTheDocument();
      expect(screen.getByText("Child Content")).toBeInTheDocument();
    });

    it("applies custom className", () => {
      render(<Button className="custom-class">Button</Button>);

      const button = screen.getByRole("button");
      expect(button).toHaveClass("custom-class");
    });

    it("spreads additional props to the element", () => {
      render(<Button data-testid="test-button">Button</Button>);

      expect(screen.getByTestId("test-button")).toBeInTheDocument();
    });

    it("renders as button element by default", () => {
      render(<Button>Button</Button>);

      const button = screen.getByRole("button");
      expect(button.tagName).toBe("BUTTON");
    });
  });

  describe("variants", () => {
    it("applies default variant classes", () => {
      render(<Button variant="default">Default</Button>);

      const button = screen.getByRole("button");
      expect(button).toHaveClass("bg-primary");
      expect(button).toHaveClass("text-primary-foreground");
    });

    it("applies destructive variant classes", () => {
      render(<Button variant="destructive">Destructive</Button>);

      const button = screen.getByRole("button");
      expect(button).toHaveClass("bg-destructive");
      expect(button).toHaveClass("text-destructive-foreground");
    });

    it("applies outline variant classes", () => {
      render(<Button variant="outline">Outline</Button>);

      const button = screen.getByRole("button");
      expect(button).toHaveClass("border");
      expect(button).toHaveClass("border-input");
      expect(button).toHaveClass("bg-background");
    });

    it("applies secondary variant classes", () => {
      render(<Button variant="secondary">Secondary</Button>);

      const button = screen.getByRole("button");
      expect(button).toHaveClass("bg-secondary");
      expect(button).toHaveClass("text-secondary-foreground");
    });

    it("applies ghost variant classes", () => {
      render(<Button variant="ghost">Ghost</Button>);

      const button = screen.getByRole("button");
      expect(button).toHaveClass("hover:bg-accent");
      expect(button).toHaveClass("hover:text-accent-foreground");
    });

    it("applies link variant classes", () => {
      render(<Button variant="link">Link</Button>);

      const button = screen.getByRole("button");
      expect(button).toHaveClass("text-primary");
      expect(button).toHaveClass("underline-offset-4");
      expect(button).toHaveClass("hover:underline");
    });
  });

  describe("sizes", () => {
    it("applies default size classes", () => {
      render(<Button size="default">Default Size</Button>);

      const button = screen.getByRole("button");
      expect(button).toHaveClass("h-10");
      expect(button).toHaveClass("px-4");
      expect(button).toHaveClass("py-2");
    });

    it("applies sm size classes", () => {
      render(<Button size="sm">Small</Button>);

      const button = screen.getByRole("button");
      expect(button).toHaveClass("h-9");
      expect(button).toHaveClass("px-3");
    });

    it("applies lg size classes", () => {
      render(<Button size="lg">Large</Button>);

      const button = screen.getByRole("button");
      expect(button).toHaveClass("h-11");
      expect(button).toHaveClass("px-8");
    });

    it("applies icon size classes", () => {
      render(<Button size="icon">Icon</Button>);

      const button = screen.getByRole("button");
      expect(button).toHaveClass("h-10");
      expect(button).toHaveClass("w-10");
    });
  });

  describe("asChild prop", () => {
    it("renders as child element when asChild is true", () => {
      render(
        <Button asChild>
          <a href="/test">Link Button</a>
        </Button>
      );

      const link = screen.getByRole("link", { name: "Link Button" });
      expect(link).toBeInTheDocument();
      expect(link).toHaveAttribute("href", "/test");
      expect(link).toHaveClass("bg-primary");
    });

    it("passes className to child element", () => {
      render(
        <Button asChild className="custom-class">
          <a href="/test">Link Button</a>
        </Button>
      );

      const link = screen.getByRole("link");
      expect(link).toHaveClass("custom-class");
    });
  });

  describe("click handlers", () => {
    it("calls onClick handler when clicked", () => {
      const handleClick = vi.fn();
      render(<Button onClick={handleClick}>Click me</Button>);

      fireEvent.click(screen.getByRole("button"));

      expect(handleClick).toHaveBeenCalledTimes(1);
    });

    it("does not call onClick when disabled", () => {
      const handleClick = vi.fn();
      render(
        <Button onClick={handleClick} disabled>
          Click me
        </Button>
      );

      fireEvent.click(screen.getByRole("button"));

      expect(handleClick).not.toHaveBeenCalled();
    });
  });

  describe("disabled state", () => {
    it("applies disabled attribute", () => {
      render(<Button disabled>Disabled</Button>);

      const button = screen.getByRole("button");
      expect(button).toBeDisabled();
    });

    it("applies disabled styles", () => {
      render(<Button disabled>Disabled</Button>);

      const button = screen.getByRole("button");
      expect(button).toHaveClass("disabled:pointer-events-none");
      expect(button).toHaveClass("disabled:opacity-50");
    });
  });

  describe("base styles", () => {
    it("applies base button styles", () => {
      render(<Button>Styled</Button>);

      const button = screen.getByRole("button");
      expect(button).toHaveClass("inline-flex");
      expect(button).toHaveClass("items-center");
      expect(button).toHaveClass("justify-center");
      expect(button).toHaveClass("whitespace-nowrap");
      expect(button).toHaveClass("rounded-md");
      expect(button).toHaveClass("text-sm");
      expect(button).toHaveClass("font-medium");
    });
  });

  describe("ref forwarding", () => {
    it("forwards ref to button element", () => {
      const ref = vi.fn();
      render(<Button ref={ref}>Button</Button>);

      expect(ref).toHaveBeenCalled();
      expect(ref.mock.calls[0]?.[0]).toBeInstanceOf(HTMLButtonElement);
    });
  });

  describe("buttonVariants", () => {
    it("returns correct class string for default variant and size", () => {
      const classes = buttonVariants({ variant: "default", size: "default" });

      expect(classes).toContain("bg-primary");
      expect(classes).toContain("h-10");
    });

    it("returns correct class string for destructive variant", () => {
      const classes = buttonVariants({ variant: "destructive" });

      expect(classes).toContain("bg-destructive");
      expect(classes).toContain("text-destructive-foreground");
    });

    it("returns correct class string for outline variant", () => {
      const classes = buttonVariants({ variant: "outline" });

      expect(classes).toContain("border");
      expect(classes).toContain("border-input");
      expect(classes).toContain("bg-background");
    });

    it("returns correct class string for sm size", () => {
      const classes = buttonVariants({ size: "sm" });

      expect(classes).toContain("h-9");
      expect(classes).toContain("px-3");
    });

    it("returns correct class string for lg size", () => {
      const classes = buttonVariants({ size: "lg" });

      expect(classes).toContain("h-11");
      expect(classes).toContain("px-8");
    });

    it("returns correct class string for icon size", () => {
      const classes = buttonVariants({ size: "icon" });

      expect(classes).toContain("h-10");
      expect(classes).toContain("w-10");
    });

    it("uses default variant and size when not specified", () => {
      const classes = buttonVariants({});

      expect(classes).toContain("bg-primary");
      expect(classes).toContain("h-10");
      expect(classes).toContain("px-4");
    });

    it("accepts custom className", () => {
      const classes = buttonVariants({ className: "custom-class" });

      expect(classes).toContain("custom-class");
    });
  });
});
