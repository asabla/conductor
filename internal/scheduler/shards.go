package scheduler

import (
	"context"
	"fmt"

	"github.com/conductor/conductor/internal/database"
)

func ensureShards(ctx context.Context, run *database.TestRun, tests []database.TestDefinition, shardRepo database.RunShardRepository) ([]database.RunShard, [][]database.TestDefinition, error) {
	if shardRepo == nil {
		return nil, nil, fmt.Errorf("shard repository not configured")
	}

	shardCount := run.ShardCount
	if shardCount <= 0 {
		shardCount = 1
	}

	shardTests := splitTests(tests, shardCount)

	shards, err := shardRepo.ListByRun(ctx, run.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list shards: %w", err)
	}
	if len(shards) > 0 {
		return shards, shardTests, nil
	}

	shards = make([]database.RunShard, 0, shardCount)
	for i := 0; i < shardCount; i++ {
		shard := database.RunShard{
			RunID:      run.ID,
			ShardIndex: i,
			ShardCount: shardCount,
			Status:     database.ShardStatusPending,
			TotalTests: len(shardTests[i]),
		}
		if err := shardRepo.Create(ctx, &shard); err != nil {
			return nil, nil, fmt.Errorf("failed to create shard %d: %w", i, err)
		}
		shards = append(shards, shard)
	}

	return shards, shardTests, nil
}

func splitTests(tests []database.TestDefinition, shardCount int) [][]database.TestDefinition {
	if shardCount <= 0 {
		shardCount = 1
	}

	shards := make([][]database.TestDefinition, shardCount)
	for i, test := range tests {
		index := i % shardCount
		shards[index] = append(shards[index], test)
	}
	return shards
}

func nextPendingShard(shards []database.RunShard, shardTests [][]database.TestDefinition) (*database.RunShard, []database.TestDefinition) {
	for i := range shards {
		if shards[i].Status == database.ShardStatusPending {
			var tests []database.TestDefinition
			if shards[i].ShardIndex >= 0 && shards[i].ShardIndex < len(shardTests) {
				tests = shardTests[shards[i].ShardIndex]
			}
			return &shards[i], tests
		}
	}
	return nil, nil
}

func hasPendingShards(shards []database.RunShard) bool {
	for _, shard := range shards {
		if shard.Status == database.ShardStatusPending {
			return true
		}
	}
	return false
}
