package result

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetParser(t *testing.T) {
	tests := []struct {
		name       string
		format     string
		wantFormat string
		wantErr    bool
	}{
		{"junit", "junit", "junit", false},
		{"junit_xml", "junit_xml", "junit", false},
		{"jest", "jest", "jest", false},
		{"jest_json", "jest_json", "jest", false},
		{"playwright", "playwright", "playwright", false},
		{"go_test", "go_test", "go_test", false},
		{"gotest", "gotest", "go_test", false},
		{"tap", "tap", "tap", false},
		{"json", "json", "json", false},
		{"generic", "generic", "json", false},
		{"case insensitive", "JUNIT", "junit", false},
		{"unknown format", "unknown", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := GetParser(tt.format)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFormat, parser.Format())
		})
	}
}

func TestJUnitParser_Parse(t *testing.T) {
	t.Run("parses testsuites format", func(t *testing.T) {
		xml := `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="MyTests" tests="3" failures="1" errors="0" skipped="1" time="1.234">
    <testcase name="test_passing" classname="tests.MyTests" time="0.100">
      <system-out>some output</system-out>
    </testcase>
    <testcase name="test_failing" classname="tests.MyTests" time="0.200">
      <failure message="assertion failed" type="AssertionError">
        Expected 1 but got 2
      </failure>
    </testcase>
    <testcase name="test_skipped" classname="tests.MyTests" time="0.001">
      <skipped message="not implemented yet"/>
    </testcase>
  </testsuite>
</testsuites>`

		parser := &JUnitParser{}
		results, err := parser.Parse(strings.NewReader(xml))
		require.NoError(t, err)
		require.Len(t, results, 3)

		// Passing test
		assert.Equal(t, "tests.MyTests.test_passing", results[0].TestName)
		assert.Equal(t, "pass", results[0].Status)
		assert.Equal(t, int64(100), results[0].DurationMs)
		assert.Equal(t, "some output", results[0].Stdout)

		// Failing test
		assert.Equal(t, "tests.MyTests.test_failing", results[1].TestName)
		assert.Equal(t, "fail", results[1].Status)
		assert.Equal(t, "assertion failed", results[1].ErrorMessage)
		assert.Contains(t, results[1].StackTrace, "Expected 1 but got 2")

		// Skipped test
		assert.Equal(t, "tests.MyTests.test_skipped", results[2].TestName)
		assert.Equal(t, "skip", results[2].Status)
		assert.Equal(t, "not implemented yet", results[2].ErrorMessage)
	})

	t.Run("parses single testsuite format", func(t *testing.T) {
		xml := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="SingleSuite" tests="1" failures="0" errors="0" time="0.5">
  <testcase name="test_one" time="0.5"/>
</testsuite>`

		parser := &JUnitParser{}
		results, err := parser.Parse(strings.NewReader(xml))
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "pass", results[0].Status)
	})

	t.Run("handles error testcases", func(t *testing.T) {
		xml := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="ErrorSuite" tests="1" errors="1">
  <testcase name="test_error" time="0.1">
    <error message="null pointer" type="NullPointerException">
      Stack trace here
    </error>
  </testcase>
</testsuite>`

		parser := &JUnitParser{}
		results, err := parser.Parse(strings.NewReader(xml))
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "error", results[0].Status)
		assert.Equal(t, "null pointer", results[0].ErrorMessage)
	})

	t.Run("returns error for invalid XML", func(t *testing.T) {
		parser := &JUnitParser{}
		_, err := parser.Parse(strings.NewReader("not xml"))
		assert.Error(t, err)
	})
}

