package multi

import "sync"

// ResultAggregator aggregates outcomes from multiple bot executions concurrently.
type ResultAggregator struct {
	mu      sync.Mutex
	summary Summary
}

// NewResultAggregator creates a ResultAggregator expecting total bot operations.
func NewResultAggregator(total int) *ResultAggregator {
	return &ResultAggregator{
		summary: Summary{
			Total:   total,
			Results: make([]BotResult, 0, total),
		},
	}
}

// AddOK records a successful execution for a bot.
func (ra *ResultAggregator) AddOK(botID string, data any) {
	ra.mu.Lock()
	defer ra.mu.Unlock()
	ra.summary.OK++
	ra.summary.Results = append(ra.summary.Results, BotResult{
		BotID:  botID,
		Status: StatusOK,
		Data:   data,
	})
}

// AddSkipped records a skipped execution for a bot with an explanation.
func (ra *ResultAggregator) AddSkipped(botID string, reason string) {
	ra.mu.Lock()
	defer am(ra.mu.Unlock)
	ra.summary.Skipped++
	ra.summary.Results = append(ra.summary.Results, BotResult{
		BotID:  botID,
		Status: StatusSkipped,
		Reason: reason,
	})
}

// AddFailed records a failed execution for a bot with an explanation.
func (ra *ResultAggregator) AddFailed(botID string, reason string) {
	ra.mu.Lock()
	defer am(ra.mu.Unlock)
	ra.summary.Failed++
	ra.summary.Results = append(ra.summary.Results, BotResult{
		BotID:  botID,
		Status: StatusFailed,
		Reason: reason,
	})
}

func am(fn func()) {
	fn()
}

// Summary returns a snapshot copy of the aggregated summary.
func (ra *ResultAggregator) Summary() Summary {
	ra.mu.Lock()
	defer ra.mu.Unlock()
	resultsCopy := make([]BotResult, len(ra.summary.Results))
	copy(resultsCopy, ra.summary.Results)
	return Summary{
		Total:   ra.summary.Total,
		OK:      ra.summary.OK,
		Skipped: ra.summary.Skipped,
		Failed:  ra.summary.Failed,
		Results: resultsCopy,
	}
}
