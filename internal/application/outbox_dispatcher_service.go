package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	outboxLeaseDuration     = 30 * time.Second
	initialOutboxRetryDelay = 5 * time.Second
	maxOutboxRetryDelay     = 5 * time.Minute
)

type OutboxDispatcherService struct {
	repository OutboxDispatcherRepository
	publisher  IntegrationEventPublisher
	clock      Clock
	workerID   string
}

// NewOutboxDispatcherService creates the outbox dispatcher application service.
func NewOutboxDispatcherService(
	repository OutboxDispatcherRepository,
	publisher IntegrationEventPublisher,
	clock Clock,
) *OutboxDispatcherService {
	return &OutboxDispatcherService{
		repository: repository,
		publisher:  publisher,
		clock:      clock,
		workerID:   "outbox-" + uuid.NewString(),
	}
}

// DispatchNext publishes one available outbox event.
func (s *OutboxDispatcherService) DispatchNext(
	ctx context.Context,
) (bool, error) {
	now, err := s.clock.Now(ctx)
	if err != nil {
		return false, err
	}

	event, err := s.repository.ClaimPending(
		ctx,
		s.workerID,
		now.Add(outboxLeaseDuration),
	)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}

		return false, err
	}

	if err := s.publisher.Publish(ctx, event); err != nil {
		return true, s.handlePublishFailure(
			ctx,
			event,
			err,
		)
	}

	publishedAt, err := s.clock.Now(ctx)
	if err != nil {
		return true, err
	}

	if err := s.repository.MarkPublished(
		ctx,
		event.EventID,
		s.workerID,
		publishedAt,
	); err != nil {
		return true, err
	}

	return true, nil
}

// handlePublishFailure schedules another delivery attempt after a publication failure.
func (s *OutboxDispatcherService) handlePublishFailure(
	ctx context.Context,
	event *PendingOutboxEvent,
	publishErr error,
) error {
	now, err := s.clock.Now(ctx)
	if err != nil {
		return errors.Join(publishErr, err)
	}

	nextAttemptAt := now.Add(
		outboxRetryDelay(event.Attempts),
	)

	if err := s.repository.ScheduleRetry(
		ctx,
		event.EventID,
		s.workerID,
		nextAttemptAt,
	); err != nil {
		return errors.Join(publishErr, err)
	}

	return publishErr
}

// outboxRetryDelay calculates the exponential publication retry delay.
func outboxRetryDelay(attempts int) time.Duration {
	if attempts <= 1 {
		return initialOutboxRetryDelay
	}

	delay := initialOutboxRetryDelay

	for attempt := 1; attempt < attempts; attempt++ {
		if delay >= maxOutboxRetryDelay/2 {
			return maxOutboxRetryDelay
		}

		delay *= 2
	}

	return delay
}