func TestJestParser_Parse(t *testing.T) {
	t.Run("parses jest output", func(t *testing.T) {
		json := `{
  "numTotalTests": 3,
  "numPassedTests": 2,
  "numFailedTests": 1,
  "testResults": [
    {
      "name": "/path/to/test.spec.js",
      "assertionResults": [
        {
          "title": "should add numbers",
          "fullName": "Calculator should add numbers",
          "status": "passed",
          "duration": 10,
          "ancestorTitles": ["Calculator"]
        },
        {
          "title": "should subtract numbers",
          "fullName": "Calculator should subtract numbers",
          "status": "passed",
          "duration": 5,
          "ancestorTitles": ["Calculator"]
        },
        {
          "title": "should divide numbers",
          "fullName": "Calculator should divide numbers",
          "status": "failed",
          "duration": 15,
          "failureMessages": ["Error: expected 0 to equal Infinity", "at divide (calc.js:10)"],
          "ancestorTitles": ["Calculator"]
        }
      ]
    }
  ]
}`

		parser := &JestParser{}
		results, err := parser.Parse(strings.NewReader(json))
		require.NoError(t, err)
		require.Len(t, results, 3)

		assert.Equal(t, "Calculator should add numbers", results[0].TestName)
		assert.Equal(t, "pass", results[0].Status)
		assert.Equal(t, int64(10), results[0].DurationMs)
		assert.Equal(t, "Calculator", results[0].SuiteName)

		assert.Equal(t, "fail", results[2].Status)
		assert.Contains(t, results[2].ErrorMessage, "expected 0 to equal Infinity")
	})

	t.Run("handles pending tests", func(t *testing.T) {
		json := `{
  "numTotalTests": 1,
  "testResults": [{
    "name": "test.js",
    "assertionResults": [{
      "title": "pending test",
      "fullName": "pending test",
      "status": "pending",
      "ancestorTitles": []
    }]
  }]
}`

		parser := &JestParser{}
		results, err := parser.Parse(strings.NewReader(json))
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "skip", results[0].Status)
	})
}

func TestGoTestParser_Parse(t *testing.T) {
	t.Run("parses go test json output", func(t *testing.T) {
		output := `{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"mypackage","Test":"TestAdd"}
{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"mypackage","Test":"TestAdd","Output":"=== RUN   TestAdd\n"}
{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"mypackage","Test":"TestAdd","Output":"--- PASS: TestAdd (0.01s)\n"}
{"Time":"2024-01-01T00:00:01Z","Action":"pass","Package":"mypackage","Test":"TestAdd","Elapsed":0.01}
{"Time":"2024-01-01T00:00:01Z","Action":"run","Package":"mypackage","Test":"TestSubtract"}
{"Time":"2024-01-01T00:00:01Z","Action":"output","Package":"mypackage","Test":"TestSubtract","Output":"=== RUN   TestSubtract\n"}
{"Time":"2024-01-01T00:00:01Z","Action":"output","Package":"mypackage","Test":"TestSubtract","Output":"    subtract_test.go:15: Error: expected 5 but got 3\n"}
{"Time":"2024-01-01T00:00:02Z","Action":"fail","Package":"mypackage","Test":"TestSubtract","Elapsed":0.02}`

		parser := &GoTestParser{}
		results, err := parser.Parse(strings.NewReader(output))
		require.NoError(t, err)
		require.Len(t, results, 2)

		// Find results by name since map iteration order is not guaranteed
		var passResult, failResult *TestResult
		for i := range results {
			if results[i].TestName == "TestAdd" {
				passResult = &results[i]
			} else if results[i].TestName == "TestSubtract" {
				failResult = &results[i]
			}
		}

		require.NotNil(t, passResult)
		assert.Equal(t, "pass", passResult.Status)
		assert.Equal(t, "mypackage", passResult.SuiteName)
		assert.Equal(t, int64(10), passResult.DurationMs)

		require.NotNil(t, failResult)
		assert.Equal(t, "fail", failResult.Status)
		assert.Contains(t, failResult.Stdout, "Error")
	})

	t.Run("handles skipped tests", func(t *testing.T) {
		output := `{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"pkg","Test":"TestSkipped"}
{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"pkg","Test":"TestSkipped","Output":"--- SKIP: TestSkipped\n"}
{"Time":"2024-01-01T00:00:00Z","Action":"skip","Package":"pkg","Test":"TestSkipped","Elapsed":0.001}`

		parser := &GoTestParser{}
		results, err := parser.Parse(strings.NewReader(output))
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "skip", results[0].Status)
	})

	t.Run("skips package-level events", func(t *testing.T) {
		output := `{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"pkg","Output":"PASS\n"}
{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"pkg","Elapsed":1.0}`

		parser := &GoTestParser{}
		results, err := parser.Parse(strings.NewReader(output))
		require.NoError(t, err)
		assert.Len(t, results, 0) // No test-level results
	})
}

