/**
 * Real-time log viewer component
 * Features: auto-scroll, ANSI color support, search, download
 */

import {
  useState,
  useEffect,
  useRef,
  useMemo,
  useCallback,
  memo,
} from "react";
import {
  Download,
  Search,
  ArrowDown,
  Pause,
  X,
  ChevronDown,
  ChevronUp,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { useRunLogs } from "@/hooks/use-websocket";

// =============================================================================
// Types
// =============================================================================

export interface LogLine {
  sequence: number;
  timestamp: string;
  stream: "stdout" | "stderr";
  message: string;
  testId?: string;
}

export interface LogViewerProps {
  /** Run ID to stream logs from */
  runId?: string;
  /** Static logs to display (if not using WebSocket) */
  logs?: LogLine[];
  /** Height of the log viewer */
  height?: number | string;
  /** Whether the run is still in progress */
  isLive?: boolean;
  /** Show line numbers */
  showLineNumbers?: boolean;
  /** Show timestamps */
  showTimestamps?: boolean;
  /** Class name for container */
  className?: string;
  /** Callback when download is clicked */
  onDownload?: () => void;
}

// =============================================================================
// ANSI Color Parser
// =============================================================================

interface AnsiSegment {
  text: string;
  className?: string;
}

const ANSI_COLORS: Record<string, string> = {
  "30": "text-gray-900 dark:text-gray-100",
  "31": "text-red-500",
  "32": "text-green-500",
  "33": "text-yellow-500",
  "34": "text-blue-500",
  "35": "text-purple-500",
  "36": "text-cyan-500",
  "37": "text-gray-300",
  "90": "text-gray-500",
  "91": "text-red-400",
  "92": "text-green-400",
  "93": "text-yellow-400",
  "94": "text-blue-400",
  "95": "text-purple-400",
  "96": "text-cyan-400",
  "97": "text-white",
  "1": "font-bold",
  "2": "opacity-60",
  "3": "italic",
  "4": "underline",
};

function parseAnsi(text: string): AnsiSegment[] {
  const segments: AnsiSegment[] = [];
  const escapeChar = String.fromCharCode(27);
  const regex = new RegExp(`${escapeChar}\\[([0-9;]+)m`, "g");
  let lastIndex = 0;
  let currentClasses: string[] = [];
  let match;

  while ((match = regex.exec(text)) !== null) {
    // Add text before this escape sequence
    if (match.index > lastIndex) {
      segments.push({
        text: text.slice(lastIndex, match.index),
        className: currentClasses.join(" ") || undefined,
      });
    }

    // Parse the escape codes
    const codes = match[1]?.split(";") || [];
    for (const code of codes) {
      if (code === "0") {
        currentClasses = [];
      } else if (ANSI_COLORS[code]) {
        currentClasses.push(ANSI_COLORS[code]);
      }
    }

    lastIndex = match.index + match[0].length;
  }

  // Add remaining text
  if (lastIndex < text.length) {
    segments.push({
      text: text.slice(lastIndex),
      className: currentClasses.join(" ") || undefined,
    });
  }

  return segments.length > 0 ? segments : [{ text }];
}

// =============================================================================
// Log Line Component
// =============================================================================

interface LogLineProps {
  line: LogLine;
  lineNumber: number;
  showLineNumbers: boolean;
  showTimestamps: boolean;
  searchQuery: string;
  isHighlighted: boolean;
}

const LogLineComponent = memo(function LogLineComponent({
  line,
  lineNumber,
  showLineNumbers,
  showTimestamps,
  searchQuery,
  isHighlighted,
}: LogLineProps) {
  const segments = useMemo(() => parseAnsi(line.message), [line.message]);

  const formatTimestamp = (ts: string) => {
    try {
      const date = new Date(ts);
      return date.toLocaleTimeString("en-US", {
        hour12: false,
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        fractionalSecondDigits: 3,
      });
    } catch {
      return ts;
    }
  };

  const highlightSearch = (text: string): React.ReactNode => {
    if (!searchQuery) return text;

    const parts = text.split(new RegExp(`(${escapeRegex(searchQuery)})`, "gi"));
    return parts.map((part, i) =>
      part.toLowerCase() === searchQuery.toLowerCase() ? (
        <mark key={i} className="bg-yellow-300 dark:bg-yellow-700 text-inherit">
          {part}
        </mark>
      ) : (
        part
      )
    );
  };

  return (
    <div
      className={cn(
        "flex font-mono text-xs leading-5 hover:bg-muted/50",
        line.stream === "stderr" && "bg-destructive/5",
        isHighlighted && "bg-yellow-100 dark:bg-yellow-900/30"
      )}
    >
      {showLineNumbers && (
        <span className="w-12 flex-shrink-0 select-none px-2 text-right text-muted-foreground">
          {lineNumber}
        </span>
      )}
      {showTimestamps && (
        <span className="w-24 flex-shrink-0 select-none px-2 text-muted-foreground">
          {formatTimestamp(line.timestamp)}
        </span>
      )}
      <span className="flex-1 whitespace-pre-wrap break-all px-2">
        {segments.map((segment, i) => (
          <span key={i} className={segment.className}>
            {highlightSearch(segment.text)}
          </span>
        ))}
      </span>
    </div>
  );
});

// =============================================================================
// Main Component
// =============================================================================

export function LogViewer({
  runId,
  logs: staticLogs,
  height = 400,
  isLive = false,
  showLineNumbers = true,
  showTimestamps = true,
  className,
  onDownload,
}: LogViewerProps) {
  const [searchQuery, setSearchQuery] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const [currentMatch, setCurrentMatch] = useState(0);
  const scrollRef = useRef<HTMLDivElement>(null);
  const endRef = useRef<HTMLDivElement>(null);

  // Use WebSocket for live logs
  const {
    logs: wsLogs,
    isConnected,
  } = useRunLogs(isLive ? runId : undefined, { enabled: isLive });

  // Combine static and WebSocket logs
  const logs = useMemo(() => {
    if (staticLogs && staticLogs.length > 0) {
      return staticLogs;
    }
    return wsLogs;
  }, [staticLogs, wsLogs]);

  // Filter logs by search
  const { filteredLogs, matchIndices } = useMemo(() => {
    if (!searchQuery) {
      return { filteredLogs: logs, matchIndices: [] };
    }

    const indices: number[] = [];
    logs.forEach((log, index) => {
      if (log.message.toLowerCase().includes(searchQuery.toLowerCase())) {
        indices.push(index);
      }
    });

    return { filteredLogs: logs, matchIndices: indices };
  }, [logs, searchQuery]);

  // Auto-scroll to bottom when new logs arrive
  useEffect(() => {
    if (autoScroll && endRef.current) {
      endRef.current.scrollIntoView({ behavior: "smooth" });
    }
  }, [logs.length, autoScroll]);

  // Handle scroll to detect if user scrolled up
  const handleScroll = useCallback(() => {
    if (!scrollRef.current) return;
    
    const { scrollTop, scrollHeight, clientHeight } = scrollRef.current;
    const isAtBottom = scrollHeight - scrollTop - clientHeight < 50;
    
    if (!isAtBottom && autoScroll) {
      setAutoScroll(false);
    }
  }, [autoScroll]);

  // Navigate to match
  const goToMatch = useCallback(
    (direction: "next" | "prev") => {
      if (matchIndices.length === 0) return;

      let newIndex = currentMatch;
      if (direction === "next") {
        newIndex = (currentMatch + 1) % matchIndices.length;
      } else {
        newIndex = (currentMatch - 1 + matchIndices.length) % matchIndices.length;
      }
      setCurrentMatch(newIndex);

      // Scroll to the match
      const lineElement = document.querySelector(
        `[data-line-index="${matchIndices[newIndex]}"]`
      );
      lineElement?.scrollIntoView({ behavior: "smooth", block: "center" });
    },
    [currentMatch, matchIndices]
  );

  // Download logs
  const handleDownload = useCallback(() => {
    if (onDownload) {
      onDownload();
      return;
    }

    const content = logs.map((log) => `[${log.timestamp}] ${log.message}`).join("\n");
    const blob = new Blob([content], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `logs-${runId || "export"}.txt`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  }, [logs, runId, onDownload]);

  // Keyboard shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "f" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        setSearchOpen(true);
      }
      if (e.key === "Escape" && searchOpen) {
        setSearchOpen(false);
        setSearchQuery("");
      }
      if (e.key === "Enter" && searchOpen && matchIndices.length > 0) {
        goToMatch(e.shiftKey ? "prev" : "next");
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [searchOpen, matchIndices.length, goToMatch]);

  return (
    <div className={cn("flex flex-col rounded-lg border bg-background", className)}>
      {/* Toolbar */}
      <div className="flex items-center justify-between border-b px-3 py-2">
        <div className="flex items-center gap-2">
          {isLive && (
            <Badge
              variant={isConnected ? "success" : "secondary"}
              className="text-xs"
            >
              {isConnected ? "Live" : "Disconnected"}
            </Badge>
          )}
          <span className="text-xs text-muted-foreground">
            {logs.length} lines
          </span>
        </div>

        <div className="flex items-center gap-1">
          {searchOpen ? (
            <div className="flex items-center gap-1">
              <div className="relative">
                <Search className="absolute left-2 top-1/2 h-3 w-3 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={searchQuery}
                  onChange={(e) => {
                    setSearchQuery(e.target.value);
                    setCurrentMatch(0);
                  }}
                  placeholder="Search logs..."
                  className="h-7 w-48 pl-7 text-xs"
                  autoFocus
                />
              </div>
              {matchIndices.length > 0 && (
                <>
                  <span className="text-xs text-muted-foreground">
                    {currentMatch + 1}/{matchIndices.length}
                  </span>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7"
                    onClick={() => goToMatch("prev")}
                  >
                    <ChevronUp className="h-3 w-3" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7"
                    onClick={() => goToMatch("next")}
                  >
                    <ChevronDown className="h-3 w-3" />
                  </Button>
                </>
              )}
              <Button
                variant="ghost"
                size="icon"
                className="h-7 w-7"
                onClick={() => {
                  setSearchOpen(false);
                  setSearchQuery("");
                }}
              >
                <X className="h-3 w-3" />
              </Button>
            </div>
          ) : (
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7"
              onClick={() => setSearchOpen(true)}
              title="Search (Cmd+F)"
            >
              <Search className="h-3 w-3" />
            </Button>
          )}

          {isLive && (
            <Button
              variant={autoScroll ? "default" : "ghost"}
              size="icon"
              className="h-7 w-7"
              onClick={() => setAutoScroll(!autoScroll)}
              title={autoScroll ? "Auto-scroll on" : "Auto-scroll off"}
            >
              {autoScroll ? (
                <ArrowDown className="h-3 w-3" />
              ) : (
                <Pause className="h-3 w-3" />
              )}
            </Button>
          )}

          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7"
            onClick={handleDownload}
            title="Download logs"
          >
            <Download className="h-3 w-3" />
          </Button>
        </div>
      </div>

      {/* Log content */}
      <div
        ref={scrollRef}
        className="overflow-auto bg-muted/30"
        style={{ height }}
        onScroll={handleScroll}
      >
        {filteredLogs.length === 0 ? (
          <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
            {isLive ? "Waiting for logs..." : "No logs available"}
          </div>
        ) : (
          <div className="py-2">
            {filteredLogs.map((log, index) => (
              <div key={log.sequence} data-line-index={index}>
                <LogLineComponent
                  line={log}
                  lineNumber={index + 1}
                  showLineNumbers={showLineNumbers}
                  showTimestamps={showTimestamps}
                  searchQuery={searchQuery}
                  isHighlighted={matchIndices[currentMatch] === index}
                />
              </div>
            ))}
            <div ref={endRef} />
          </div>
        )}
      </div>
    </div>
  );
}

// =============================================================================
// Utilities
// =============================================================================

function escapeRegex(string: string): string {
  return string.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

export default LogViewer;
