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
		_ = conn.Close()
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
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("failed to declare a queue: %w", err)
	}

	auditLogClient = &AuditLogClient{
		connection: conn,
		channel:    ch,
	}

	return nil
}

// PublishAuditLog sends an audit log to the RabbitMQ queue.
func PublishAuditLog(log AuditLog) error {
	log.ActionTime = time.Now()
	payload, err := json.Marshal(log)
	if err != nil {
		return fmt.Errorf("failed to marshal audit log: %w", err)
	}

	if err := auditLogClient.channel.Publish(
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
func ConsumeAuditLogs(consumerName *string, handler func(AuditLog, func(bool)), prefetchCount *int) error {
	if consumerName == nil {
		defaultName := "default_consumer"
		consumerName = &defaultName
	}

	// Set the prefetch count based on the value provided
	var effectivePrefetchCount int
	if prefetchCount != nil {
		effectivePrefetchCount = *prefetchCount
	} else {
		effectivePrefetchCount = 50 // Default value
	}

	// Set the QoS with the effective prefetch count
	if err := auditLogClient.channel.Qos(effectivePrefetchCount, 0, false); err != nil {
		return fmt.Errorf("failed to set QoS: %w", err)
	}

	msgs, err := auditLogClient.channel.Consume(
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
func Close() {
	if auditLogClient.channel != nil {
		_ = auditLogClient.channel.Close()
	}
	if auditLogClient.connection != nil {
		_ = auditLogClient.connection.Close()
	}
}

// CloseGlobalClient closes the global audit log client.
func CloseGlobalClient() {
	Close()
}
