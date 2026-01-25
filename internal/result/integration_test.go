//go:build integration

// Package result provides integration tests for result parsing and collection.
package result

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/conductor/conductor/internal/artifact"
	"github.com/conductor/conductor/internal/database"
	"github.com/conductor/conductor/pkg/testfixtures"
	"github.com/conductor/conductor/pkg/testutil"
)

// testDB holds the shared database for result integration tests.
var testInfra struct {
	db       *database.DB
	storage  *artifact.Storage
	postgres *testutil.PostgresContainer
	minio    *testutil.MinioContainer
}

func TestMain(m *testing.M) {
	if !testutil.IsDockerAvailable() {
		os.Exit(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start postgres
	pg, err := testutil.NewPostgresContainer(ctx, testutil.DefaultPostgresConfig())
	if err != nil {
		panic("failed to start postgres: " + err.Error())
	}
	testInfra.postgres = pg

	// Create database connection
	dbCfg := database.DefaultConfig(pg.ConnStr)
	dbCfg.MaxConns = 5
	dbCfg.MinConns = 1
	db, err := database.New(ctx, dbCfg)
	if err != nil {
		pg.Terminate(ctx)
		panic("failed to create database: " + err.Error())
	}
	testInfra.db = db

	// Run migrations
	migrationsFS := os.DirFS("../../migrations")
	migrator, err := database.NewMigratorFromFS(db, migrationsFS)
	if err != nil {
		db.Close()
		pg.Terminate(ctx)
		panic("failed to create migrator: " + err.Error())
	}
	if _, err := migrator.Up(ctx); err != nil {
		db.Close()
		pg.Terminate(ctx)
		panic("failed to run migrations: " + err.Error())
	}

	// Start minio
	mc, err := testutil.NewMinioContainer(ctx, testutil.DefaultMinioConfig())
	if err != nil {
		db.Close()
		pg.Terminate(ctx)
		panic("failed to start minio: " + err.Error())
	}
	testInfra.minio = mc

	// Create storage client
	storage, err := artifact.NewStorage(artifact.StorageConfig{
		Endpoint:        mc.Endpoint,
		Bucket:          "conductor-test",
		Region:          "us-east-1",
		AccessKeyID:     mc.AccessKeyID,
		SecretAccessKey: mc.SecretAccessKey,
		UseSSL:          false,
		PathStyle:       true,
	}, nil)
	if err != nil {
		mc.Terminate(ctx)
		db.Close()
		pg.Terminate(ctx)
		panic("failed to create storage: " + err.Error())
	}
	testInfra.storage = storage

	// Ensure bucket exists
	if err := storage.EnsureBucket(ctx); err != nil {
		mc.Terminate(ctx)
		db.Close()
		pg.Terminate(ctx)
		panic("failed to ensure bucket: " + err.Error())
	}

	// Run tests
	code := m.Run()

	// Cleanup
	mc.Terminate(context.Background())
	db.Close()
	pg.Terminate(context.Background())

	os.Exit(code)
}

// ============================================================================
// JUNIT PARSER TESTS
// ============================================================================

func TestJUnitParser(t *testing.T) {
	parser := &JUnitParser{}

	t.Run("ParseTestSuites", func(t *testing.T) {
		xml := testfixtures.SampleJUnitXML()
		results, err := parser.Parse(strings.NewReader(xml))

		require.NoError(t, err)
		assert.Len(t, results, 4)

		// Check individual results
		passCount := 0
		failCount := 0
		skipCount := 0

		for _, r := range results {
			switch r.Status {
			case "pass":
				passCount++
			case "fail":
				failCount++
				assert.NotEmpty(t, r.ErrorMessage)
			case "skip":
				skipCount++
			}
		}

		assert.Equal(t, 2, passCount)
		assert.Equal(t, 1, failCount)
		assert.Equal(t, 1, skipCount)
	})

	t.Run("ParseSingleTestSuite", func(t *testing.T) {
		xml := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="SingleSuite" tests="2" failures="0">
  <testcase name="Test1" classname="pkg.Single" time="0.05"/>
  <testcase name="Test2" classname="pkg.Single" time="0.10"/>
</testsuite>`

		results, err := parser.Parse(strings.NewReader(xml))
		require.NoError(t, err)
		assert.Len(t, results, 2)

		for _, r := range results {
			assert.Equal(t, "pass", r.Status)
			assert.Equal(t, "SingleSuite", r.SuiteName)
		}
	})

	t.Run("ParseWithSystemOut", func(t *testing.T) {
		xml := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="Suite">
  <testcase name="TestWithOutput" classname="pkg.Suite" time="0.1">
    <system-out>This is stdout output</system-out>
    <system-err>This is stderr output</system-err>
  </testcase>
</testsuite>`

		results, err := parser.Parse(strings.NewReader(xml))
		require.NoError(t, err)
		require.Len(t, results, 1)

		assert.Equal(t, "This is stdout output", results[0].Stdout)
		assert.Equal(t, "This is stderr output", results[0].Stderr)
	})

	t.Run("ParseError", func(t *testing.T) {
		xml := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="Suite">
  <testcase name="TestError" classname="pkg.Suite" time="0.1">
    <error message="Runtime error" type="RuntimeError">Error stack trace</error>
  </testcase>
</testsuite>`

		results, err := parser.Parse(strings.NewReader(xml))
		require.NoError(t, err)
		require.Len(t, results, 1)

		assert.Equal(t, "error", results[0].Status)
		assert.Equal(t, "Runtime error", results[0].ErrorMessage)
		assert.Contains(t, results[0].StackTrace, "Error stack trace")
	})
}

// ============================================================================
// JEST PARSER TESTS
// ============================================================================

func TestJestParser(t *testing.T) {
	parser := &JestParser{}

	t.Run("ParseJestJSON", func(t *testing.T) {
		json := testfixtures.SampleJestJSON()
		results, err := parser.Parse(strings.NewReader(json))

		require.NoError(t, err)
		assert.Len(t, results, 3)

		passCount := 0
		failCount := 0
		for _, r := range results {
			switch r.Status {
			case "pass":
				passCount++
				assert.NotEmpty(t, r.SuiteName)
			case "fail":
				failCount++
				assert.NotEmpty(t, r.ErrorMessage)
			}
		}

		assert.Equal(t, 2, passCount)
		assert.Equal(t, 1, failCount)
	})

	t.Run("ParsePendingTests", func(t *testing.T) {
		json := `{
			"numTotalTests": 1,
			"numPassedTests": 0,
			"numFailedTests": 0,
			"testResults": [{
				"name": "test.js",
				"status": "passed",
				"assertionResults": [{
					"title": "pending test",
					"fullName": "Suite pending test",
					"status": "pending",
					"duration": 0,
					"failureMessages": [],
					"ancestorTitles": ["Suite"]
				}]
			}]
		}`

		results, err := parser.Parse(strings.NewReader(json))
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "skip", results[0].Status)
	})
}

// ============================================================================
// PLAYWRIGHT PARSER TESTS
// ============================================================================

func TestPlaywrightParser(t *testing.T) {
	parser := &PlaywrightParser{}

	t.Run("ParsePlaywrightJSON", func(t *testing.T) {
		json := testfixtures.SamplePlaywrightJSON()
		results, err := parser.Parse(strings.NewReader(json))

		require.NoError(t, err)
		assert.Len(t, results, 2)

		// Find the test that had retries
		for _, r := range results {
			if r.TestName == "should show error on invalid password" {
				assert.Equal(t, "pass", r.Status, "should use final retry status")
				assert.Equal(t, 1, r.RetryCount, "should record retry count")
			}
		}
	})

	t.Run("ParseNestedSuites", func(t *testing.T) {
		json := `{
			"suites": [{
				"title": "Parent",
				"specs": [],
				"suites": [{
					"title": "Child",
					"specs": [{
						"title": "nested test",
						"tests": [{
							"projectName": "chromium",
							"results": [{"status": "passed", "duration": 100, "retry": 0}]
						}]
					}],
					"suites": []
				}]
			}]
		}`

		results, err := parser.Parse(strings.NewReader(json))
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Parent > Child", results[0].SuiteName)
	})
}

// ============================================================================
// GO TEST PARSER TESTS
// ============================================================================

func TestGoTestParser(t *testing.T) {
	parser := &GoTestParser{}

	t.Run("ParseGoTestJSON", func(t *testing.T) {
		json := testfixtures.SampleGoTestJSON()
		results, err := parser.Parse(strings.NewReader(json))

		require.NoError(t, err)
		assert.Len(t, results, 3)

		statusMap := make(map[string]string)
		for _, r := range results {
			statusMap[r.TestName] = r.Status
		}

		assert.Equal(t, "pass", statusMap["TestExample1"])
		assert.Equal(t, "fail", statusMap["TestExample2"])
		assert.Equal(t, "skip", statusMap["TestExample3"])
	})

	t.Run("ParseCapturesOutput", func(t *testing.T) {
		json := `{"Time":"2024-01-25T10:00:00Z","Action":"run","Package":"pkg","Test":"TestOutput"}
{"Time":"2024-01-25T10:00:00.1Z","Action":"output","Package":"pkg","Test":"TestOutput","Output":"line 1\n"}
{"Time":"2024-01-25T10:00:00.2Z","Action":"output","Package":"pkg","Test":"TestOutput","Output":"line 2\n"}
{"Time":"2024-01-25T10:00:00.3Z","Action":"pass","Package":"pkg","Test":"TestOutput","Elapsed":0.3}`

		results, err := parser.Parse(strings.NewReader(json))
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Contains(t, results[0].Stdout, "line 1")
		assert.Contains(t, results[0].Stdout, "line 2")
	})
}

// ============================================================================
// TAP PARSER TESTS
// ============================================================================

func TestTAPParser(t *testing.T) {
	parser := &TAPParser{}

	t.Run("ParseTAP", func(t *testing.T) {
		tap := testfixtures.SampleTAPOutput()
		results, err := parser.Parse(strings.NewReader(tap))

		require.NoError(t, err)
		assert.Len(t, results, 4)

		passCount := 0
		failCount := 0
		skipCount := 0
		for _, r := range results {
			switch r.Status {
			case "pass":
				passCount++
			case "fail":
				failCount++
			case "skip":
				skipCount++
			}
		}

		assert.Equal(t, 2, passCount)
		assert.Equal(t, 1, failCount)
		assert.Equal(t, 1, skipCount)
	})

	t.Run("ParseTAPWithDescriptions", func(t *testing.T) {
		tap := `TAP version 13
1..2
ok 1 - First test with description
not ok 2 - Second test fails`

		results, err := parser.Parse(strings.NewReader(tap))
		require.NoError(t, err)
		require.Len(t, results, 2)

		assert.Equal(t, "First test with description", results[0].TestName)
		assert.Equal(t, "pass", results[0].Status)
		assert.Equal(t, "Second test fails", results[1].TestName)
		assert.Equal(t, "fail", results[1].Status)
	})
}

// ============================================================================
// GENERIC JSON PARSER TESTS
// ============================================================================

func TestGenericJSONParser(t *testing.T) {
	parser := &GenericJSONParser{}

	t.Run("ParseResultsArray", func(t *testing.T) {
		json := `{
			"results": [
				{"name": "Test1", "status": "pass", "duration_ms": 100},
				{"name": "Test2", "status": "fail", "error": "Failed assertion"}
			]
		}`

		results, err := parser.Parse(strings.NewReader(json))
		require.NoError(t, err)
		assert.Len(t, results, 2)

		assert.Equal(t, "Test1", results[0].TestName)
		assert.Equal(t, "pass", results[0].Status)
		assert.Equal(t, int64(100), results[0].DurationMs)

		assert.Equal(t, "Test2", results[1].TestName)
		assert.Equal(t, "fail", results[1].Status)
		assert.Equal(t, "Failed assertion", results[1].ErrorMessage)
	})

	t.Run("ParseTestsArray", func(t *testing.T) {
		json := `{
			"tests": [
				{"test_name": "TestA", "result": "success", "suite": "MySuite"}
			]
		}`

		results, err := parser.Parse(strings.NewReader(json))
		require.NoError(t, err)
		require.Len(t, results, 1)

		assert.Equal(t, "TestA", results[0].TestName)
		assert.Equal(t, "pass", results[0].Status)
		assert.Equal(t, "MySuite", results[0].SuiteName)
	})
}

// ============================================================================
// PARSE RESULTS FUNCTION TESTS
// ============================================================================

func TestIntegration_ParseResults(t *testing.T) {
	t.Run("JUnit", func(t *testing.T) {
		results, err := ParseResults("junit", strings.NewReader(testfixtures.SampleJUnitXML()))
		require.NoError(t, err)
		assert.NotEmpty(t, results)
	})

	t.Run("Jest", func(t *testing.T) {
		results, err := ParseResults("jest", strings.NewReader(testfixtures.SampleJestJSON()))
		require.NoError(t, err)
		assert.NotEmpty(t, results)
	})

	t.Run("GoTest", func(t *testing.T) {
		results, err := ParseResults("go_test", strings.NewReader(testfixtures.SampleGoTestJSON()))
		require.NoError(t, err)
		assert.NotEmpty(t, results)
	})

	t.Run("TAP", func(t *testing.T) {
		results, err := ParseResults("tap", strings.NewReader(testfixtures.SampleTAPOutput()))
		require.NoError(t, err)
		assert.NotEmpty(t, results)
	})

	t.Run("UnsupportedFormat", func(t *testing.T) {
		_, err := ParseResults("unknown", strings.NewReader(""))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported")
	})
}

// ============================================================================
// COLLECTOR INTEGRATION TESTS
// ============================================================================

func TestCollector(t *testing.T) {
	if testInfra.db == nil {
		t.Skip("Database not available")
	}

	ctx := context.Background()
	repos := database.NewRepositories(testInfra.db)

	// Create test service
	svc := &database.Service{
		Name:          "collector-test-service-" + uuid.New().String()[:8],
		GitURL:        "https://github.com/example/repo.git",
		DefaultBranch: "main",
	}
	err := repos.Services.Create(ctx, svc)
	require.NoError(t, err)
	defer repos.Services.Delete(ctx, svc.ID)

	// Create test run
	run := &database.TestRun{
		ServiceID: svc.ID,
		Status:    database.RunStatusRunning,
	}
	err = repos.Runs.Create(ctx, run)
	require.NoError(t, err)

	collector := NewCollector(
		repos.Runs,
		repos.Results,
		repos.Artifacts,
		testInfra.storage,
		nil,
	)

	t.Run("ProcessResults", func(t *testing.T) {
		results := []TestResult{
			{
				TestName:   "TestPass",
				SuiteName:  "TestSuite",
				Status:     "pass",
				DurationMs: 100,
			},
			{
				TestName:     "TestFail",
				SuiteName:    "TestSuite",
				Status:       "fail",
				DurationMs:   200,
				ErrorMessage: "assertion failed",
				StackTrace:   "at line 10",
			},
			{
				TestName:   "TestSkip",
				SuiteName:  "TestSuite",
				Status:     "skip",
				DurationMs: 0,
			},
		}

		err := collector.ProcessResults(ctx, run.ID, results)
		require.NoError(t, err)

		// Verify results were stored
		stored, err := collector.GetRunResults(ctx, run.ID)
		require.NoError(t, err)
		assert.Len(t, stored, 3)

		// Check status mapping
		statusCounts := make(map[database.ResultStatus]int)
		for _, r := range stored {
			statusCounts[r.Status]++
		}
		assert.Equal(t, 1, statusCounts[database.ResultStatusPass])
		assert.Equal(t, 1, statusCounts[database.ResultStatusFail])
		assert.Equal(t, 1, statusCounts[database.ResultStatusSkip])
	})

	t.Run("UpdateRunSummary", func(t *testing.T) {
		err := collector.UpdateRunSummary(ctx, run.ID)
		require.NoError(t, err)

		// Verify run was updated
		updatedRun, err := repos.Runs.Get(ctx, run.ID)
		require.NoError(t, err)

		assert.Equal(t, 3, updatedRun.TotalTests)
		assert.Equal(t, 1, updatedRun.PassedTests)
		assert.Equal(t, 1, updatedRun.FailedTests)
		assert.Equal(t, 1, updatedRun.SkippedTests)
	})

	t.Run("CompleteRun", func(t *testing.T) {
		// Start the run first (set started_at)
		now := time.Now()
		run.StartedAt = &now
		err := repos.Runs.Update(ctx, run)
		require.NoError(t, err)

		err = collector.CompleteRun(ctx, run.ID, database.RunStatusFailed, "")
		require.NoError(t, err)

		// Verify run was completed
		completedRun, err := repos.Runs.Get(ctx, run.ID)
		require.NoError(t, err)

		assert.Equal(t, database.RunStatusFailed, completedRun.Status)
		assert.NotNil(t, completedRun.FinishedAt)
		assert.NotNil(t, completedRun.DurationMs)
	})
}

// ============================================================================
// ARTIFACT STORAGE INTEGRATION TESTS
// ============================================================================

func TestArtifactStorage(t *testing.T) {
	if testInfra.storage == nil {
		t.Skip("Storage not available")
	}

	ctx := context.Background()
	runID := uuid.New()

	t.Run("UploadAndDownload", func(t *testing.T) {
		content := []byte("test artifact content")
		reader := bytes.NewReader(content)

		// Upload
		path, err := testInfra.storage.Upload(ctx, runID, "test.txt", reader)
		require.NoError(t, err)
		assert.Contains(t, path, runID.String())

		// Download
		downloadReader, err := testInfra.storage.Download(ctx, path)
		require.NoError(t, err)
		defer downloadReader.Close()

		downloaded, err := io.ReadAll(downloadReader)
		require.NoError(t, err)
		assert.Equal(t, content, downloaded)

		// Cleanup
		err = testInfra.storage.Delete(ctx, path)
		require.NoError(t, err)
	})

	t.Run("GetMetadata", func(t *testing.T) {
		content := []byte("metadata test content")
		path, err := testInfra.storage.UploadWithSize(ctx, runID, "metadata.json", bytes.NewReader(content), int64(len(content)))
		require.NoError(t, err)
		defer testInfra.storage.Delete(ctx, path)

		metadata, err := testInfra.storage.GetMetadata(ctx, path)
		require.NoError(t, err)

		assert.Equal(t, path, metadata.Path)
		assert.Equal(t, "metadata.json", metadata.Name)
		assert.Equal(t, int64(len(content)), metadata.Size)
		assert.Equal(t, "application/json", metadata.ContentType)
	})

	t.Run("GetPresignedURL", func(t *testing.T) {
		content := []byte("presigned test")
		path, err := testInfra.storage.Upload(ctx, runID, "presigned.txt", bytes.NewReader(content))
		require.NoError(t, err)
		defer testInfra.storage.Delete(ctx, path)

		url, err := testInfra.storage.GetPresignedURL(ctx, path, time.Hour)
		require.NoError(t, err)
		assert.Contains(t, url, path)
		assert.Contains(t, url, "X-Amz-Signature")
	})

	t.Run("List", func(t *testing.T) {
		listRunID := uuid.New()

		// Upload multiple files
		for i := 0; i < 3; i++ {
			content := []byte("file content")
			name := "file" + string(rune('A'+i)) + ".txt"
			_, err := testInfra.storage.Upload(ctx, listRunID, name, bytes.NewReader(content))
			require.NoError(t, err)
		}

		// List
		artifacts, err := testInfra.storage.List(ctx, listRunID)
		require.NoError(t, err)
		assert.Len(t, artifacts, 3)

		// Cleanup
		err = testInfra.storage.DeleteByRun(ctx, listRunID)
		require.NoError(t, err)
	})

	t.Run("DeleteByRun", func(t *testing.T) {
		deleteRunID := uuid.New()

		// Upload files
		for i := 0; i < 3; i++ {
			_, err := testInfra.storage.Upload(ctx, deleteRunID, "todelete"+string(rune('0'+i))+".txt", bytes.NewReader([]byte("delete me")))
			require.NoError(t, err)
		}

		// Delete all
		err := testInfra.storage.DeleteByRun(ctx, deleteRunID)
		require.NoError(t, err)

		// Verify empty
		artifacts, err := testInfra.storage.List(ctx, deleteRunID)
		require.NoError(t, err)
		assert.Empty(t, artifacts)
	})

	t.Run("ContentTypeDetection", func(t *testing.T) {
		testCases := []struct {
			name     string
			expected string
		}{
			{"report.html", "text/html"},
			{"screenshot.png", "image/png"},
			{"data.json", "application/json"},
			{"log.txt", "text/plain"},
			{"video.mp4", "video/mp4"},
			{"archive.zip", "application/zip"},
			{"unknown.xyz", "application/octet-stream"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				path, err := testInfra.storage.Upload(ctx, runID, tc.name, bytes.NewReader([]byte("content")))
				require.NoError(t, err)
				defer testInfra.storage.Delete(ctx, path)

				metadata, err := testInfra.storage.GetMetadata(ctx, path)
				require.NoError(t, err)
				assert.Equal(t, tc.expected, metadata.ContentType)
			})
		}
	})
}

// ============================================================================
// COLLECTOR WITH ARTIFACT TESTS
// ============================================================================

func TestCollectorWithArtifacts(t *testing.T) {
	if testInfra.db == nil || testInfra.storage == nil {
		t.Skip("Infrastructure not available")
	}

	ctx := context.Background()
	repos := database.NewRepositories(testInfra.db)

	// Create test service
	svc := &database.Service{
		Name:          "artifact-test-service-" + uuid.New().String()[:8],
		GitURL:        "https://github.com/example/repo.git",
		DefaultBranch: "main",
	}
	err := repos.Services.Create(ctx, svc)
	require.NoError(t, err)
	defer repos.Services.Delete(ctx, svc.ID)

	// Create test run
	run := &database.TestRun{
		ServiceID: svc.ID,
		Status:    database.RunStatusRunning,
	}
	err = repos.Runs.Create(ctx, run)
	require.NoError(t, err)

	collector := NewCollector(
		repos.Runs,
		repos.Results,
		repos.Artifacts,
		testInfra.storage,
		nil,
	)

	t.Run("ProcessArtifact", func(t *testing.T) {
		// Upload artifact to storage first
		content := []byte("test artifact data")
		path, err := testInfra.storage.Upload(ctx, run.ID, "test-artifact.txt", bytes.NewReader(content))
		require.NoError(t, err)

		// Record artifact
		artifactInfo := ArtifactInfo{
			Name:        "test-artifact.txt",
			Path:        path,
			ContentType: "text/plain",
			SizeBytes:   int64(len(content)),
		}

		err = collector.ProcessArtifact(ctx, run.ID, artifactInfo)
		require.NoError(t, err)

		// Verify artifact was recorded
		artifacts, err := collector.GetRunArtifacts(ctx, run.ID)
		require.NoError(t, err)
		assert.Len(t, artifacts, 1)
		assert.Equal(t, "test-artifact.txt", artifacts[0].Name)
		assert.Equal(t, path, artifacts[0].Path)
	})
}

// ============================================================================
// DETERMINE RUN STATUS TESTS
// ============================================================================

func TestDetermineRunStatus(t *testing.T) {
	testCases := []struct {
		name     string
		passed   int
		failed   int
		total    int
		hasError bool
		expected database.RunStatus
	}{
		{"AllPassed", 10, 0, 10, false, database.RunStatusPassed},
		{"SomeFailed", 8, 2, 10, false, database.RunStatusFailed},
		{"AllFailed", 0, 10, 10, false, database.RunStatusFailed},
		{"NoTests", 0, 0, 0, false, database.RunStatusError},
		{"HasError", 5, 0, 5, true, database.RunStatusError},
		{"ErrorTakesPrecedence", 5, 5, 10, true, database.RunStatusError},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			status := DetermineRunStatus(tc.passed, tc.failed, tc.total, tc.hasError)
			assert.Equal(t, tc.expected, status)
		})
	}
}

// ============================================================================
// STREAMING COLLECTOR TESTS
// ============================================================================

func TestStreamingCollector(t *testing.T) {
	if testInfra.db == nil {
		t.Skip("Database not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repos := database.NewRepositories(testInfra.db)

	// Create test service
	svc := &database.Service{
		Name:          "streaming-test-service-" + uuid.New().String()[:8],
		GitURL:        "https://github.com/example/repo.git",
		DefaultBranch: "main",
	}
	err := repos.Services.Create(ctx, svc)
	require.NoError(t, err)
	defer repos.Services.Delete(ctx, svc.ID)

	// Create test run
	run := &database.TestRun{
		ServiceID: svc.ID,
		Status:    database.RunStatusRunning,
	}
	err = repos.Runs.Create(ctx, run)
	require.NoError(t, err)

	collector := NewStreamingCollector(
		repos.Runs,
		repos.Results,
		repos.Artifacts,
		testInfra.storage,
		nil,
		10, // Small buffer for testing
	)

	t.Run("ProcessStream", func(t *testing.T) {
		ch := NewResultChannel(10)

		// Start processing in goroutine
		errCh := make(chan error, 1)
		go func() {
			errCh <- collector.ProcessStream(ctx, run.ID, ch)
		}()

		// Send results
		for i := 0; i < 15; i++ {
			ch.Results <- TestResult{
				TestName:   "StreamTest" + string(rune('A'+i%26)),
				Status:     "pass",
				DurationMs: int64(i * 10),
			}
		}

		// Signal done
		close(ch.Results)
		close(ch.Done)

		// Wait for processing
		err := <-errCh
		require.NoError(t, err)

		// Verify results
		results, err := repos.Results.ListByRun(ctx, run.ID)
		require.NoError(t, err)
		assert.Len(t, results, 15)
	})
}
