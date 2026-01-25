/**
 * WebSocket connection hook for real-time updates
 * Handles connection, reconnection, room subscriptions, and message parsing
 */

import { useEffect, useRef, useCallback, useState } from "react";

// =============================================================================
// Types
// =============================================================================

export type WebSocketStatus =
  | "connecting"
  | "connected"
  | "disconnected"
  | "reconnecting";

export interface WebSocketMessage<T = unknown> {
  type: string;
  room?: string;
  data: T;
  timestamp: string;
}

export interface LogMessage {
  sequence: number;
  timestamp: string;
  stream: "stdout" | "stderr";
  message: string;
  testId?: string;
}

export interface RunStatusUpdate {
  runId: string;
  status: string;
  totalTests: number;
  passedTests: number;
  failedTests: number;
  skippedTests: number;
}

export interface AgentStatusUpdate {
  agentId: string;
  status: string;
  currentRunId?: string;
}

export type MessageHandler<T = unknown> = (message: WebSocketMessage<T>) => void;

export interface UseWebSocketOptions {
  /** URL to connect to (defaults to /ws) */
  url?: string;
  /** Auto-connect on mount (default: true) */
  autoConnect?: boolean;
  /** Reconnection attempts (default: 5) */
  maxReconnectAttempts?: number;
  /** Base delay between reconnection attempts in ms (default: 1000) */
  reconnectDelay?: number;
  /** Maximum reconnection delay in ms (default: 30000) */
  maxReconnectDelay?: number;
  /** Callback when connection is established */
  onConnect?: () => void;
  /** Callback when connection is closed */
  onDisconnect?: (event: CloseEvent) => void;
  /** Callback when an error occurs */
  onError?: (event: Event) => void;
}

export interface UseWebSocketReturn {
  /** Current connection status */
  status: WebSocketStatus;
  /** Whether the socket is connected */
  isConnected: boolean;
  /** Manually connect to the WebSocket */
  connect: () => void;
  /** Manually disconnect from the WebSocket */
  disconnect: () => void;
  /** Subscribe to a room */
  subscribe: (room: string) => void;
  /** Unsubscribe from a room */
  unsubscribe: (room: string) => void;
  /** Send a message */
  send: <T>(type: string, data: T, room?: string) => void;
  /** Add a message handler */
  addMessageHandler: <T>(type: string, handler: MessageHandler<T>) => () => void;
  /** Remove a message handler */
  removeMessageHandler: (type: string, handler: MessageHandler) => void;
  /** Last error that occurred */
  error: Event | null;
  /** List of currently subscribed rooms */
  subscribedRooms: string[];
}

// =============================================================================
// Hook Implementation
// =============================================================================

