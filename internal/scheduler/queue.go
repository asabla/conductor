package scheduler

import (
	"container/heap"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/conductor/conductor/internal/database"
)

// WorkItem represents a pending test run in the queue.
type WorkItem struct {
	RunID        uuid.UUID
	ServiceID    uuid.UUID
	Priority     int
	NetworkZones []string
	CreatedAt    time.Time
	index        int // Used by heap.Interface
}

// Queue manages pending work items with priority ordering.
// Items are ordered by priority (higher first), then by creation time (older first).
type Queue struct {
	mu      sync.RWMutex
	items   priorityQueue
	byRunID map[uuid.UUID]*WorkItem
	runRepo database.TestRunRepository
}

// NewQueue creates a new work queue.
func NewQueue(runRepo database.TestRunRepository) *Queue {
	return &Queue{
		items:   make(priorityQueue, 0),
		byRunID: make(map[uuid.UUID]*WorkItem),
		runRepo: runRepo,
	}
}

// Push adds a work item to the queue.
func (q *Queue) Push(ctx context.Context, item *WorkItem) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check for duplicates
	if _, exists := q.byRunID[item.RunID]; exists {
		return fmt.Errorf("run %s already in queue", item.RunID)
	}

	heap.Push(&q.items, item)
	q.byRunID[item.RunID] = item

	return nil
}

// Pop removes and returns the highest priority item from the queue.
func (q *Queue) Pop(ctx context.Context) (*WorkItem, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.items.Len() == 0 {
		return nil, nil
	}

	item := heap.Pop(&q.items).(*WorkItem)
	delete(q.byRunID, item.RunID)

	return item, nil
}

// Peek returns the highest priority item without removing it.
func (q *Queue) Peek(ctx context.Context) (*WorkItem, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.items.Len() == 0 {
		return nil, nil
	}

	return q.items[0], nil
}

// PeekBatch returns up to n highest priority items without removing them.
func (q *Queue) PeekBatch(ctx context.Context, n int) ([]*WorkItem, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.items.Len() == 0 {
		return nil, nil
	}

	// We need to return items in priority order
	// Since heap doesn't guarantee order beyond the root, we need to sort
	count := n
	if count > q.items.Len() {
		count = q.items.Len()
	}

	// Create a copy of the queue to extract items in order
	tempQueue := make(priorityQueue, q.items.Len())
	copy(tempQueue, q.items)
	heap.Init(&tempQueue)

	result := make([]*WorkItem, 0, count)
	for i := 0; i < count && tempQueue.Len() > 0; i++ {
		item := heap.Pop(&tempQueue).(*WorkItem)
		result = append(result, item)
	}

	return result, nil
}

// Remove removes a specific item from the queue.
func (q *Queue) Remove(ctx context.Context, runID uuid.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, exists := q.byRunID[runID]
	if !exists {
		return nil // Not in queue, nothing to do
	}

	heap.Remove(&q.items, item.index)
	delete(q.byRunID, runID)

	return nil
}

// Len returns the number of items in the queue.
func (q *Queue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.items.Len()
}

// Contains checks if a run is in the queue.
func (q *Queue) Contains(runID uuid.UUID) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	_, exists := q.byRunID[runID]
	return exists
}

// LoadFromDatabase loads pending runs from the database into the queue.
// This should be called on startup to restore queue state.
func (q *Queue) LoadFromDatabase(ctx context.Context, serviceRepo database.ServiceRepository) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Get all pending runs
	runs, err := q.runRepo.GetPending(ctx, 1000) // Load up to 1000 pending runs
	if err != nil {
		return fmt.Errorf("failed to load pending runs: %w", err)
	}

	// Clear existing queue
	q.items = make(priorityQueue, 0, len(runs))
	q.byRunID = make(map[uuid.UUID]*WorkItem)

	// Add each run to the queue
	for _, run := range runs {
		// Get service to get network zones
		service, err := serviceRepo.Get(ctx, run.ServiceID)
		if err != nil {
			return fmt.Errorf("failed to get service %s: %w", run.ServiceID, err)
		}
		if service == nil {
			continue // Skip orphaned runs
		}

		item := &WorkItem{
			RunID:        run.ID,
			ServiceID:    run.ServiceID,
			Priority:     run.Priority,
			NetworkZones: service.NetworkZones,
			CreatedAt:    run.CreatedAt,
		}

		heap.Push(&q.items, item)
		q.byRunID[item.RunID] = item
	}

	return nil
}

// UpdatePriority updates the priority of an item in the queue.
func (q *Queue) UpdatePriority(ctx context.Context, runID uuid.UUID, newPriority int) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, exists := q.byRunID[runID]
	if !exists {
		return fmt.Errorf("run %s not in queue", runID)
	}

	item.Priority = newPriority
	heap.Fix(&q.items, item.index)

	return nil
}

// GetAll returns all items in the queue (for debugging/monitoring).
func (q *Queue) GetAll() []*WorkItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := make([]*WorkItem, len(q.items))
	copy(result, q.items)
	return result
}

// priorityQueue implements heap.Interface for WorkItem priority queue.
// Higher priority items come first; for equal priority, older items come first.
type priorityQueue []*WorkItem

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	// Higher priority first
	if pq[i].Priority != pq[j].Priority {
		return pq[i].Priority > pq[j].Priority
	}
	// For equal priority, older items first (FIFO within priority level)
	return pq[i].CreatedAt.Before(pq[j].CreatedAt)
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(x any) {
	n := len(*pq)
	item := x.(*WorkItem)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // Avoid memory leak
	item.index = -1 // For safety
	*pq = old[0 : n-1]
	return item
}

// PersistentQueue wraps Queue with database persistence.
// This ensures queue state survives control plane restarts.
type PersistentQueue struct {
	*Queue
	runRepo     database.TestRunRepository
	serviceRepo database.ServiceRepository
}

// NewPersistentQueue creates a queue that persists state to database.
func NewPersistentQueue(
	runRepo database.TestRunRepository,
	serviceRepo database.ServiceRepository,
) *PersistentQueue {
	return &PersistentQueue{
		Queue:       NewQueue(runRepo),
		runRepo:     runRepo,
		serviceRepo: serviceRepo,
	}
}

// Initialize loads queue state from database.
func (pq *PersistentQueue) Initialize(ctx context.Context) error {
	return pq.Queue.LoadFromDatabase(ctx, pq.serviceRepo)
}
