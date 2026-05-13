#!/bin/sh
set -eu

# SQS queues used by the saga between MS2 and MS3
awslocal sqs create-queue --queue-name order-inventory-op-requested
awslocal sqs create-queue --queue-name order-inventory-op-succeeded
awslocal sqs create-queue --queue-name order-inventory-op-failed

# Customer deletion event published by MS1
awslocal sqs create-queue --queue-name customer-deleted

# SNS topic for inventory low alerts published by MS3
awslocal sns create-topic --name inventory-low-alert

# DynamoDB table for service order history snapshots
awslocal dynamodb create-table \
  --table-name order_history \
  --attribute-definitions AttributeName=order_id,AttributeType=S AttributeName=occurred_at,AttributeType=S \
  --key-schema AttributeName=order_id,KeyType=HASH AttributeName=occurred_at,KeyType=RANGE \
  --billing-mode PAY_PER_REQUEST
