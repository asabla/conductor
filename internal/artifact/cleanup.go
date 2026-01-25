package artifact

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/conductor/conductor/internal/database"
)

// CleanupConfig defines retention cleanup settings.
type CleanupConfig struct {
	Interval  time.Duration
	Retention time.Duration
	BatchSize int
}

// CleanupService removes expired artifacts from storage and the database.
type CleanupService struct {
	repo      database.ArtifactRepository
	storage   ArtifactStorage
	logger    *slog.Logger
	interval  time.Duration
	retention time.Duration
	batchSize int
}

// NewCleanupService creates a new CleanupService.
func NewCleanupService(
	repo database.ArtifactRepository,
	storage ArtifactStorage,
	config CleanupConfig,
	logger *slog.Logger,
) *CleanupService {
	if logger == nil {
		logger = slog.Default()
	}

	interval := config.Interval
	if interval <= 0 {
		interval = time.Hour
	}

	retention := config.Retention
	if retention <= 0 {
		retention = 30 * 24 * time.Hour
	}

	batchSize := config.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	return &CleanupService{
		repo:      repo,
		storage:   storage,
		logger:    logger.With("component", "artifact_cleanup"),
		interval:  interval,
		retention: retention,
		batchSize: batchSize,
	}
}

// Start begins the cleanup loop until the context is canceled.
func (s *CleanupService) Start(ctx context.Context) {
	s.logger.Info("starting artifact cleanup",
		"interval", s.interval,
		"retention", s.retention,
		"batch_size", s.batchSize,
	)

	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			s.run(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *CleanupService) run(ctx context.Context) {
	cutoff := time.Now().Add(-s.retention)
	deleted := 0

	for {
		artifacts, err := s.repo.ListOlderThan(ctx, cutoff, s.batchSize)
		if err != nil {
			s.logger.Error("failed to list expired artifacts", "error", err)
			return
		}
		if len(artifacts) == 0 {
			break
		}

		for _, artifact := range artifacts {
			if err := s.deleteArtifact(ctx, artifact); err != nil {
				s.logger.Warn("failed to delete artifact",
					"artifact_id", artifact.ID,
					"path", artifact.Path,
					"error", err,
				)
				continue
			}
			deleted++
		}

		if len(artifacts) < s.batchSize {
			break
		}
	}

	if deleted > 0 {
		s.logger.Info("artifact cleanup completed",
			"deleted", deleted,
			"cutoff", cutoff,
		)
	}
}

func (s *CleanupService) deleteArtifact(ctx context.Context, artifact database.Artifact) error {
	if err := s.storage.Delete(ctx, artifact.Path); err != nil {
		return fmt.Errorf("delete storage: %w", err)
	}

	if err := s.repo.Delete(ctx, artifact.ID); err != nil {
		return fmt.Errorf("delete record: %w", err)
	}

	s.logger.Debug("deleted artifact",
		"artifact_id", artifact.ID,
		"run_id", artifact.RunID,
		"path", artifact.Path,
	)

	return nil
}

// DeleteRunArtifacts removes all artifacts associated with a run.
func (s *CleanupService) DeleteRunArtifacts(ctx context.Context, runID uuid.UUID) error {
	if err := s.storage.DeleteByRun(ctx, runID); err != nil {
		return fmt.Errorf("delete run storage: %w", err)
	}
	if err := s.repo.DeleteByRun(ctx, runID); err != nil {
		return fmt.Errorf("delete run records: %w", err)
	}
	return nil
}
