package sqs

import "errors"

var (
	ErrInvalidMessageBody            = errors.New("invalid message body")
	ErrInvalidGroupID                = errors.New("invalid message group id")
	ErrInvalidDedupID                = errors.New("invalid message deduplication id")
	ErrInvalidMessageID              = errors.New("invalid message id")
	ErrInvalidSQSMessageID           = errors.New("invalid SQS message id")
	ErrInvalidSQSBody                = errors.New("invalid SQS message body")
	ErrInvalidReceipt                = errors.New("invalid SQS receipt handle")
	ErrInvalidWagerMessageID         = errors.New("invalid wager message id")
	ErrInvalidWagerMessageType       = errors.New("invalid wager message type")
	ErrInvalidWagerMessageOccurredAt = errors.New("invalid wager message occurred at")
)