func TestPlaywrightParser_Parse(t *testing.T) {
	t.Run("parses playwright json report", func(t *testing.T) {
		json := `{
  "suites": [
    {
      "title": "Login Page",
      "specs": [
        {
          "title": "should display login form",
          "file": "login.spec.ts",
          "line": 5,
          "tests": [
            {
              "projectName": "chromium",
              "results": [
                {"status": "passed", "duration": 1500, "retry": 0}
              ]
            }
          ]
        }
      ],
      "suites": [
        {
          "title": "validation",
          "specs": [
            {
              "title": "should show error on invalid input",
              "tests": [
                {
                  "projectName": "chromium",
                  "results": [
                    {"status": "failed", "duration": 2000, "retry": 0, "error": {"message": "Locator not found", "stack": "at login.spec.ts:20"}},
                    {"status": "passed", "duration": 1800, "retry": 1}
                  ]
                }
              ]
            }
          ],
          "suites": []
        }
      ]
    }
  ]
}`

		parser := &PlaywrightParser{}
		results, err := parser.Parse(strings.NewReader(json))
		require.NoError(t, err)
		require.Len(t, results, 2)

		// First test
		assert.Equal(t, "should display login form", results[0].TestName)
		assert.Equal(t, "Login Page", results[0].SuiteName)
		assert.Equal(t, "pass", results[0].Status)
		assert.Equal(t, int64(1500), results[0].DurationMs)

		// Second test (with retry - should use last result)
		assert.Equal(t, "should show error on invalid input", results[1].TestName)
		assert.Equal(t, "Login Page > validation", results[1].SuiteName)
		assert.Equal(t, "pass", results[1].Status) // Passed on retry
		assert.Equal(t, 1, results[1].RetryCount)
	})

	t.Run("handles timed out tests", func(t *testing.T) {
		json := `{
  "suites": [{
    "title": "Slow Tests",
    "specs": [{
      "title": "should timeout",
      "tests": [{
        "projectName": "firefox",
        "results": [{"status": "timedOut", "duration": 30000}]
      }]
    }],
    "suites": []
  }]
}`

		parser := &PlaywrightParser{}
		results, err := parser.Parse(strings.NewReader(json))
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "error", results[0].Status)
		assert.Equal(t, "Test timed out", results[0].ErrorMessage)
	})
}

func TestTAPParser_Parse(t *testing.T) {
	t.Run("parses TAP output", func(t *testing.T) {
		tap := `TAP version 13
1..4
ok 1 - should pass
not ok 2 - should fail
ok 3 - should be skipped # SKIP not implemented
ok 4 - this is a todo # TODO implement later`

		parser := &TAPParser{}
		results, err := parser.Parse(strings.NewReader(tap))
		require.NoError(t, err)
		require.Len(t, results, 4)

		assert.Equal(t, "should pass", results[0].TestName)
		assert.Equal(t, "pass", results[0].Status)

		assert.Equal(t, "should fail", results[1].TestName)
		assert.Equal(t, "fail", results[1].Status)

		// TAP parser includes the directive in the description
		assert.Contains(t, results[2].TestName, "should be skipped")
		assert.Equal(t, "skip", results[2].Status)
		assert.Equal(t, "not implemented", results[2].ErrorMessage)

		assert.Contains(t, results[3].TestName, "this is a todo")
		assert.Equal(t, "skip", results[3].Status)
	})

	t.Run("handles tests without description", func(t *testing.T) {
		tap := `1..2
ok 1
not ok 2`

		parser := &TAPParser{}
		results, err := parser.Parse(strings.NewReader(tap))
		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.Equal(t, "Test 1", results[0].TestName)
		assert.Equal(t, "Test 2", results[1].TestName)
	})
}

