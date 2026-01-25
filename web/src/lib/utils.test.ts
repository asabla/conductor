import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  cn,
  formatRelativeTime,
  formatDuration,
  truncate,
  generateId,
} from "./utils";

describe("cn", () => {
  it("merges multiple class names", () => {
    expect(cn("foo", "bar")).toBe("foo bar");
  });

  it("handles single class name", () => {
    expect(cn("foo")).toBe("foo");
  });

  it("handles empty input", () => {
    expect(cn()).toBe("");
  });

  it("handles undefined and null values", () => {
    expect(cn("foo", undefined, "bar", null)).toBe("foo bar");
  });

  it("handles conditional classes with boolean", () => {
    expect(cn("base", true && "active")).toBe("base active");
    expect(cn("base", false && "inactive")).toBe("base");
  });

  it("handles object syntax for conditional classes", () => {
    expect(cn("base", { active: true, disabled: false })).toBe("base active");
  });

  it("handles array of classes", () => {
    expect(cn(["foo", "bar"], "baz")).toBe("foo bar baz");
  });

  it("resolves Tailwind CSS conflicts - keeps last conflicting class", () => {
    // twMerge should keep the last conflicting utility
    expect(cn("px-2", "px-4")).toBe("px-4");
    expect(cn("text-red-500", "text-blue-500")).toBe("text-blue-500");
    expect(cn("bg-white", "bg-black")).toBe("bg-black");
  });

  it("handles complex Tailwind conflicts", () => {
    expect(cn("p-4", "px-2")).toBe("p-4 px-2");
    expect(cn("text-sm", "text-lg")).toBe("text-lg");
    expect(cn("flex", "block")).toBe("block");
  });

  it("preserves non-conflicting classes", () => {
    expect(cn("flex", "items-center", "justify-between")).toBe(
      "flex items-center justify-between"
    );
  });

  it("handles whitespace in class names", () => {
    expect(cn("  foo  ", "bar")).toBe("foo bar");
  });
});

describe("formatRelativeTime", () => {
  beforeEach(() => {
    // Mock the current time to a fixed date for consistent tests
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2024-01-15T12:00:00Z"));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe("just now (less than 60 seconds)", () => {
    it('returns "just now" for current time', () => {
      const now = new Date();
      expect(formatRelativeTime(now)).toBe("just now");
    });

    it('returns "just now" for 30 seconds ago', () => {
      const date = new Date("2024-01-15T11:59:30Z");
      expect(formatRelativeTime(date)).toBe("just now");
    });

    it('returns "just now" for 59 seconds ago', () => {
      const date = new Date("2024-01-15T11:59:01Z");
      expect(formatRelativeTime(date)).toBe("just now");
    });
  });

  describe("minutes ago", () => {
    it("returns minutes for 1 minute ago", () => {
      const date = new Date("2024-01-15T11:59:00Z");
      expect(formatRelativeTime(date)).toBe("1m ago");
    });

    it("returns minutes for 30 minutes ago", () => {
      const date = new Date("2024-01-15T11:30:00Z");
      expect(formatRelativeTime(date)).toBe("30m ago");
    });

    it("returns minutes for 59 minutes ago", () => {
      const date = new Date("2024-01-15T11:01:00Z");
      expect(formatRelativeTime(date)).toBe("59m ago");
    });
  });

  describe("hours ago", () => {
    it("returns hours for 1 hour ago", () => {
      const date = new Date("2024-01-15T11:00:00Z");
      expect(formatRelativeTime(date)).toBe("1h ago");
    });

    it("returns hours for 12 hours ago", () => {
      const date = new Date("2024-01-15T00:00:00Z");
      expect(formatRelativeTime(date)).toBe("12h ago");
    });

    it("returns hours for 23 hours ago", () => {
      const date = new Date("2024-01-14T13:00:00Z");
      expect(formatRelativeTime(date)).toBe("23h ago");
    });
  });

  describe("days ago", () => {
    it("returns days for 1 day ago", () => {
      const date = new Date("2024-01-14T12:00:00Z");
      expect(formatRelativeTime(date)).toBe("1d ago");
    });

    it("returns days for 3 days ago", () => {
      const date = new Date("2024-01-12T12:00:00Z");
      expect(formatRelativeTime(date)).toBe("3d ago");
    });

    it("returns days for 6 days ago", () => {
      const date = new Date("2024-01-09T12:00:00Z");
      expect(formatRelativeTime(date)).toBe("6d ago");
    });
  });

  describe("weeks or more (localized date)", () => {
    it("returns localized date for 7 days ago", () => {
      const date = new Date("2024-01-08T12:00:00Z");
      const result = formatRelativeTime(date);
      // Should return a localized date string, not "7d ago"
      expect(result).not.toContain("ago");
    });

    it("returns localized date for 30 days ago", () => {
      const date = new Date("2023-12-16T12:00:00Z");
      const result = formatRelativeTime(date);
      expect(result).not.toContain("ago");
    });
  });

  describe("input types", () => {
    it("accepts Date object", () => {
      const date = new Date("2024-01-15T11:55:00Z");
      expect(formatRelativeTime(date)).toBe("5m ago");
    });

    it("accepts ISO string", () => {
      expect(formatRelativeTime("2024-01-15T11:55:00Z")).toBe("5m ago");
    });

    it("accepts date string", () => {
      expect(formatRelativeTime("2024-01-15T11:30:00Z")).toBe("30m ago");
    });
  });
});

