// Package artifact provides artifact storage services using S3/MinIO.
package artifact

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ArtifactStorage defines the interface for artifact storage operations.
type ArtifactStorage interface {
	// Upload uploads an artifact and returns the storage path.
	Upload(ctx context.Context, runID uuid.UUID, name string, reader io.Reader) (string, error)

	// Download retrieves an artifact by its storage path.
	Download(ctx context.Context, path string) (io.ReadCloser, error)

	// GetPresignedURL generates a presigned URL for downloading an artifact.
	GetPresignedURL(ctx context.Context, path string, expires time.Duration) (string, error)

	// Delete deletes an artifact from storage.
	Delete(ctx context.Context, path string) error

	// DeleteByRun deletes all artifacts for a test run.
	DeleteByRun(ctx context.Context, runID uuid.UUID) error

	// GetMetadata retrieves metadata for an artifact.
	GetMetadata(ctx context.Context, path string) (*ArtifactMetadata, error)

	// List lists artifacts for a test run.
	List(ctx context.Context, runID uuid.UUID) ([]ArtifactMetadata, error)
}

// ArtifactMetadata contains metadata about a stored artifact.
type ArtifactMetadata struct {
	Path         string
	Name         string
	Size         int64
	ContentType  string
	LastModified time.Time
	ETag         string
}

// StorageConfig holds configuration for artifact storage.
type StorageConfig struct {
	Endpoint        string
	Bucket          string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
	PathStyle       bool
}

// Storage implements the ArtifactStorage interface using MinIO/S3.
type Storage struct {
	client     *minio.Client
	bucket     string
	logger     *slog.Logger
	pathPrefix string
}

// NewStorage creates a new artifact Storage instance.
func NewStorage(cfg StorageConfig, logger *slog.Logger) (*Storage, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Parse endpoint
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "s3.amazonaws.com"
	}

	// Remove protocol prefix if present
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	// Create MinIO client
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	storage := &Storage{
		client:     client,
		bucket:     cfg.Bucket,
		logger:     logger.With("component", "artifact_storage"),
		pathPrefix: "artifacts",
	}

	return storage, nil
}

// EnsureBucket creates the bucket if it doesn't exist.
func (s *Storage) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}

	if !exists {
		err = s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{})
		if err != nil {
			return fmt.Errorf("failed to create bucket: %w", err)
		}
		s.logger.Info("created bucket", "bucket", s.bucket)
	}

	return nil
}

// Upload uploads an artifact and returns the storage path.
func (s *Storage) Upload(ctx context.Context, runID uuid.UUID, name string, reader io.Reader) (string, error) {
	// Generate storage path: artifacts/{run_id}/{name}
	objectPath := s.objectPath(runID, name)

	// Detect content type from extension
	contentType := detectContentType(name)

	s.logger.Debug("uploading artifact",
		"run_id", runID,
		"name", name,
		"path", objectPath,
		"content_type", contentType,
	)

	// Upload the object
	info, err := s.client.PutObject(ctx, s.bucket, objectPath, reader, -1, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload artifact: %w", err)
	}

	s.logger.Info("uploaded artifact",
		"run_id", runID,
		"name", name,
		"path", objectPath,
		"size", info.Size,
	)

	return objectPath, nil
}

// UploadWithSize uploads an artifact with a known size.
func (s *Storage) UploadWithSize(ctx context.Context, runID uuid.UUID, name string, reader io.Reader, size int64) (string, error) {
	objectPath := s.objectPath(runID, name)
	contentType := detectContentType(name)

	info, err := s.client.PutObject(ctx, s.bucket, objectPath, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload artifact: %w", err)
	}

	s.logger.Info("uploaded artifact",
		"run_id", runID,
		"name", name,
		"path", objectPath,
		"size", info.Size,
	)

	return objectPath, nil
}

// Download retrieves an artifact by its storage path.
func (s *Storage) Download(ctx context.Context, objectPath string) (io.ReadCloser, error) {
	s.logger.Debug("downloading artifact", "path", objectPath)

	obj, err := s.client.GetObject(ctx, s.bucket, objectPath, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}

	// Check if object exists by getting stat
	_, err = obj.Stat()
	if err != nil {
		obj.Close()
		return nil, fmt.Errorf("artifact not found: %w", err)
	}

	return obj, nil
}