func TestGenericJSONParser_Parse(t *testing.T) {
	t.Run("parses results array format", func(t *testing.T) {
		json := `{
  "results": [
    {"name": "test1", "status": "pass", "duration_ms": 100},
    {"name": "test2", "status": "fail", "duration_ms": 200, "error": "assertion failed"},
    {"name": "test3", "status": "skip", "duration_ms": 0}
  ]
}`

		parser := &GenericJSONParser{}
		results, err := parser.Parse(strings.NewReader(json))
		require.NoError(t, err)
		require.Len(t, results, 3)

		assert.Equal(t, "test1", results[0].TestName)
		assert.Equal(t, "pass", results[0].Status)
		assert.Equal(t, int64(100), results[0].DurationMs)

		assert.Equal(t, "fail", results[1].Status)
		assert.Equal(t, "assertion failed", results[1].ErrorMessage)

		assert.Equal(t, "skip", results[2].Status)
	})

	t.Run("parses tests array format", func(t *testing.T) {
		json := `{
  "tests": [
    {"test_name": "my_test", "result": "passed", "duration": 50}
  ]
}`

		parser := &GenericJSONParser{}
		results, err := parser.Parse(strings.NewReader(json))
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "my_test", results[0].TestName)
		assert.Equal(t, "pass", results[0].Status)
	})

	t.Run("normalizes status values", func(t *testing.T) {
		testCases := []struct {
			input    string
			expected string
		}{
			{`{"results": [{"name": "t", "status": "passed"}]}`, "pass"},
			{`{"results": [{"name": "t", "status": "success"}]}`, "pass"},
			{`{"results": [{"name": "t", "status": "ok"}]}`, "pass"},
			{`{"results": [{"name": "t", "status": "failed"}]}`, "fail"},
			{`{"results": [{"name": "t", "status": "failure"}]}`, "fail"},
			{`{"results": [{"name": "t", "status": "error"}]}`, "fail"},
			{`{"results": [{"name": "t", "status": "skipped"}]}`, "skip"},
			{`{"results": [{"name": "t", "status": "pending"}]}`, "skip"},
		}

		parser := &GenericJSONParser{}
		for _, tc := range testCases {
			results, err := parser.Parse(strings.NewReader(tc.input))
			require.NoError(t, err)
			require.Len(t, results, 1)
			assert.Equal(t, tc.expected, results[0].Status, "input: %s", tc.input)
		}
	})

	t.Run("handles mixed field names", func(t *testing.T) {
		json := `{
  "results": [
    {"title": "Test Title", "suite_name": "My Suite", "time": 250, "stack_trace": "at line 10"}
  ]
}`

		parser := &GenericJSONParser{}
		results, err := parser.Parse(strings.NewReader(json))
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Test Title", results[0].TestName)
		assert.Equal(t, "My Suite", results[0].SuiteName)
		assert.Equal(t, int64(250), results[0].DurationMs)
		assert.Equal(t, "at line 10", results[0].StackTrace)
	})

	t.Run("skips entries without name", func(t *testing.T) {
		json := `{
  "results": [
    {"status": "pass"},
    {"name": "valid", "status": "pass"}
  ]
}`

		parser := &GenericJSONParser{}
		results, err := parser.Parse(strings.NewReader(json))
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "valid", results[0].TestName)
	})
}

func TestParseResults(t *testing.T) {
	t.Run("uses correct parser", func(t *testing.T) {
		xml := `<testsuite name="Test"><testcase name="test1"/></testsuite>`
		results, err := ParseResults("junit", strings.NewReader(xml))
		require.NoError(t, err)
		require.Len(t, results, 1)
	})

	t.Run("returns error for unknown format", func(t *testing.T) {
		_, err := ParseResults("unknown", strings.NewReader(""))
		assert.Error(t, err)
	})
}

func TestCoalesce(t *testing.T) {
	assert.Equal(t, "first", coalesce("first", "second"))
	assert.Equal(t, "second", coalesce("", "second"))
	assert.Equal(t, "third", coalesce("", "", "third"))
	assert.Equal(t, "", coalesce("", "", ""))
}