describe("formatDuration", () => {
  describe("milliseconds", () => {
    it("formats 0 milliseconds", () => {
      expect(formatDuration(0)).toBe("0ms");
    });

    it("formats 1 millisecond", () => {
      expect(formatDuration(1)).toBe("1ms");
    });

    it("formats 500 milliseconds", () => {
      expect(formatDuration(500)).toBe("500ms");
    });

    it("formats 999 milliseconds", () => {
      expect(formatDuration(999)).toBe("999ms");
    });
  });

  describe("seconds", () => {
    it("formats exactly 1 second", () => {
      expect(formatDuration(1000)).toBe("1s");
    });

    it("formats 30 seconds", () => {
      expect(formatDuration(30000)).toBe("30s");
    });

    it("formats 59 seconds", () => {
      expect(formatDuration(59000)).toBe("59s");
    });

    it("truncates milliseconds when showing seconds", () => {
      expect(formatDuration(1500)).toBe("1s");
      expect(formatDuration(59999)).toBe("59s");
    });
  });

  describe("minutes", () => {
    it("formats exactly 1 minute", () => {
      expect(formatDuration(60000)).toBe("1m");
    });

    it("formats 1 minute and 30 seconds", () => {
      expect(formatDuration(90000)).toBe("1m 30s");
    });

    it("formats 30 minutes", () => {
      expect(formatDuration(1800000)).toBe("30m");
    });

    it("formats 59 minutes and 59 seconds", () => {
      expect(formatDuration(3599000)).toBe("59m 59s");
    });

    it("omits seconds when exactly 0", () => {
      expect(formatDuration(120000)).toBe("2m");
      expect(formatDuration(300000)).toBe("5m");
    });
  });

  describe("hours", () => {
    it("formats exactly 1 hour", () => {
      expect(formatDuration(3600000)).toBe("1h");
    });

    it("formats 1 hour and 30 minutes", () => {
      expect(formatDuration(5400000)).toBe("1h 30m");
    });

    it("formats 2 hours", () => {
      expect(formatDuration(7200000)).toBe("2h");
    });

    it("formats 24 hours", () => {
      expect(formatDuration(86400000)).toBe("24h");
    });

    it("omits minutes when exactly 0", () => {
      expect(formatDuration(7200000)).toBe("2h");
      expect(formatDuration(10800000)).toBe("3h");
    });

    it("includes minutes when non-zero", () => {
      expect(formatDuration(3660000)).toBe("1h 1m");
      expect(formatDuration(5430000)).toBe("1h 30m");
    });
  });

  describe("edge cases", () => {
    it("handles large durations", () => {
      // 100 hours
      expect(formatDuration(360000000)).toBe("100h");
    });

    it("rounds down to nearest unit", () => {
      // 1.9 seconds should show as 1s
      expect(formatDuration(1900)).toBe("1s");
    });
  });
});

describe("truncate", () => {
  describe("string shorter than limit", () => {
    it("returns original string when shorter than limit", () => {
      expect(truncate("hello", 10)).toBe("hello");
    });

    it("returns original string when much shorter than limit", () => {
      expect(truncate("hi", 100)).toBe("hi");
    });
  });

  describe("string at exact limit", () => {
    it("returns original string when exactly at limit", () => {
      expect(truncate("hello", 5)).toBe("hello");
    });

    it("returns original string when length equals limit", () => {
      expect(truncate("abc", 3)).toBe("abc");
    });
  });

  describe("string longer than limit", () => {
    it("truncates and adds ellipsis", () => {
      expect(truncate("hello world", 5)).toBe("hello...");
    });

    it("truncates at specified length", () => {
      expect(truncate("This is a long string", 7)).toBe("This is...");
    });

    it("truncates to single character with ellipsis", () => {
      expect(truncate("hello", 1)).toBe("h...");
    });

    it("truncates to zero characters with ellipsis", () => {
      expect(truncate("hello", 0)).toBe("...");
    });
  });

  describe("empty string", () => {
    it("returns empty string when input is empty", () => {
      expect(truncate("", 10)).toBe("");
    });

    it("returns empty string when input is empty and limit is 0", () => {
      expect(truncate("", 0)).toBe("");
    });
  });

  describe("special characters", () => {
    it("handles strings with spaces", () => {
      expect(truncate("hello world", 8)).toBe("hello wo...");
    });

    it("handles strings with special characters", () => {
      expect(truncate("hello! @#$", 6)).toBe("hello!...");
    });

    it("handles unicode characters", () => {
      expect(truncate("héllo wörld", 5)).toBe("héllo...");
    });
  });
});

describe("generateId", () => {
  it("returns a string", () => {
    const id = generateId();
    expect(typeof id).toBe("string");
  });

  it("returns a string of correct length (9 characters)", () => {
    // substring(2, 11) produces 9 characters
    const id = generateId();
    expect(id.length).toBe(9);
  });

  it("contains only alphanumeric characters", () => {
    const id = generateId();
    // Base 36 includes 0-9 and a-z
    expect(id).toMatch(/^[0-9a-z]+$/);
  });

  it("generates unique IDs on multiple calls", () => {
    const ids = new Set<string>();
    const iterations = 100;

    for (let i = 0; i < iterations; i++) {
      ids.add(generateId());
    }

    // All IDs should be unique
    expect(ids.size).toBe(iterations);
  });

  it("generates different IDs each time", () => {
    const id1 = generateId();
    const id2 = generateId();
    const id3 = generateId();

    expect(id1).not.toBe(id2);
    expect(id2).not.toBe(id3);
    expect(id1).not.toBe(id3);
  });

  it("does not start with a number necessarily", () => {
    // Just verify it runs without error and produces valid output
    // The first character could be 0-9 or a-z
    const id = generateId();
    expect(id).toBeTruthy();
  });
});
