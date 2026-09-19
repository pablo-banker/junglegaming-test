package sqs

import (
	"context"
	"errors"
	"testing"
)

// TestPublisherSendValidatesInput verifies invalid FIFO message inputs are rejected.
func TestPublisherSendValidatesInput(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		groupID string
		dedupID string
		wantErr error
	}{
		{
			name:    "empty body",
			body:    "",
			groupID: "wallet-1",
			dedupID: "message-1",
			wantErr: ErrInvalidMessageBody,
		},
		{
			name:    "blank body",
			body:    "   ",
			groupID: "wallet-1",
			dedupID: "message-1",
			wantErr: ErrInvalidMessageBody,
		},
		{
			name:    "empty group id",
			body:    `{"type":"BET"}`,
			groupID: "",
			dedupID: "message-1",
			wantErr: ErrInvalidGroupID,
		},
		{
			name:    "empty deduplication id",
			body:    `{"type":"BET"}`,
			groupID: "wallet-1",
			dedupID: "",
			wantErr: ErrInvalidDedupID,
		},
	}

	publisher := &Publisher{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := publisher.Send(context.Background(), tt.body, tt.groupID, tt.dedupID)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}