export function useWebSocket(options: UseWebSocketOptions = {}): UseWebSocketReturn {
  const {
    url = getWebSocketUrl(),
    autoConnect = true,
    maxReconnectAttempts = 5,
    reconnectDelay = 1000,
    maxReconnectDelay = 30000,
    onConnect,
    onDisconnect,
    onError,
  } = options;

  const [status, setStatus] = useState<WebSocketStatus>("disconnected");
  const [error, setError] = useState<Event | null>(null);
  const [subscribedRooms, setSubscribedRooms] = useState<string[]>([]);

  const socketRef = useRef<WebSocket | null>(null);
  const reconnectAttemptsRef = useRef(0);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout>>();
  const messageHandlersRef = useRef<Map<string, Set<MessageHandler>>>(new Map());
  const pendingSubscriptionsRef = useRef<Set<string>>(new Set());

  // Get or create handler set for a message type
  const getHandlers = useCallback((type: string): Set<MessageHandler> => {
    if (!messageHandlersRef.current.has(type)) {
      messageHandlersRef.current.set(type, new Set());
    }
    return messageHandlersRef.current.get(type)!;
  }, []);

  // Send a message through the WebSocket
  const send = useCallback(<T>(type: string, data: T, room?: string) => {
    if (socketRef.current?.readyState === WebSocket.OPEN) {
      const message: WebSocketMessage<T> = {
        type,
        data,
        room,
        timestamp: new Date().toISOString(),
      };
      socketRef.current.send(JSON.stringify(message));
    }
  }, []);

  // Subscribe to a room
  const subscribe = useCallback(
    (room: string) => {
      if (socketRef.current?.readyState === WebSocket.OPEN) {
        send("subscribe", { room });
        setSubscribedRooms((prev) =>
          prev.includes(room) ? prev : [...prev, room]
        );
      } else {
        // Queue subscription for when connected
        pendingSubscriptionsRef.current.add(room);
      }
    },
    [send]
  );

  // Unsubscribe from a room
  const unsubscribe = useCallback(
    (room: string) => {
      pendingSubscriptionsRef.current.delete(room);
      if (socketRef.current?.readyState === WebSocket.OPEN) {
        send("unsubscribe", { room });
      }
      setSubscribedRooms((prev) => prev.filter((r) => r !== room));
    },
    [send]
  );

  // Add a message handler
  const addMessageHandler = useCallback(
    <T>(type: string, handler: MessageHandler<T>): (() => void) => {
      const handlers = getHandlers(type);
      handlers.add(handler as MessageHandler);
      return () => {
        handlers.delete(handler as MessageHandler);
      };
    },
    [getHandlers]
  );

  // Remove a message handler
  const removeMessageHandler = useCallback(
    (type: string, handler: MessageHandler) => {
      const handlers = messageHandlersRef.current.get(type);
      if (handlers) {
        handlers.delete(handler);
      }
    },
    []
  );

  // Handle incoming messages
  const handleMessage = useCallback((event: MessageEvent) => {
    try {
      const message = JSON.parse(event.data) as WebSocketMessage;
      const handlers = messageHandlersRef.current.get(message.type);
      if (handlers) {
        handlers.forEach((handler) => handler(message));
      }

      // Also dispatch to wildcard handlers
      const wildcardHandlers = messageHandlersRef.current.get("*");
      if (wildcardHandlers) {
        wildcardHandlers.forEach((handler) => handler(message));
      }
    } catch (err) {
      console.error("Failed to parse WebSocket message:", err);
    }
  }, []);

  // Connect to WebSocket
  const connect = useCallback(() => {
    if (socketRef.current?.readyState === WebSocket.OPEN) {
      return;
    }

    setStatus("connecting");
    setError(null);

    try {
      const socket = new WebSocket(url);
      socketRef.current = socket;

      socket.onopen = () => {
        setStatus("connected");
        reconnectAttemptsRef.current = 0;
        onConnect?.();

        // Process pending subscriptions
        pendingSubscriptionsRef.current.forEach((room) => {
          send("subscribe", { room });
          setSubscribedRooms((prev) =>
            prev.includes(room) ? prev : [...prev, room]
          );
        });
        pendingSubscriptionsRef.current.clear();
      };

      socket.onclose = (event) => {
        setStatus("disconnected");
        onDisconnect?.(event);

        // Attempt reconnection if not a clean close
        if (!event.wasClean && reconnectAttemptsRef.current < maxReconnectAttempts) {
          setStatus("reconnecting");
          const delay = Math.min(
            reconnectDelay * Math.pow(2, reconnectAttemptsRef.current),
            maxReconnectDelay
          );
          reconnectAttemptsRef.current++;

          reconnectTimeoutRef.current = setTimeout(() => {
            connect();
          }, delay);
        }
      };

      socket.onerror = (event) => {
        setError(event);
        onError?.(event);
      };

      socket.onmessage = handleMessage;
    } catch (err) {
      setStatus("disconnected");
      console.error("Failed to connect to WebSocket:", err);
    }
  }, [
    url,
    onConnect,
    onDisconnect,
    onError,
    handleMessage,
    maxReconnectAttempts,
    reconnectDelay,
    maxReconnectDelay,
    send,
  ]);

  // Disconnect from WebSocket
  const disconnect = useCallback(() => {
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
    }
    reconnectAttemptsRef.current = maxReconnectAttempts; // Prevent reconnection
    socketRef.current?.close(1000, "User initiated disconnect");
    socketRef.current = null;
    setStatus("disconnected");
    setSubscribedRooms([]);
  }, [maxReconnectAttempts]);

  // Auto-connect on mount
  useEffect(() => {
    if (autoConnect) {
      connect();
    }

    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      socketRef.current?.close(1000, "Component unmounted");
    };
  }, [autoConnect, connect]);

  return {
    status,
    isConnected: status === "connected",
    connect,
    disconnect,
    subscribe,
    unsubscribe,
    send,
    addMessageHandler,
    removeMessageHandler,
    error,
    subscribedRooms,
  };
}

