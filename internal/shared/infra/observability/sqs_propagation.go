package observability

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// InjectTraceToSQS captura o trace context do span atual e retorna os
// MessageAttributes a serem incluídos na mensagem SQS publicada.
// O consumer extrai esses atributos via ExtractSpanLinkFromSQS e cria
// um span linked ao producer, conectando os traces no Datadog APM.
func InjectTraceToSQS(ctx context.Context) map[string]sqstypes.MessageAttributeValue {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)

	attrs := make(map[string]sqstypes.MessageAttributeValue, len(carrier))
	for k, v := range carrier {
		attrs[k] = sqstypes.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(v),
		}
	}
	return attrs
}

// ExtractSpanLinkFromSQS extrai o trace context do producer a partir dos
// MessageAttributes da mensagem SQS e retorna um trace.Link para uso em
// trace.WithLinks(). Retorna false se não há contexto válido na mensagem.
func ExtractSpanLinkFromSQS(msg sqstypes.Message) (trace.Link, bool) {
	carrier := propagation.MapCarrier{}
	for k, v := range msg.MessageAttributes {
		if v.StringValue != nil {
			carrier.Set(k, *v.StringValue)
		}
	}

	remoteCtx := otel.GetTextMapPropagator().Extract(context.Background(), carrier)
	spanCtx := trace.SpanContextFromContext(remoteCtx)
	if !spanCtx.IsValid() {
		return trace.Link{}, false
	}
	return trace.Link{SpanContext: spanCtx}, true
}
