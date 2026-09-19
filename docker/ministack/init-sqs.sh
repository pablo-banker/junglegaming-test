#!/bin/bash

set -e

REGION="us-east-1"
ENDPOINT="http://localhost:4566"

DLQ_NAME="wager-transactions-dlq.fifo"
QUEUE_NAME="wager-transactions.fifo"

DLQ_URL=$(aws \
  --endpoint-url="$ENDPOINT" \
  --region "$REGION" \
  sqs create-queue \
  --queue-name "$DLQ_NAME" \
  --attributes FifoQueue=true \
  --query QueueUrl \
  --output text)

DLQ_ARN=$(aws \
  --endpoint-url="$ENDPOINT" \
  --region "$REGION" \
  sqs get-queue-attributes \
  --queue-url "$DLQ_URL" \
  --attribute-names QueueArn \
  --query 'Attributes.QueueArn' \
  --output text)

QUEUE_URL=$(aws \
  --endpoint-url="$ENDPOINT" \
  --region "$REGION" \
  sqs create-queue \
  --queue-name "$QUEUE_NAME" \
  --attributes FifoQueue=true \
  --query QueueUrl \
  --output text)

cat > /tmp/redrive-policy.json <<EOF
{
  "RedrivePolicy": "{\"deadLetterTargetArn\":\"${DLQ_ARN}\",\"maxReceiveCount\":\"5\"}"
}
EOF

aws \
  --endpoint-url="$ENDPOINT" \
  --region "$REGION" \
  sqs set-queue-attributes \
  --queue-url "$QUEUE_URL" \
  --attributes file:///tmp/redrive-policy.json