// =============================================================================
// Specialized Hooks
// =============================================================================

/**
 * Hook for subscribing to run log updates
 */
export function useRunLogs(
  runId: string | undefined,
  options?: { enabled?: boolean }
) {
  const { enabled = true } = options ?? {};
  const [logs, setLogs] = useState<LogMessage[]>([]);
  const { subscribe, unsubscribe, addMessageHandler, isConnected } =
    useWebSocket({ autoConnect: enabled && !!runId });

  useEffect(() => {
    if (!runId || !enabled || !isConnected) return;

    const room = `run:${runId}:logs`;
    subscribe(room);

    const unsubscribeHandler = addMessageHandler<LogMessage>("log", (msg) => {
      if (msg.room === room) {
        setLogs((prev) => [...prev, msg.data]);
      }
    });

    return () => {
      unsubscribe(room);
      unsubscribeHandler();
    };
  }, [runId, enabled, isConnected, subscribe, unsubscribe, addMessageHandler]);

  const clearLogs = useCallback(() => {
    setLogs([]);
  }, []);

  return { logs, clearLogs, isConnected };
}

/**
 * Hook for subscribing to run status updates
 */
export function useRunStatus(
  runId: string | undefined,
  options?: { enabled?: boolean }
) {
  const { enabled = true } = options ?? {};
  const [runStatus, setRunStatus] = useState<RunStatusUpdate | null>(null);
  const { subscribe, unsubscribe, addMessageHandler, isConnected } =
    useWebSocket({ autoConnect: enabled && !!runId });

  useEffect(() => {
    if (!runId || !enabled || !isConnected) return;

    const room = `run:${runId}`;
    subscribe(room);

    const unsubscribeHandler = addMessageHandler<RunStatusUpdate>(
      "run_status",
      (msg) => {
        if (msg.data.runId === runId) {
          setRunStatus(msg.data);
        }
      }
    );

    return () => {
      unsubscribe(room);
      unsubscribeHandler();
    };
  }, [runId, enabled, isConnected, subscribe, unsubscribe, addMessageHandler]);

  return { runStatus, isConnected };
}

/**
 * Hook for subscribing to agent status updates
 */
export function useAgentStatus(options?: { enabled?: boolean }) {
  const { enabled = true } = options ?? {};
  const [agentStatuses, setAgentStatuses] = useState<
    Map<string, AgentStatusUpdate>
  >(new Map());
  const { subscribe, unsubscribe, addMessageHandler, isConnected } =
    useWebSocket({ autoConnect: enabled });

  useEffect(() => {
    if (!enabled || !isConnected) return;

    const room = "agents";
    subscribe(room);

    const unsubscribeHandler = addMessageHandler<AgentStatusUpdate>(
      "agent_status",
      (msg) => {
        setAgentStatuses((prev) => {
          const next = new Map(prev);
          next.set(msg.data.agentId, msg.data);
          return next;
        });
      }
    );

    return () => {
      unsubscribe(room);
      unsubscribeHandler();
    };
  }, [enabled, isConnected, subscribe, unsubscribe, addMessageHandler]);

  return { agentStatuses, isConnected };
}

// =============================================================================
// Utilities
// =============================================================================

/**
 * Get WebSocket URL based on current location
 */
function getWebSocketUrl(): string {
  const wsUrl = import.meta.env.VITE_WS_URL;
  if (wsUrl) return wsUrl;

  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  const host = window.location.host;
  return `${protocol}//${host}/ws`;
}