// GetPresignedURL generates a presigned URL for downloading an artifact.
func (s *Storage) GetPresignedURL(ctx context.Context, objectPath string, expires time.Duration) (string, error) {
	if expires <= 0 {
		expires = 1 * time.Hour // Default expiry
	}
	if expires > 7*24*time.Hour {
		expires = 7 * 24 * time.Hour // Max 7 days
	}

	s.logger.Debug("generating presigned URL",
		"path", objectPath,
		"expires", expires,
	)

	reqParams := make(url.Values)
	presignedURL, err := s.client.PresignedGetObject(ctx, s.bucket, objectPath, expires, reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignedURL.String(), nil
}

// Delete deletes an artifact from storage.
func (s *Storage) Delete(ctx context.Context, objectPath string) error {
	s.logger.Debug("deleting artifact", "path", objectPath)

	err := s.client.RemoveObject(ctx, s.bucket, objectPath, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete artifact: %w", err)
	}

	s.logger.Info("deleted artifact", "path", objectPath)
	return nil
}

// DeleteByRun deletes all artifacts for a test run.
func (s *Storage) DeleteByRun(ctx context.Context, runID uuid.UUID) error {
	prefix := fmt.Sprintf("%s/%s/", s.pathPrefix, runID)

	s.logger.Info("deleting artifacts for run", "run_id", runID, "prefix", prefix)

	// List all objects with the prefix
	objectsCh := s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	// Collect objects to delete
	objectsToDelete := make(chan minio.ObjectInfo)
	go func() {
		defer close(objectsToDelete)
		for object := range objectsCh {
			if object.Err != nil {
				s.logger.Error("error listing object", "error", object.Err)
				continue
			}
			objectsToDelete <- object
		}
	}()

	// Delete objects
	errorCh := s.client.RemoveObjects(ctx, s.bucket, objectsToDelete, minio.RemoveObjectsOptions{})
	var deleteErrors []error
	for err := range errorCh {
		s.logger.Error("error deleting object", "key", err.ObjectName, "error", err.Err)
		deleteErrors = append(deleteErrors, err.Err)
	}

	if len(deleteErrors) > 0 {
		return fmt.Errorf("failed to delete %d artifacts", len(deleteErrors))
	}

	return nil
}

// GetMetadata retrieves metadata for an artifact.
func (s *Storage) GetMetadata(ctx context.Context, objectPath string) (*ArtifactMetadata, error) {
	info, err := s.client.StatObject(ctx, s.bucket, objectPath, minio.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object metadata: %w", err)
	}

	return &ArtifactMetadata{
		Path:         objectPath,
		Name:         path.Base(objectPath),
		Size:         info.Size,
		ContentType:  info.ContentType,
		LastModified: info.LastModified,
		ETag:         info.ETag,
	}, nil
}

// List lists artifacts for a test run.
func (s *Storage) List(ctx context.Context, runID uuid.UUID) ([]ArtifactMetadata, error) {
	prefix := fmt.Sprintf("%s/%s/", s.pathPrefix, runID)

	s.logger.Debug("listing artifacts", "run_id", runID, "prefix", prefix)

	var artifacts []ArtifactMetadata

	objectsCh := s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	for object := range objectsCh {
		if object.Err != nil {
			return nil, fmt.Errorf("error listing objects: %w", object.Err)
		}

		artifacts = append(artifacts, ArtifactMetadata{
			Path:         object.Key,
			Name:         strings.TrimPrefix(object.Key, prefix),
			Size:         object.Size,
			ContentType:  object.ContentType,
			LastModified: object.LastModified,
			ETag:         object.ETag,
		})
	}

	return artifacts, nil
}

// objectPath generates the storage path for an artifact.
func (s *Storage) objectPath(runID uuid.UUID, name string) string {
	// Sanitize the name to prevent path traversal
	name = path.Clean(name)
	name = strings.TrimPrefix(name, "/")
	name = strings.TrimPrefix(name, "../")

	return fmt.Sprintf("%s/%s/%s", s.pathPrefix, runID, name)
}

// detectContentType detects the content type based on file extension.
func detectContentType(name string) string {
	ext := strings.ToLower(path.Ext(name))

	contentTypes := map[string]string{
		".html":  "text/html",
		".htm":   "text/html",
		".css":   "text/css",
		".js":    "application/javascript",
		".json":  "application/json",
		".xml":   "application/xml",
		".txt":   "text/plain",
		".log":   "text/plain",
		".md":    "text/markdown",
		".png":   "image/png",
		".jpg":   "image/jpeg",
		".jpeg":  "image/jpeg",
		".gif":   "image/gif",
		".svg":   "image/svg+xml",
		".webp":  "image/webp",
		".pdf":   "application/pdf",
		".zip":   "application/zip",
		".gz":    "application/gzip",
		".tar":   "application/x-tar",
		".mp4":   "video/mp4",
		".webm":  "video/webm",
		".mp3":   "audio/mpeg",
		".wav":   "audio/wav",
		".csv":   "text/csv",
		".yaml":  "text/yaml",
		".yml":   "text/yaml",
		".toml":  "text/toml",
		".ini":   "text/plain",
		".conf":  "text/plain",
		".sh":    "text/x-shellscript",
		".py":    "text/x-python",
		".go":    "text/x-go",
		".java":  "text/x-java",
		".ts":    "text/typescript",
		".tsx":   "text/typescript",
		".jsx":   "text/javascript",
		".vue":   "text/x-vue",
		".rb":    "text/x-ruby",
		".rs":    "text/x-rust",
		".cpp":   "text/x-c++",
		".c":     "text/x-c",
		".h":     "text/x-c",
		".hpp":   "text/x-c++",
		".cs":    "text/x-csharp",
		".sql":   "text/x-sql",
		".proto": "text/x-protobuf",
	}

	if ct, ok := contentTypes[ext]; ok {
		return ct
	}

	return "application/octet-stream"
}

// HealthCheck checks if the storage backend is reachable.
func (s *Storage) HealthCheck(ctx context.Context) error {
	_, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("storage health check failed: %w", err)
	}
	return nil
}
