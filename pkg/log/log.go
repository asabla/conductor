// Package log provides structured logging for the Conductor platform.
// It wraps zerolog to provide a consistent logging interface with support
// for JSON and console output formats, log levels, and context propagation.
package log

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// Logger provides structured logging with context support.
type Logger interface {
	// Debug logs a message at debug level.
	Debug() Event
	// Info logs a message at info level.
	Info() Event
	// Warn logs a message at warn level.
	Warn() Event
	// Error logs a message at error level.
	Error() Event

	// With returns a new Logger with the given key-value pair added to the context.
	With(key string, value interface{}) Logger
	// WithError returns a new Logger with the error added to the context.
	WithError(err error) Logger
	// WithContext returns a new Logger with values from the context (e.g., request ID).
	WithContext(ctx context.Context) Logger

	// Underlying returns the underlying zerolog.Logger for advanced usage.
	Underlying() *zerolog.Logger
}

// Event represents a log event that can have fields added before being sent.
type Event interface {
	// Str adds a string field to the event.
	Str(key, val string) Event
	// Int adds an integer field to the event.
	Int(key string, val int) Event
	// Int64 adds an int64 field to the event.
	Int64(key string, val int64) Event
	// Float64 adds a float64 field to the event.
	Float64(key string, val float64) Event
	// Bool adds a boolean field to the event.
	Bool(key string, val bool) Event
	// Dur adds a duration field to the event.
	Dur(key string, val time.Duration) Event
	// Time adds a time field to the event.
	Time(key string, val time.Time) Event
	// Any adds any value field to the event.
	Any(key string, val interface{}) Event
	// Err adds an error field to the event.
	Err(err error) Event
	// Msg sends the event with the given message.
	Msg(msg string)
	// Msgf sends the event with the formatted message.
	Msgf(format string, args ...interface{})
}

// logger wraps zerolog.Logger to implement the Logger interface.
type logger struct {
	zl zerolog.Logger
}

// event wraps zerolog.Event to implement the Event interface.
type event struct {
	ze *zerolog.Event
}

// New creates a new Logger with the specified level and format.
// Level should be one of: debug, info, warn, error.
// Format should be one of: json, console.
func New(level, format string) Logger {
	return NewWithWriter(level, format, os.Stdout)
}

// NewWithWriter creates a new Logger with a custom writer.
func NewWithWriter(level, format string, w io.Writer) Logger {
	// Set global settings
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.DurationFieldUnit = time.Millisecond
	zerolog.DurationFieldInteger = false

	// Configure output format
	var output io.Writer = w
	if format == "console" {
		output = zerolog.ConsoleWriter{
			Out:        w,
			TimeFormat: time.RFC3339,
		}
	}

	// Parse log level
	zl := zerolog.New(output).With().Timestamp().Logger()
	zl = zl.Level(parseLevel(level))

	return &logger{zl: zl}
}

// NewNop creates a no-op logger that discards all output.
// Useful for testing.
func NewNop() Logger {
	return &logger{zl: zerolog.Nop()}
}

// parseLevel converts a string level to zerolog.Level.
func parseLevel(level string) zerolog.Level {
	switch level {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

// Logger interface implementation

func (l *logger) Debug() Event {
	return &event{ze: l.zl.Debug()}
}

func (l *logger) Info() Event {
	return &event{ze: l.zl.Info()}
}

func (l *logger) Warn() Event {
	return &event{ze: l.zl.Warn()}
}

func (l *logger) Error() Event {
	return &event{ze: l.zl.Error()}
}

func (l *logger) With(key string, value interface{}) Logger {
	return &logger{zl: l.zl.With().Interface(key, value).Logger()}
}

func (l *logger) WithError(err error) Logger {
	return &logger{zl: l.zl.With().Err(err).Logger()}
}

func (l *logger) WithContext(ctx context.Context) Logger {
	newLogger := l.zl

	// Extract request ID from context if present
	if requestID := RequestIDFromContext(ctx); requestID != "" {
		newLogger = newLogger.With().Str("request_id", requestID).Logger()
	}

	// Extract correlation ID from context if present
	if correlationID := CorrelationIDFromContext(ctx); correlationID != "" {
		newLogger = newLogger.With().Str("correlation_id", correlationID).Logger()
	}

	// Extract user ID from context if present
	if userID := UserIDFromContext(ctx); userID != "" {
		newLogger = newLogger.With().Str("user_id", userID).Logger()
	}

	return &logger{zl: newLogger}
}

func (l *logger) Underlying() *zerolog.Logger {
	return &l.zl
}

// Event interface implementation

func (e *event) Str(key, val string) Event {
	e.ze = e.ze.Str(key, val)
	return e
}

func (e *event) Int(key string, val int) Event {
	e.ze = e.ze.Int(key, val)
	return e
}

func (e *event) Int64(key string, val int64) Event {
	e.ze = e.ze.Int64(key, val)
	return e
}

func (e *event) Float64(key string, val float64) Event {
	e.ze = e.ze.Float64(key, val)
	return e
}

func (e *event) Bool(key string, val bool) Event {
	e.ze = e.ze.Bool(key, val)
	return e
}

func (e *event) Dur(key string, val time.Duration) Event {
	e.ze = e.ze.Dur(key, val)
	return e
}

func (e *event) Time(key string, val time.Time) Event {
	e.ze = e.ze.Time(key, val)
	return e
}

func (e *event) Any(key string, val interface{}) Event {
	e.ze = e.ze.Interface(key, val)
	return e
}

func (e *event) Err(err error) Event {
	e.ze = e.ze.Err(err)
	return e
}

func (e *event) Msg(msg string) {
	e.ze.Msg(msg)
}

func (e *event) Msgf(format string, args ...interface{}) {
	e.ze.Msgf(format, args...)
}

// Context keys for extracting values

type contextKey string

const (
	requestIDKey     contextKey = "request_id"
	correlationIDKey contextKey = "correlation_id"
	userIDKey        contextKey = "user_id"
	loggerKey        contextKey = "logger"
)

// ContextWithRequestID adds a request ID to the context.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// RequestIDFromContext extracts the request ID from the context.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// ContextWithCorrelationID adds a correlation ID to the context.
func ContextWithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, correlationIDKey, correlationID)
}

// CorrelationIDFromContext extracts the correlation ID from the context.
func CorrelationIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(correlationIDKey).(string); ok {
		return id
	}
	return ""
}

// ContextWithUserID adds a user ID to the context.
func ContextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// UserIDFromContext extracts the user ID from the context.
func UserIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(userIDKey).(string); ok {
		return id
	}
	return ""
}

// ContextWithLogger adds a logger to the context.
func ContextWithLogger(ctx context.Context, log Logger) context.Context {
	return context.WithValue(ctx, loggerKey, log)
}

// FromContext extracts the logger from the context.
// Returns a no-op logger if none is present.
func FromContext(ctx context.Context) Logger {
	if log, ok := ctx.Value(loggerKey).(Logger); ok {
		return log
	}
	return NewNop()
}
