#!/bin/bash
#
# Provisions the SQS queues used by the service. Safe to run more than once.
#
#   wager-transactions.fifo          inbound WagerTransactionRequested messages
#   wager-transactions-dlq.fifo      inbound messages that failed permanently or ran out of retries
#   integration-events.fifo          outbound integration events published from the outbox
#   integration-events-dlq.fifo      outbound events that a downstream consumer could not process

set -euo pipefail

REGION="${AWS_DEFAULT_REGION:-us-east-1}"
ENDPOINT="http://localhost:4566"

# Transient failures back off from 5s up to 5min per attempt, so 10 receives
# tolerate roughly 20 minutes of dependency outage before reaching the DLQ.
MAX_RECEIVE_COUNT=10
VISIBILITY_TIMEOUT=60
RECEIVE_WAIT_SECONDS=20
RETENTION_SECONDS=1209600

sqs() {
  aws --endpoint-url="$ENDPOINT" --region "$REGION" sqs "$@"
}

create_fifo_queue() {
  sqs create-queue \
    --queue-name "$1" \
    --attributes FifoQueue=true \
    --query QueueUrl \
    --output text
}

queue_arn() {
  sqs get-queue-attributes \
    --queue-url "$1" \
    --attribute-names QueueArn \
    --query 'Attributes.QueueArn' \
    --output text
}

configure_queue() {
  local queue_url="$1"
  local dlq_arn="$2"
  local attributes

  attributes=$(cat <<EOF
{
  "ContentBasedDeduplication": "false",
  "VisibilityTimeout": "${VISIBILITY_TIMEOUT}",
  "ReceiveMessageWaitTimeSeconds": "${RECEIVE_WAIT_SECONDS}",
  "MessageRetentionPeriod": "${RETENTION_SECONDS}",
  "RedrivePolicy": "{\"deadLetterTargetArn\":\"${dlq_arn}\",\"maxReceiveCount\":\"${MAX_RECEIVE_COUNT}\"}"
}
EOF
)

  sqs set-queue-attributes --queue-url "$queue_url" --attributes "$attributes"
}

provision() {
  local queue_name="$1"
  local dlq_name="$2"

  local dlq_url queue_url

  dlq_url=$(create_fifo_queue "$dlq_name")
  sqs set-queue-attributes \
    --queue-url "$dlq_url" \
    --attributes "MessageRetentionPeriod=${RETENTION_SECONDS}"

  queue_url=$(create_fifo_queue "$queue_name")
  configure_queue "$queue_url" "$(queue_arn "$dlq_url")"

  echo "provisioned ${queue_name} -> ${dlq_name}"
}

provision "wager-transactions.fifo" "wager-transactions-dlq.fifo"
provision "integration-events.fifo" "integration-events-dlq.fifo"
