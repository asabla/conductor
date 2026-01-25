package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Color codes
var (
	colorEnabled = true

	resetCode   = "\033[0m"
	boldCode    = "\033[1m"
	dimCode     = "\033[2m"
	redCode     = "\033[31m"
	greenCode   = "\033[32m"
	yellowCode  = "\033[33m"
	blueCode    = "\033[34m"
	magentaCode = "\033[35m"
	cyanCode    = "\033[36m"
)

// InitColor initializes color output based on environment
func InitColor(enabled bool) {
	colorEnabled = enabled

	// Disable colors if not a terminal
	if !isTerminal() {
		colorEnabled = false
	}

	// Check NO_COLOR environment variable
	if os.Getenv("NO_COLOR") != "" {
		colorEnabled = false
	}
}

// isTerminal checks if stdout is a terminal
func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// Color functions
func colorize(s, code string) string {
	if !colorEnabled {
		return s
	}
	return code + s + resetCode
}

// Bold returns bold text
func Bold(s string) string {
	return colorize(s, boldCode)
}

// Dim returns dimmed text
func Dim(s string) string {
	return colorize(s, dimCode)
}

// Red returns red text
func Red(s string) string {
	return colorize(s, redCode)
}

// Green returns green text
func Green(s string) string {
	return colorize(s, greenCode)
}

// Yellow returns yellow text
func Yellow(s string) string {
	return colorize(s, yellowCode)
}

// Blue returns blue text
func Blue(s string) string {
	return colorize(s, blueCode)
}

// Magenta returns magenta text
func Magenta(s string) string {
	return colorize(s, magentaCode)
}

// Cyan returns cyan text
func Cyan(s string) string {
	return colorize(s, cyanCode)
}

// OutputFormatter is the interface for output formatters
type OutputFormatter interface {
	Format(data interface{}) (string, error)
}

// JSONFormatter formats output as JSON
type JSONFormatter struct {
	Indent bool
}

// Format formats data as JSON
func (f *JSONFormatter) Format(data interface{}) (string, error) {
	var out []byte
	var err error
	if f.Indent {
		out, err = json.MarshalIndent(data, "", "  ")
	} else {
		out, err = json.MarshalIndent(data, "", "  ")
	}
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// TableFormatter formats output as an ASCII table
type TableFormatter struct {
	Headers []string
	Rows    [][]string
}

// Format formats data as a table
func (f *TableFormatter) Format(data interface{}) (string, error) {
	return formatTable(f.Headers, f.Rows), nil
}

// printJSON prints data as formatted JSON
func printJSON(data interface{}) error {
	formatter := &JSONFormatter{Indent: true}
	output, err := formatter.Format(data)
	if err != nil {
		return err
	}
	fmt.Println(output)
	return nil
}

// printTable prints an ASCII table
func printTable(headers []string, rows [][]string) {
	fmt.Print(formatTable(headers, rows))
}

// formatTable creates an ASCII table string
func formatTable(headers []string, rows [][]string) string {
	if len(headers) == 0 {
		return ""
	}

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(stripAnsi(h))
	}

	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				cellLen := len(stripAnsi(cell))
				if cellLen > widths[i] {
					widths[i] = cellLen
				}
			}
		}
	}

	// Build output
	var sb strings.Builder

	// Print headers
	for i, h := range headers {
		sb.WriteString(padRight(h, widths[i]))
		if i < len(headers)-1 {
			sb.WriteString("  ")
		}
	}
	sb.WriteString("\n")

	// Print separator
	for i, w := range widths {
		sb.WriteString(strings.Repeat("-", w))
		if i < len(widths)-1 {
			sb.WriteString("  ")
		}
	}
	sb.WriteString("\n")

	// Print rows
	for _, row := range rows {
		for i := 0; i < len(headers); i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			sb.WriteString(padRight(cell, widths[i]))
			if i < len(headers)-1 {
				sb.WriteString("  ")
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// stripAnsi removes ANSI color codes from a string
func stripAnsi(s string) string {
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

// padRight pads a string to the given width, accounting for ANSI codes
func padRight(s string, width int) string {
	stripped := stripAnsi(s)
	padding := width - len(stripped)
	if padding <= 0 {
		return s
	}
	return s + strings.Repeat(" ", padding)
}

// truncate truncates a string to the given length
func truncate(s string, length int) string {
	stripped := stripAnsi(s)
	if len(stripped) <= length {
		return s
	}
	if length <= 3 {
		return s[:length]
	}
	return s[:length-3] + "..."
}

// formatTimestamp formats a timestamp string for display
func formatTimestamp(ts string) string {
	if ts == "" {
		return Dim("-")
	}

	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		t, err = time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return ts
		}
	}

	now := time.Now()
	diff := now.Sub(t)

	// Show relative time for recent timestamps
	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Format("2006-01-02 15:04")
	}
}

