package rabbit

import (
	"context"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// PublishProtoJSONWithTTL отправляет proto-сообщение в очередь RabbitMQ с поддержкой TTL (в миллисекундах, 0 — бессрочно)
func PublishProtoJSONWithTTL(ctx context.Context, conn *amqp.Connection, queue string, msg proto.Message, ttl int) error {
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	body, err := protojson.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = ch.QueueDeclare(
		queue, true, false, false, false, nil,
	)
	if err != nil {
		return err
	}

	publishing := amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
		Timestamp:   time.Now(),
	}
	if ttl > 0 {
		publishing.Expiration = fmt.Sprintf("%d", ttl)
	}

	return ch.PublishWithContext(ctx, "", queue, false, false, publishing)
}
