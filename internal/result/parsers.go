package result

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Parser defines the interface for test result parsers.
type Parser interface {
	// Parse reads test results from the reader and returns parsed results.
	Parse(r io.Reader) ([]TestResult, error)

	// Format returns the name of the format this parser handles.
	Format() string
}

// ParseResults parses test results from a reader using the specified format.
func ParseResults(format string, r io.Reader) ([]TestResult, error) {
	parser, err := GetParser(format)
	if err != nil {
		return nil, err
	}
	return parser.Parse(r)
}

// GetParser returns a parser for the specified format.
func GetParser(format string) (Parser, error) {
	switch strings.ToLower(format) {
	case "junit", "junit_xml":
		return &JUnitParser{}, nil
	case "jest", "jest_json":
		return &JestParser{}, nil
	case "playwright", "playwright_json":
		return &PlaywrightParser{}, nil
	case "go_test", "gotest":
		return &GoTestParser{}, nil
	case "tap":
		return &TAPParser{}, nil
	case "json", "generic":
		return &GenericJSONParser{}, nil
	default:
		return nil, fmt.Errorf("unsupported result format: %s", format)
	}
}

// JUnitParser parses JUnit XML format results.
type JUnitParser struct{}

// Format returns the format name.
func (p *JUnitParser) Format() string { return "junit" }