// formatDuration formats a Duration for display
func formatDuration(d *Duration) string {
	if d == nil {
		return "-"
	}

	totalSecs := d.Seconds
	if totalSecs == 0 && d.Nanos > 0 {
		return fmt.Sprintf("%dms", d.Nanos/1000000)
	}

	if totalSecs < 60 {
		return fmt.Sprintf("%ds", totalSecs)
	}

	mins := totalSecs / 60
	secs := totalSecs % 60
	if mins < 60 {
		if secs == 0 {
			return fmt.Sprintf("%dm", mins)
		}
		return fmt.Sprintf("%dm%ds", mins, secs)
	}

	hours := mins / 60
	mins = mins % 60
	if mins == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh%dm", hours, mins)
}

// formatBytes formats bytes for display
func formatBytes(b int64) string {
	if b == 0 {
		return "0 B"
	}

	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}

	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), units[exp])
}

// formatBool formats a boolean for display
func formatBool(b bool) string {
	if b {
		return Green("yes")
	}
	return Red("no")
}

// Spinner for long-running operations
var spinnerActive = false

// ShowSpinner displays a loading spinner with message
func ShowSpinner(msg string) {
	if !isTerminal() {
		fmt.Println(msg)
		return
	}
	spinnerActive = true
	fmt.Printf("\r%s %s", Dim("⠋"), msg)
}

// HideSpinner hides the spinner
func HideSpinner() {
	if !spinnerActive {
		return
	}
	spinnerActive = false
	fmt.Print("\r\033[K") // Clear line
}

// ProgressBar displays a progress bar
type ProgressBar struct {
	Total   int
	Current int
	Width   int
	Message string
}

// NewProgressBar creates a new progress bar
func NewProgressBar(total int, message string) *ProgressBar {
	return &ProgressBar{
		Total:   total,
		Current: 0,
		Width:   40,
		Message: message,
	}
}

// Update updates the progress bar
func (p *ProgressBar) Update(current int) {
	p.Current = current
	if !isTerminal() {
		return
	}

	percent := float64(p.Current) / float64(p.Total)
	filled := int(percent * float64(p.Width))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", p.Width-filled)

	fmt.Printf("\r%s [%s] %d%% (%d/%d)",
		p.Message, bar, int(percent*100), p.Current, p.Total)
}

// Done completes the progress bar
func (p *ProgressBar) Done() {
	if isTerminal() {
		fmt.Print("\r\033[K")
	}
}

// Success prints a success message
func Success(msg string) {
	fmt.Printf("%s %s\n", Green("✓"), msg)
}

// Error prints an error message
func Error(msg string) {
	fmt.Printf("%s %s\n", Red("✗"), msg)
}

// Warning prints a warning message
func Warning(msg string) {
	fmt.Printf("%s %s\n", Yellow("!"), msg)
}

// Info prints an info message
func Info(msg string) {
	fmt.Printf("%s %s\n", Blue("→"), msg)
}
