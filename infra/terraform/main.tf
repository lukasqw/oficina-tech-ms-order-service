terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region

  dynamic "endpoints" {
    for_each = var.aws_endpoint_url == "" ? [] : [var.aws_endpoint_url]
    content {
      sqs      = endpoints.value
      sns      = endpoints.value
      dynamodb = endpoints.value
    }
  }

  skip_credentials_validation = var.aws_endpoint_url != ""
  skip_metadata_api_check     = var.aws_endpoint_url != ""
  skip_requesting_account_id  = var.aws_endpoint_url != ""
}

variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "aws_endpoint_url" {
  type    = string
  default = ""
}

locals {
  queues = {
    customer_deleted = {
      name = "customer-deleted"
    }
    order_inventory_op_requested = {
      name = "order-inventory-op-requested"
    }
    order_inventory_op_succeeded = {
      name = "order-inventory-op-succeeded"
    }
    order_inventory_op_failed = {
      name = "order-inventory-op-failed"
    }
  }
}

resource "aws_sqs_queue" "dlq" {
  for_each = local.queues

  name                      = "${each.value.name}-dlq"
  message_retention_seconds = 1209600
}

resource "aws_sqs_queue" "main" {
  for_each = local.queues

  name                       = each.value.name
  visibility_timeout_seconds = 30
  message_retention_seconds  = 1209600

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.dlq[each.key].arn
    maxReceiveCount     = 3
  })
}

resource "aws_sns_topic" "inventory_low_alert" {
  name = "inventory-low-alert"
}

resource "aws_dynamodb_table" "order_history" {
  name         = "order_history"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "order_id"
  range_key    = "occurred_at"

  attribute {
    name = "order_id"
    type = "S"
  }

  attribute {
    name = "occurred_at"
    type = "S"
  }
}

data "aws_iam_policy_document" "ms1" {
  statement {
    sid     = "PublishCustomerDeleted"
    effect  = "Allow"
    actions = ["sqs:SendMessage", "sqs:GetQueueUrl", "sqs:GetQueueAttributes"]
    resources = [
      aws_sqs_queue.main["customer_deleted"].arn
    ]
  }
}

data "aws_iam_policy_document" "ms2" {
  statement {
    sid    = "ConsumeCustomerDeleted"
    effect = "Allow"
    actions = [
      "sqs:ReceiveMessage",
      "sqs:DeleteMessage",
      "sqs:ChangeMessageVisibility",
      "sqs:GetQueueUrl",
      "sqs:GetQueueAttributes"
    ]
    resources = [
      aws_sqs_queue.main["customer_deleted"].arn
    ]
  }

  statement {
    sid     = "PublishInventoryRequests"
    effect  = "Allow"
    actions = ["sqs:SendMessage", "sqs:GetQueueUrl", "sqs:GetQueueAttributes"]
    resources = [
      aws_sqs_queue.main["order_inventory_op_requested"].arn
    ]
  }

  statement {
    sid    = "ConsumeInventoryResults"
    effect = "Allow"
    actions = [
      "sqs:ReceiveMessage",
      "sqs:DeleteMessage",
      "sqs:ChangeMessageVisibility",
      "sqs:GetQueueUrl",
      "sqs:GetQueueAttributes"
    ]
    resources = [
      aws_sqs_queue.main["order_inventory_op_succeeded"].arn,
      aws_sqs_queue.main["order_inventory_op_failed"].arn
    ]
  }

  statement {
    sid    = "OrderHistoryDynamoDB"
    effect = "Allow"
    actions = [
      "dynamodb:PutItem",
      "dynamodb:Query",
      "dynamodb:Scan",
      "dynamodb:DescribeTable"
    ]
    resources = [
      aws_dynamodb_table.order_history.arn
    ]
  }
}

data "aws_iam_policy_document" "ms3" {
  statement {
    sid    = "ConsumeInventoryRequests"
    effect = "Allow"
    actions = [
      "sqs:ReceiveMessage",
      "sqs:DeleteMessage",
      "sqs:ChangeMessageVisibility",
      "sqs:GetQueueUrl",
      "sqs:GetQueueAttributes"
    ]
    resources = [
      aws_sqs_queue.main["order_inventory_op_requested"].arn
    ]
  }

  statement {
    sid     = "PublishInventoryResults"
    effect  = "Allow"
    actions = ["sqs:SendMessage", "sqs:GetQueueUrl", "sqs:GetQueueAttributes"]
    resources = [
      aws_sqs_queue.main["order_inventory_op_succeeded"].arn,
      aws_sqs_queue.main["order_inventory_op_failed"].arn
    ]
  }

  statement {
    sid     = "PublishInventoryLowAlert"
    effect  = "Allow"
    actions = ["sns:Publish"]
    resources = [
      aws_sns_topic.inventory_low_alert.arn
    ]
  }
}

resource "aws_iam_policy" "ms1_messaging" {
  name   = "oficina-tech-ms1-messaging"
  policy = data.aws_iam_policy_document.ms1.json
}

resource "aws_iam_policy" "ms2_messaging" {
  name   = "oficina-tech-ms2-messaging"
  policy = data.aws_iam_policy_document.ms2.json
}

resource "aws_iam_policy" "ms3_messaging" {
  name   = "oficina-tech-ms3-messaging"
  policy = data.aws_iam_policy_document.ms3.json
}

output "sqs_queue_urls" {
  value = {
    for key, queue in aws_sqs_queue.main : key => queue.url
  }
}

output "sqs_dlq_urls" {
  value = {
    for key, queue in aws_sqs_queue.dlq : key => queue.url
  }
}

output "inventory_low_alert_topic_arn" {
  value = aws_sns_topic.inventory_low_alert.arn
}

output "order_history_table_name" {
  value = aws_dynamodb_table.order_history.name
}