// JUnit XML structures
type junitTestSuites struct {
	XMLName    xml.Name         `xml:"testsuites"`
	TestSuites []junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	XMLName   xml.Name        `xml:"testsuite"`
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	Skipped   int             `xml:"skipped,attr"`
	Time      float64         `xml:"time,attr"`
	TestCases []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	XMLName   xml.Name      `xml:"testcase"`
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Time      float64       `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure"`
	Error     *junitError   `xml:"error"`
	Skipped   *junitSkipped `xml:"skipped"`
	SystemOut string        `xml:"system-out"`
	SystemErr string        `xml:"system-err"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

type junitError struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

type junitSkipped struct {
	Message string `xml:"message,attr"`
}

// Parse parses JUnit XML format.
func (p *JUnitParser) Parse(r io.Reader) ([]TestResult, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read data: %w", err)
	}

	var results []TestResult

	// Try parsing as testsuites first
	var suites junitTestSuites
	if err := xml.Unmarshal(data, &suites); err == nil && len(suites.TestSuites) > 0 {
		for _, suite := range suites.TestSuites {
			results = append(results, p.parseSuite(suite)...)
		}
		return results, nil
	}

	// Try parsing as single testsuite
	var suite junitTestSuite
	if err := xml.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("failed to parse JUnit XML: %w", err)
	}

	return p.parseSuite(suite), nil
}

func (p *JUnitParser) parseSuite(suite junitTestSuite) []TestResult {
	results := make([]TestResult, 0, len(suite.TestCases))

	for _, tc := range suite.TestCases {
		result := TestResult{
			TestName:   tc.Name,
			SuiteName:  suite.Name,
			DurationMs: int64(tc.Time * 1000),
			Stdout:     tc.SystemOut,
			Stderr:     tc.SystemErr,
		}

		if tc.ClassName != "" {
			result.TestName = tc.ClassName + "." + tc.Name
		}

		switch {
		case tc.Failure != nil:
			result.Status = "fail"
			result.ErrorMessage = tc.Failure.Message
			result.StackTrace = tc.Failure.Content
		case tc.Error != nil:
			result.Status = "error"
			result.ErrorMessage = tc.Error.Message
			result.StackTrace = tc.Error.Content
		case tc.Skipped != nil:
			result.Status = "skip"
			result.ErrorMessage = tc.Skipped.Message
		default:
			result.Status = "pass"
		}

		results = append(results, result)
	}

	return results
}

// JestParser parses Jest JSON format results.
type JestParser struct{}

// Format returns the format name.
func (p *JestParser) Format() string { return "jest" }

// Jest JSON structures
type jestResults struct {
	NumTotalTests  int              `json:"numTotalTests"`
	NumPassedTests int              `json:"numPassedTests"`
	NumFailedTests int              `json:"numFailedTests"`
	TestResults    []jestTestResult `json:"testResults"`
}

type jestTestResult struct {
	Name         string          `json:"name"`
	AssertionRes []jestAssertion `json:"assertionResults"`
	Status       string          `json:"status"`
	Message      string          `json:"message"`
	EndTime      int64           `json:"endTime"`
	StartTime    int64           `json:"startTime"`
}

type jestAssertion struct {
	Title           string   `json:"title"`
	FullName        string   `json:"fullName"`
	Status          string   `json:"status"`
	Duration        int64    `json:"duration"`
	FailureMessages []string `json:"failureMessages"`
	AncestorTitles  []string `json:"ancestorTitles"`
}

// Parse parses Jest JSON format.
func (p *JestParser) Parse(r io.Reader) ([]TestResult, error) {
	var jestRes jestResults
	if err := json.NewDecoder(r).Decode(&jestRes); err != nil {
		return nil, fmt.Errorf("failed to parse Jest JSON: %w", err)
	}

	var results []TestResult

	for _, testFile := range jestRes.TestResults {
		for _, assertion := range testFile.AssertionRes {
			result := TestResult{
				TestName:   assertion.FullName,
				SuiteName:  strings.Join(assertion.AncestorTitles, " > "),
				DurationMs: assertion.Duration,
			}

			switch assertion.Status {
			case "passed":
				result.Status = "pass"
			case "failed":
				result.Status = "fail"
				if len(assertion.FailureMessages) > 0 {
					result.ErrorMessage = strings.Join(assertion.FailureMessages, "\n")
				}
			case "pending", "skipped":
				result.Status = "skip"
			default:
				result.Status = "error"
			}

			results = append(results, result)
		}
	}

	return results, nil
}

// PlaywrightParser parses Playwright JSON format results.
type PlaywrightParser struct{}

// Format returns the format name.
func (p *PlaywrightParser) Format() string { return "playwright" }

// Playwright JSON structures
type playwrightReport struct {
	Suites []playwrightSuite `json:"suites"`
}

type playwrightSuite struct {
	Title  string            `json:"title"`
	Specs  []playwrightSpec  `json:"specs"`
	Suites []playwrightSuite `json:"suites"`
}

type playwrightSpec struct {
	Title string           `json:"title"`
	Tests []playwrightTest `json:"tests"`
	File  string           `json:"file"`
	Line  int              `json:"line"`
}

type playwrightTest struct {
	ProjectName string             `json:"projectName"`
	Results     []playwrightResult `json:"results"`
}

type playwrightResult struct {
	Status      string             `json:"status"`
	Duration    int64              `json:"duration"`
	Error       *playwrightError   `json:"error"`
	Retry       int                `json:"retry"`
	Attachments []playwrightAttach `json:"attachments"`
}

type playwrightError struct {
	Message string `json:"message"`
	Stack   string `json:"stack"`
}

type playwrightAttach struct {
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
	Path        string `json:"path"`
}

// Parse parses Playwright JSON format.
func (p *PlaywrightParser) Parse(r io.Reader) ([]TestResult, error) {
	var report playwrightReport
	if err := json.NewDecoder(r).Decode(&report); err != nil {
		return nil, fmt.Errorf("failed to parse Playwright JSON: %w", err)
	}

	var results []TestResult
	p.parseSuites("", report.Suites, &results)
	return results, nil
}

func (p *PlaywrightParser) parseSuites(prefix string, suites []playwrightSuite, results *[]TestResult) {
	for _, suite := range suites {
		suiteName := suite.Title
		if prefix != "" {
			suiteName = prefix + " > " + suite.Title
		}

		for _, spec := range suite.Specs {
			for _, test := range spec.Tests {
				if len(test.Results) == 0 {
					continue
				}

				// Use the last result (after retries)
				lastResult := test.Results[len(test.Results)-1]

				result := TestResult{
					TestName:   spec.Title,
					SuiteName:  suiteName,
					DurationMs: lastResult.Duration,
					RetryCount: lastResult.Retry,
				}

				switch lastResult.Status {
				case "passed":
					result.Status = "pass"
				case "failed":
					result.Status = "fail"
					if lastResult.Error != nil {
						result.ErrorMessage = lastResult.Error.Message
						result.StackTrace = lastResult.Error.Stack
					}
				case "skipped":
					result.Status = "skip"
				case "timedOut":
					result.Status = "error"
					result.ErrorMessage = "Test timed out"
				default:
					result.Status = "error"
				}

				*results = append(*results, result)
			}
		}

		// Recursively process nested suites
		p.parseSuites(suiteName, suite.Suites, results)
	}
}

// GoTestParser parses Go test JSON format (go test -json).
type GoTestParser struct{}

// Format returns the format name.
func (p *GoTestParser) Format() string { return "go_test" }

// Go test JSON event
type goTestEvent struct {
	Time    time.Time `json:"Time"`
	Action  string    `json:"Action"`
	Package string    `json:"Package"`
	Test    string    `json:"Test"`
	Output  string    `json:"Output"`
	Elapsed float64   `json:"Elapsed"`
}

// Parse parses Go test JSON format.
func (p *GoTestParser) Parse(r io.Reader) ([]TestResult, error) {
	testResults := make(map[string]*TestResult)
	var output strings.Builder

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		var event goTestEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue // Skip malformed lines
		}

		if event.Test == "" {
			continue // Skip package-level events
		}

		key := event.Package + "/" + event.Test

		switch event.Action {
		case "run":
			testResults[key] = &TestResult{
				TestName:  event.Test,
				SuiteName: event.Package,
			}

		case "output":
			if result, ok := testResults[key]; ok {
				output.WriteString(event.Output)
				result.Stdout = output.String()
			}

		case "pass":
			if result, ok := testResults[key]; ok {
				result.Status = "pass"
				result.DurationMs = int64(event.Elapsed * 1000)
			}

		case "fail":
			if result, ok := testResults[key]; ok {
				result.Status = "fail"
				result.DurationMs = int64(event.Elapsed * 1000)
				// Extract error message from output
				result.ErrorMessage = extractGoTestError(result.Stdout)
			}

		case "skip":
			if result, ok := testResults[key]; ok {
				result.Status = "skip"
				result.DurationMs = int64(event.Elapsed * 1000)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading go test output: %w", err)
	}

	results := make([]TestResult, 0, len(testResults))
	for _, r := range testResults {
		if r.Status != "" {
			results = append(results, *r)
		}
	}

	return results, nil
}

// extractGoTestError extracts error message from Go test output.
func extractGoTestError(output string) string {
	lines := strings.Split(output, "\n")
	var errorLines []string
	inError := false

	for _, line := range lines {
		if strings.Contains(line, "Error") || strings.Contains(line, "FAIL") {
			inError = true
		}
		if inError {
			errorLines = append(errorLines, line)
		}
	}

	if len(errorLines) > 0 {
		return strings.Join(errorLines, "\n")
	}
	return output
}

// TAPParser parses TAP (Test Anything Protocol) format.
type TAPParser struct{}

// Format returns the format name.
func (p *TAPParser) Format() string { return "tap" }

// Parse parses TAP format.
func (p *TAPParser) Parse(r io.Reader) ([]TestResult, error) {
	var results []TestResult

	okRegex := regexp.MustCompile(`^(ok|not ok)\s+(\d+)\s*-?\s*(.*)$`)
	skipRegex := regexp.MustCompile(`#\s*(?:SKIP|skip|TODO|todo)\s*(.*)$`)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()

		// Skip TAP version and plan lines
		if strings.HasPrefix(line, "TAP version") || strings.HasPrefix(line, "1..") {
			continue
		}

		// Parse test result line
		match := okRegex.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		status := match[1]
		testNum, _ := strconv.Atoi(match[2])
		description := match[3]

		result := TestResult{
			TestName: fmt.Sprintf("Test %d", testNum),
		}

		if description != "" {
			result.TestName = description
		}

		// Check for skip/todo
		if skipMatch := skipRegex.FindStringSubmatch(line); skipMatch != nil {
			result.Status = "skip"
			result.ErrorMessage = skipMatch[1]
		} else if status == "ok" {
			result.Status = "pass"
		} else {
			result.Status = "fail"
		}

		results = append(results, result)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading TAP output: %w", err)
	}

	return results, nil
}

