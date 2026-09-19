package observability

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics records the operational metrics of the service. A nil *Metrics records nothing,
// which keeps tests free of metric setup.
type Metrics struct {
	wagerOutcomes             *prometheus.CounterVec
	idempotentReplays         *prometheus.CounterVec
	conflicts                 *prometheus.CounterVec
	processingDuration        *prometheus.HistogramVec
	transactionRetries        *prometheus.CounterVec
	sqsMessages               *prometheus.CounterVec
	outboxPublications        *prometheus.CounterVec
	reconciliationDivergences prometheus.Counter
}

// NewMetrics creates and registers the service metrics.
func NewMetrics(registry prometheus.Registerer) *Metrics {
	m := &Metrics{
		wagerOutcomes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "wager_transactions_total",
			Help: "Wager operations by entry point, kind and resulting status.",
		}, []string{"source", "kind", "status"}),
		idempotentReplays: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "wager_idempotent_replays_total",
			Help: "Duplicate operations answered with the persisted result.",
		}, []string{"source"}),
		conflicts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "wager_conflicts_total",
			Help: "Idempotency and external transaction conflicts.",
		}, []string{"reason"}),
		processingDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "wager_processing_duration_seconds",
			Help:    "Time to process a wager operation.",
			Buckets: prometheus.DefBuckets,
		}, []string{"source"}),
		transactionRetries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "db_transaction_retries_total",
			Help: "Transactions retried after a deadlock or serialization failure.",
		}, []string{"sqlstate"}),
		sqsMessages: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "sqs_messages_total",
			Help: "Inbound SQS messages by outcome: processed, retried, dead_lettered or released.",
		}, []string{"result"}),
		outboxPublications: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "outbox_publications_total",
			Help: "Outbox publication attempts by result.",
		}, []string{"result"}),
		reconciliationDivergences: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "reconciliation_divergences_total",
			Help: "Reconciliations whose stored balance differs from the ledger.",
		}),
	}

	registry.MustRegister(
		m.wagerOutcomes,
		m.idempotentReplays,
		m.conflicts,
		m.processingDuration,
		m.transactionRetries,
		m.sqsMessages,
		m.outboxPublications,
		m.reconciliationDivergences,
	)

	return m
}

// ObserveWager records the outcome and latency of a wager operation.
func (m *Metrics) ObserveWager(source string, kind string, status string, replay bool, duration time.Duration) {
	if m == nil {
		return
	}

	m.wagerOutcomes.WithLabelValues(source, kind, status).Inc()
	m.processingDuration.WithLabelValues(source).Observe(duration.Seconds())

	if replay {
		m.idempotentReplays.WithLabelValues(source).Inc()
	}
}

// CountReplay records a duplicate delivery answered without processing it again.
func (m *Metrics) CountReplay(source string) {
	if m != nil {
		m.idempotentReplays.WithLabelValues(source).Inc()
	}
}

// CountConflict records an idempotency or external transaction conflict.
func (m *Metrics) CountConflict(reason string) {
	if m != nil {
		m.conflicts.WithLabelValues(reason).Inc()
	}
}

// CountTransactionRetry records a transaction retried after a concurrency conflict.
func (m *Metrics) CountTransactionRetry(sqlstate string) {
	if m != nil {
		m.transactionRetries.WithLabelValues(sqlstate).Inc()
	}
}

// CountSQSMessage records the outcome of an inbound SQS message.
func (m *Metrics) CountSQSMessage(result string) {
	if m != nil {
		m.sqsMessages.WithLabelValues(result).Inc()
	}
}

// CountOutboxPublication records an outbox publication attempt.
func (m *Metrics) CountOutboxPublication(result string) {
	if m != nil {
		m.outboxPublications.WithLabelValues(result).Inc()
	}
}

// CountReconciliationDivergence records a reconciliation that found a divergence.
func (m *Metrics) CountReconciliationDivergence() {
	if m != nil {
		m.reconciliationDivergences.Inc()
	}
}

// Backlog is durable work still waiting to be done.
type Backlog struct {
	OutboxPending          int64
	OutboxOldestPendingAge time.Duration
	PendingReferences      int64
	DeadLetteredMessages   int64
}

// backlogScrapeTimeout bounds the queries made on every scrape.
const backlogScrapeTimeout = 2 * time.Second

// backlogCollector reads the backlog at scrape time, so every instance reports the shared state.
type backlogCollector struct {
	read func(ctx context.Context) (Backlog, error)

	outboxPending     *prometheus.Desc
	outboxLag         *prometheus.Desc
	pendingReferences *prometheus.Desc
	deadLettered      *prometheus.Desc
}

// RegisterBacklog registers gauges for the outbox delay, pending references and DLQ size.
func RegisterBacklog(registry prometheus.Registerer, read func(ctx context.Context) (Backlog, error)) {
	registry.MustRegister(&backlogCollector{
		read:              read,
		outboxPending:     prometheus.NewDesc("outbox_pending_events", "Outbox events not published yet.", nil, nil),
		outboxLag:         prometheus.NewDesc("outbox_oldest_pending_age_seconds", "Age of the oldest unpublished outbox event.", nil, nil),
		pendingReferences: prometheus.NewDesc("pending_reference_transactions", "Transactions waiting for their reference.", nil, nil),
		deadLettered:      prometheus.NewDesc("sqs_dlq_messages", "Approximate number of messages in the wager DLQ.", nil, nil),
	})
}

// Describe sends the backlog metric descriptions.
func (c *backlogCollector) Describe(descs chan<- *prometheus.Desc) {
	descs <- c.outboxPending
	descs <- c.outboxLag
	descs <- c.pendingReferences
	descs <- c.deadLettered
}

// Collect reads the backlog; a failed read simply omits the gauges from the scrape.
func (c *backlogCollector) Collect(metrics chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), backlogScrapeTimeout)
	defer cancel()

	backlog, err := c.read(ctx)
	if err != nil {
		return
	}

	metrics <- prometheus.MustNewConstMetric(c.outboxPending, prometheus.GaugeValue, float64(backlog.OutboxPending))
	metrics <- prometheus.MustNewConstMetric(c.outboxLag, prometheus.GaugeValue, backlog.OutboxOldestPendingAge.Seconds())
	metrics <- prometheus.MustNewConstMetric(c.pendingReferences, prometheus.GaugeValue, float64(backlog.PendingReferences))
	metrics <- prometheus.MustNewConstMetric(c.deadLettered, prometheus.GaugeValue, float64(backlog.DeadLetteredMessages))
}
