package auditlogs

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/streadway/amqp"
)

const (
	TopicName = "audit_logs"
)

// AuditLog represents a single audit log entry.
type AuditLog struct {
	Module     string    `json:"module"`
	ActionType string    `json:"actionType"`
	SearchKey  string    `json:"searchKey"`
	Before     string    `json:"before"`
	After      string    `json:"after"`
	ActionBy   string    `json:"actionBy"`
	ActionTime time.Time `json:"timestamp"`
}

// AuditLogClient manages the connection and channel for RabbitMQ.
type AuditLogClient struct {
	connection *amqp.Connection
	channel    *amqp.Channel
}

// Global instance of AuditLogClient
var auditLogClient *AuditLogClient

// InitAuditLogClient initializes the global AuditLogClient.
func InitAuditLogClient() error {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: Could not load .env file, using environment variables from the host")
	}

	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		return errors.New("RABBITMQ_URL must be set in the environment variables or .env file")
	}

	conn, err := amqp.Dial(rabbitMQURL)
	if err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to open a channel: %w", err)
	}

	if _, err := ch.QueueDeclare(
		TopicName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	); err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("failed to declare a queue: %w", err)
	}

	auditLogClient = &AuditLogClient{
		connection: conn,
		channel:    ch,
	}

	return nil
}

// PublishAuditLog sends an audit log to the RabbitMQ queue.
func (c *AuditLogClient) PublishAuditLog(log AuditLog) error {
	log.ActionTime = time.Now()
	payload, err := json.Marshal(log)
	if err != nil {
		return fmt.Errorf("failed to marshal audit log: %w", err)
	}

	if err := c.channel.Publish(
		"",        // exchange
		TopicName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        payload,
		},
	); err != nil {
		return fmt.Errorf("failed to publish a message: %w", err)
	}

	return nil
}

// ConsumeAuditLogs starts consuming audit logs from the queue.
func (c *AuditLogClient) ConsumeAuditLogs(consumerName *string, handler func(AuditLog, func(bool))) error {
	if consumerName == nil {
		defaultName := "default_consumer"
		consumerName = &defaultName
	}

	msgs, err := c.channel.Consume(
		TopicName,  // queue
		*consumerName, // consumer name
		false,      // auto-ack (set to false for manual ack)
		false,      // exclusive
		false,      // no-local
		false,      // no-wait
		nil,        // args
	)
	if err != nil {
		return fmt.Errorf("failed to register a consumer: %w", err)
	}

	go func() {
		for msg := range msgs {
			var auditLog AuditLog
			if err := json.Unmarshal(msg.Body, &auditLog); err != nil {
				log.Printf("Error unmarshaling message: %v", err)
				continue
			}

			handler(auditLog, func(ack bool) {
				if ack {
					if err := msg.Ack(false); err != nil {
						log.Printf("Failed to acknowledge message: %v", err)
					}
				} else {
					if err := msg.Nack(false, true); err != nil {
						log.Printf("Failed to nack message: %v", err)
					}
				}
			})
		}
	}()

	log.Printf("Consumer %s is waiting for audit log messages. To exit press CTRL+C", *consumerName)
	return nil
}

// Close closes the channel and connection of the AuditLogClient.
func (c *AuditLogClient) Close() {
	if c.channel != nil {
		c.channel.Close()
	}
	if c.connection != nil {
		c.connection.Close()
	}
}

// CloseGlobalClient closes the global audit log client.
func CloseGlobalClient() {
	if auditLogClient != nil {
		auditLogClient.Close()
	}
}
