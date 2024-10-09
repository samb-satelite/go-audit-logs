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

type AuditLog struct {
	Module     string                 `json:"module"`
	ActionType string                 `json:"actionType"`
	SearchKey  string                 `json:"searchKey"`
	Before     map[string]interface{} `json:"before"`
	After      map[string]interface{} `json:"after"`
	ActionBy   string                 `json:"actionBy"`
	ActionTime time.Time              `json:"timestamp"`
}

type AuditLogClient struct {
	connection *amqp.Connection
	channel    *amqp.Channel
}

func NewAuditLogClient() (*AuditLogClient, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: Could not load .env file, using environment variables from the host")
	}

	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		return nil, errors.New("RABBITMQ_URL must be set in the environment variables or .env file")
	}

	conn, err := amqp.Dial(rabbitMQURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open a channel: %w", err)
	}

	_, err = ch.QueueDeclare(
		TopicName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare a queue: %w", err)
	}

	return &AuditLogClient{
		connection: conn,
		channel:    ch,
	}, nil
}

func (c *AuditLogClient) PublishAuditLog(log AuditLog) error {
	log.ActionTime = time.Now()
	payload, err := json.Marshal(log)
	if err != nil {
		return fmt.Errorf("failed to marshal audit log: %w", err)
	}

	err = c.channel.Publish(
		"",        // exchange
		TopicName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        payload,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish a message: %w", err)
	}

	return nil
}

func (c *AuditLogClient) ConsumeAuditLogs(handler func(AuditLog)) error {
	msgs, err := c.channel.Consume(
		TopicName, // queue
		"",        // consumer
		true,      // auto-ack
		false,     // exclusive
		false,     // no-local
		false,     // no-wait
		nil,       // args
	)
	if err != nil {
		return fmt.Errorf("failed to register a consumer: %w", err)
	}

	go func() {
		for msg := range msgs {
			var auditLog AuditLog
			err := json.Unmarshal(msg.Body, &auditLog)
			if err != nil {
				log.Printf("Error unmarshaling message: %v", err)
				continue
			}
			handler(auditLog)
		}
	}()

	log.Println("Waiting for audit log messages. To exit press CTRL+C")
	return nil
}

func (c *AuditLogClient) Close() {
	if c.channel != nil {
		c.channel.Close()
	}
	if c.connection != nil {
		c.connection.Close()
	}
}