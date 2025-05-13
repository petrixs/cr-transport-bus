package transport

import (
	"context"
	"errors"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Publisher для отправки сообщений
func PublishProtoJSON(ctx context.Context, conn *amqp.Connection, queue string, msg proto.Message) error {
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

	return ch.PublishWithContext(ctx, "", queue, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
		Timestamp:   time.Now(),
	})
}

// Consumer для получения сообщений
func ConsumeProtoJSON(ctx context.Context, conn *amqp.Connection, queue string, newMsg func() proto.Message, handler func(proto.Message) error) error {
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	msgs, err := ch.Consume(
		queue, "", true, false, false, false, nil,
	)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case d, ok := <-msgs:
			if !ok {
				return errors.New("channel closed")
			}
			msg := newMsg()
			if err := protojson.Unmarshal(d.Body, msg); err != nil {
				continue // можно логировать ошибку
			}
			if err := handler(msg); err != nil {
				continue // можно логировать ошибку
			}
		}
	}
}
