package rabbit

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// RabbitMQClient представляет клиент RabbitMQ с автоматическим переподключением
type RabbitMQClient struct {
	conn      *amqp.Connection
	amqpURL   string
	mutex     sync.RWMutex
	isClosing bool
	closed    chan struct{}
}

// NewRabbitMQClient создает новый клиент RabbitMQ с автоматическим переподключением
func NewRabbitMQClient(amqpURL string) (*RabbitMQClient, error) {
	client := &RabbitMQClient{
		amqpURL: amqpURL,
		closed:  make(chan struct{}),
	}

	err := client.connect()
	if err != nil {
		return nil, err
	}

	// Запускаем мониторинг соединения
	go client.monitorConnection()

	return client, nil
}

// connect устанавливает соединение с RabbitMQ
func (c *RabbitMQClient) connect() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	conn, err := amqp.Dial(c.amqpURL)
	if err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	c.conn = conn
	log.Println("Подключение к RabbitMQ установлено")
	return nil
}

// reconnect переподключается к RabbitMQ с экспоненциальной задержкой
func (c *RabbitMQClient) reconnect() {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-c.closed:
			return
		default:
		}

		if c.isClosing {
			return
		}

		log.Printf("Попытка переподключения к RabbitMQ через %v...", backoff)
		time.Sleep(backoff)

		err := c.connect()
		if err == nil {
			log.Println("Переподключение к RabbitMQ успешно")
			return
		}

		log.Printf("Ошибка переподключения к RabbitMQ: %v", err)

		// Увеличиваем задержку с экспоненциальным ростом
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// monitorConnection мониторит состояние соединения
func (c *RabbitMQClient) monitorConnection() {
	for {
		select {
		case <-c.closed:
			return
		default:
		}

		c.mutex.RLock()
		conn := c.conn
		c.mutex.RUnlock()

		if conn == nil {
			c.reconnect()
			continue
		}

		// Ждем уведомления о закрытии соединения
		errChan := make(chan *amqp.Error)
		conn.NotifyClose(errChan)

		select {
		case err := <-errChan:
			if err != nil && !c.isClosing {
				log.Printf("Соединение с RabbitMQ потеряно: %v", err)
				c.reconnect()
			}
		case <-c.closed:
			return
		}
	}
}

// getConnection возвращает текущее соединение (thread-safe)
func (c *RabbitMQClient) getConnection() *amqp.Connection {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.conn
}

// PublishProtoJSONWithTTL отправляет proto-сообщение в очередь RabbitMQ с поддержкой TTL и автоматическим переподключением
func (c *RabbitMQClient) PublishProtoJSONWithTTL(ctx context.Context, queue string, msg proto.Message, ttl int) error {
	const maxRetries = 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		conn := c.getConnection()
		if conn == nil {
			if attempt == maxRetries {
				return fmt.Errorf("no connection available after %d attempts", maxRetries)
			}
			time.Sleep(time.Second)
			continue
		}

		err := c.publishOnce(ctx, conn, queue, msg, ttl)
		if err == nil {
			return nil
		}

		// Проверяем, является ли ошибка связанной с соединением
		if isConnectionError(err) {
			log.Printf("Ошибка соединения при отправке (попытка %d/%d): %v", attempt, maxRetries, err)
			if attempt < maxRetries {
				time.Sleep(time.Millisecond * 100)
				continue
			}
		}

		return err
	}

	return fmt.Errorf("failed to publish after %d attempts", maxRetries)
}

// publishOnce выполняет одну попытку отправки сообщения
func (c *RabbitMQClient) publishOnce(ctx context.Context, conn *amqp.Connection, queue string, msg proto.Message, ttl int) error {
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

// isConnectionError проверяет, является ли ошибка связанной с соединением
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}

	// Проверяем типичные ошибки соединения
	switch err.(type) {
	case *amqp.Error:
		amqpErr := err.(*amqp.Error)
		return amqpErr.Code == amqp.ChannelError || amqpErr.Code == amqp.ConnectionForced ||
			amqpErr.Code == amqp.NotAllowed || amqpErr.Code == 504
	default:
		errStr := err.Error()
		return contains(errStr, "connection") || contains(errStr, "channel") ||
			contains(errStr, "closed") || contains(errStr, "not open")
	}
}

// contains проверяет, содержит ли строка подстроку (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > len(substr) &&
				(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
					containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Close закрывает клиент RabbitMQ
func (c *RabbitMQClient) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.isClosing = true
	close(c.closed)

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// PublishProtoJSONWithTTL отправляет proto-сообщение в очередь RabbitMQ с поддержкой TTL (в миллисекундах, 0 — бессрочно)
// Оставлено для обратной совместимости
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
