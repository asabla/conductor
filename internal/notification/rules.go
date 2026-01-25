package notification

import (
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/conductor/conductor/internal/database"
)

// RuleEngine evaluates notification rules against events.
type RuleEngine struct {
	// throttleCache tracks recent notifications for throttling
	throttleCache map[string]time.Time
	throttleMu    sync.RWMutex
	// throttleDuration is how long to throttle duplicate notifications
	throttleDuration time.Duration
}

// NewRuleEngine creates a new rule engine.
func NewRuleEngine(throttleDuration time.Duration) *RuleEngine {
	if throttleDuration <= 0 {
		throttleDuration = 5 * time.Minute
	}
	return &RuleEngine{
		throttleCache:    make(map[string]time.Time),
		throttleDuration: throttleDuration,
	}
}

// RuleMatch contains a matched rule with its associated channel.
type RuleMatch struct {
	Rule    *database.NotificationRule
	Channel *database.NotificationChannel
}

// Evaluate evaluates all rules against an event and returns matching rules.
func (e *RuleEngine) Evaluate(rules []database.NotificationRule, channels map[uuid.UUID]*database.NotificationChannel, event *Event) []RuleMatch {
	var matches []RuleMatch

	triggerEvent := mapTriggerEvent(event.Type)

	for i := range rules {
		rule := &rules[i]

		// Skip disabled rules
		if !rule.Enabled {
			continue
		}

		// Check if rule matches the event
		if !e.ruleMatchesEvent(rule, triggerEvent, event) {
			continue
		}

		// Get the channel for this rule
		channel, ok := channels[rule.ChannelID]
		if !ok || !channel.Enabled {
			continue
		}

		// Check throttling
		if e.isThrottled(rule.ID, event) {
			continue
		}

		matches = append(matches, RuleMatch{
			Rule:    rule,
			Channel: channel,
		})
	}

	return matches
}

// ruleMatchesEvent checks if a rule matches the given event.
func (e *RuleEngine) ruleMatchesEvent(rule *database.NotificationRule, triggerEvent database.TriggerEvent, event *Event) bool {
	// Check if the trigger event matches
	triggerMatches := false
	for _, trigger := range rule.TriggerOn {
		if trigger == database.TriggerEventAlways || trigger == triggerEvent {
			triggerMatches = true
			break
		}
	}
	if !triggerMatches {
		return false
	}

	// Check service filter
	// If rule has a service ID, it must match the event's service ID
	// If rule has no service ID (nil), it matches all services (global rule)
	if rule.ServiceID != nil && *rule.ServiceID != event.ServiceID {
		return false
	}

	return true
}

// isThrottled checks if a notification should be throttled.
func (e *RuleEngine) isThrottled(ruleID uuid.UUID, event *Event) bool {
	// Create a unique key for this rule+event combination
	key := e.throttleKey(ruleID, event)

	e.throttleMu.RLock()
	lastSent, exists := e.throttleCache[key]
	e.throttleMu.RUnlock()

	if exists && time.Since(lastSent) < e.throttleDuration {
		return true
	}

	return false
}

// MarkSent marks a notification as sent for throttling purposes.
func (e *RuleEngine) MarkSent(ruleID uuid.UUID, event *Event) {
	key := e.throttleKey(ruleID, event)

	e.throttleMu.Lock()
	e.throttleCache[key] = time.Now()
	e.throttleMu.Unlock()
}

// throttleKey creates a unique key for throttling.
func (e *RuleEngine) throttleKey(ruleID uuid.UUID, event *Event) string {
	// Throttle by rule + service + event type
	return ruleID.String() + ":" + event.ServiceID.String() + ":" + string(event.Type)
}

// CleanupThrottleCache removes expired entries from the throttle cache.
func (e *RuleEngine) CleanupThrottleCache() {
	e.throttleMu.Lock()
	defer e.throttleMu.Unlock()

	now := time.Now()
	for key, lastSent := range e.throttleCache {
		if now.Sub(lastSent) > e.throttleDuration*2 {
			delete(e.throttleCache, key)
		}
	}
}

// ShouldNotifyOnFirstFailure checks if this is the first failure after success.
func ShouldNotifyOnFirstFailure(currentRun, previousRun *database.TestRun) bool {
	if previousRun == nil {
		// No previous run, this is the first failure
		return currentRun.Status == database.RunStatusFailed
	}

	// Notify if current is failed and previous was passed
	return currentRun.Status == database.RunStatusFailed &&
		previousRun.Status == database.RunStatusPassed
}

// IsRecovery checks if the current run represents a recovery from failure.
func IsRecovery(currentRun, previousRun *database.TestRun) bool {
	if previousRun == nil {
		return false
	}

	// Recovery is when current passes and previous failed
	return currentRun.Status == database.RunStatusPassed &&
		(previousRun.Status == database.RunStatusFailed ||
			previousRun.Status == database.RunStatusError ||
			previousRun.Status == database.RunStatusTimeout)
}

// DetermineNotificationType determines the notification type from a run.
func DetermineNotificationType(run *database.TestRun, previousRun *database.TestRun) NotificationType {
	switch run.Status {
	case database.RunStatusPassed:
		if IsRecovery(run, previousRun) {
			return NotificationTypeRunRecovered
		}
		return NotificationTypeRunPassed
	case database.RunStatusFailed:
		return NotificationTypeRunFailed
	case database.RunStatusError:
		return NotificationTypeRunError
	case database.RunStatusTimeout:
		return NotificationTypeRunTimeout
	case database.RunStatusRunning:
		return NotificationTypeRunStarted
	default:
		return NotificationTypeRunStarted
	}
}
