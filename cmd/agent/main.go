// Package main is the entrypoint for the Conductor agent.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/conductor/conductor/internal/agent"
	"github.com/conductor/conductor/pkg/metrics"
	"github.com/conductor/conductor/pkg/tracing"
	"github.com/rs/zerolog"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load configuration
	cfg, err := agent.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Setup logger for startup
	logger := setupLogger(cfg)
	logger.Info().
		Str("agent_id", cfg.AgentID).
		Str("control_plane", cfg.ControlPlaneURL).
		Int("max_parallel", cfg.MaxParallel).
		Msg("Starting Conductor Agent")

	// Initialize metrics
	agentMetrics := metrics.NewAgentMetrics()
	logger.Info().Msg("metrics initialized")

	// Initialize tracing if configured
	var tracer *tracing.Tracer
	tracingEndpoint := os.Getenv("CONDUCTOR_TRACING_ENDPOINT")
	tracingEnabled := os.Getenv("CONDUCTOR_TRACING_ENABLED") == "true"
	if tracingEnabled && tracingEndpoint != "" {
		tracingCfg := tracing.Config{
			ServiceName:    "conductor-agent",
			ServiceVersion: agent.Version,
			Endpoint:       tracingEndpoint,
			Insecure:       os.Getenv("CONDUCTOR_TRACING_INSECURE") != "false",
			SampleRate:     1.0, // Default to 100% sampling
			Environment:    os.Getenv("CONDUCTOR_ENVIRONMENT"),
			Enabled:        true,
		}
		tracer, err = tracing.InitTracer(tracingCfg)
		if err != nil {
			logger.Warn().Err(err).Msg("failed to initialize tracing - continuing without tracing")
		} else {
			logger.Info().
				Str("endpoint", tracingEndpoint).
				Msg("tracing initialized")
		}
	} else {
		logger.Info().Msg("tracing disabled")
	}

	// Start metrics server in background
	metricsPort := os.Getenv("CONDUCTOR_AGENT_METRICS_PORT")
	if metricsPort == "" {
		metricsPort = "9092"
	}
	metricsServer := &http.Server{
		Addr:         ":" + metricsPort,
		Handler:      agentMetrics.Handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info().Str("address", metricsServer.Addr).Msg("starting agent metrics server")
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error().Err(err).Msg("metrics server error")
		}
	}()

	// Start resource metrics reporter
	if agentMetrics.Agent != nil {
		go reportResourceMetrics(cfg, agentMetrics.Agent, logger)
	}

	// Create agent
	agnt, err := agent.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start agent in goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := agnt.Start(ctx); err != nil {
			errChan <- err
		}
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		logger.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
	case err := <-errChan:
		logger.Error().Err(err).Msg("Agent error")
		return err
	}

	// Graceful shutdown
	logger.Info().Msg("Initiating graceful shutdown")
	cancel()

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Shutdown tracer first
	if tracer != nil {
		if err := tracer.Shutdown(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("tracer shutdown error")
		} else {
			logger.Info().Msg("tracer shutdown complete")
		}
	}

	// Shutdown metrics server
	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("metrics server shutdown error")
	}

	if err := agnt.Stop(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("Error during shutdown")
		return err
	}

	logger.Info().Msg("Agent shutdown complete")
	return nil
}

// setupLogger creates a logger based on configuration.
func setupLogger(cfg *agent.Config) zerolog.Logger {
	var logger zerolog.Logger

	if cfg.LogFormat == "console" {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
			With().Timestamp().Logger()
	} else {
		logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	}

	switch cfg.LogLevel {
	case "debug":
		logger = logger.Level(zerolog.DebugLevel)
	case "info":
		logger = logger.Level(zerolog.InfoLevel)
	case "warn":
		logger = logger.Level(zerolog.WarnLevel)
	case "error":
		logger = logger.Level(zerolog.ErrorLevel)
	default:
		logger = logger.Level(zerolog.InfoLevel)
	}

	return logger.With().Str("component", "main").Logger()
}

// reportResourceMetrics periodically reports resource usage metrics.
func reportResourceMetrics(cfg *agent.Config, m *metrics.AgentMetrics, logger zerolog.Logger) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Get resource usage from the system
		// Note: In a real implementation, you would use the agent's Monitor
		// to get actual resource usage. Here we provide a placeholder.

		// These values would come from the agent's resource monitor
		// For now, we'll leave them as placeholders that can be integrated
		// with the agent's existing Monitor component

		// The agent's Monitor component already tracks these values
		// This goroutine is a bridge to expose them via Prometheus metrics
	}
}