// GenericJSONParser parses a generic JSON format for test results.
type GenericJSONParser struct{}

// Format returns the format name.
func (p *GenericJSONParser) Format() string { return "json" }

// Generic JSON structures
type genericResults struct {
	Results []genericResult `json:"results"`
	Tests   []genericResult `json:"tests"`
}

type genericResult struct {
	Name         string `json:"name"`
	TestName     string `json:"test_name"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	Result       string `json:"result"`
	Duration     int64  `json:"duration"`
	DurationMs   int64  `json:"duration_ms"`
	Time         int64  `json:"time"`
	Suite        string `json:"suite"`
	SuiteName    string `json:"suite_name"`
	Error        string `json:"error"`
	ErrorMessage string `json:"error_message"`
	Message      string `json:"message"`
	StackTrace   string `json:"stack_trace"`
	Stack        string `json:"stack"`
}

// Parse parses generic JSON format.
func (p *GenericJSONParser) Parse(r io.Reader) ([]TestResult, error) {
	var generic genericResults
	if err := json.NewDecoder(r).Decode(&generic); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Combine results and tests arrays
	allResults := append(generic.Results, generic.Tests...)

	results := make([]TestResult, 0, len(allResults))
	for _, gr := range allResults {
		result := TestResult{}

		// Name (try multiple fields)
		result.TestName = coalesce(gr.Name, gr.TestName, gr.Title)

		// Suite name
		result.SuiteName = coalesce(gr.Suite, gr.SuiteName)

		// Duration (try multiple fields)
		if gr.DurationMs > 0 {
			result.DurationMs = gr.DurationMs
		} else if gr.Duration > 0 {
			result.DurationMs = gr.Duration
		} else if gr.Time > 0 {
			result.DurationMs = gr.Time
		}

		// Status
		status := coalesce(gr.Status, gr.Result)
		switch strings.ToLower(status) {
		case "pass", "passed", "success", "ok":
			result.Status = "pass"
		case "fail", "failed", "failure", "error":
			result.Status = "fail"
		case "skip", "skipped", "pending", "todo":
			result.Status = "skip"
		default:
			result.Status = status
		}

		// Error message
		result.ErrorMessage = coalesce(gr.Error, gr.ErrorMessage, gr.Message)

		// Stack trace
		result.StackTrace = coalesce(gr.StackTrace, gr.Stack)

		if result.TestName != "" {
			results = append(results, result)
		}
	}

	return results, nil
}

// coalesce returns the first non-empty string.
func coalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
