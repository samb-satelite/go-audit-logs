# Audit Log Library

## Description
The Audit Log Library is a Go package that utilizes RabbitMQ to manage audit logs. This library provides functionalities for publishing and consuming audit logs in an efficient manner.

## Prerequisites
- Go 1.16 or newer
- RabbitMQ
- Package `github.com/joho/godotenv`
- Package `github.com/streadway/amqp`

## Installation
1. **Install dependencies**:
   ```bash
   go get github.com/samb-satelite/go-audit-logs
   ```

2. **Create a `.env` file** in the root directory with the following content:
   ```
   RABBITMQ_URL=amqp://user:password@localhost:5672/
   ```

   Replace `user`, `password`, and `localhost` with your RabbitMQ configuration.

## Usage

### 1. Publish Audit Log

Create a file named `publish.go` with the following content:

```go
package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/joho/godotenv"
	auditlogs "github.com/samb-satelite/go-audit-logs"
)

type User struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

func main() {
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: Could not load .env file, using environment variables from the host")
	}

	// Create a new AuditLog client
	client, err := auditlogs.NewAuditLogClient()
	if err != nil {
		log.Fatalf("Failed to create audit log client: %v", err)
	}
	defer client.Close()

	userBefore := User{
		Email: "user@example.com",
		Name:  "Jane Doe",
	}
	userAfter := User{
		Email: "user@example.com",
		Name:  "John Doe",
	}

	// Convert objek to JSON string
	beforeJSON, _ := json.Marshal(userBefore)
	afterJSON, _ := json.Marshal(userAfter)

	// Publish some sample audit logs
	for i := 1; i <= 5; i++ {
		auditLog := auditlogs.AuditLog{
			Module:     "UserModule",
			ActionType: "CREATE",
			SearchKey:  string(i),
			Before:     string(beforeJSON),
			After:      string(afterJSON),
			ActionBy:   "admin",
		}

		if err := client.PublishAuditLog(auditLog); err != nil {
			log.Printf("Failed to publish audit log: %v", err)
		} else {
			fmt.Printf("Published audit log: %+v\n", auditLog)
		}
	}

	fmt.Println("Finished publishing audit logs.")
}
```

Run the following command to publish the audit log:
```bash
go run publish.go
```

### 2. Consume Audit Log

Create a file named `consume.go` with the following content:

```go
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	auditlogs "github.com/samb-satelite/go-audit-logs"
)

// AuditLogModel defines the structure for GORM
type AuditLogModel struct {
	gorm.Model
	Module      string `json:"module"`
	ActionType  string `json:"actionType"`
	SearchKey   string `json:"searchKey"`
	Before      string `json:"before"` // Store as JSON string
	After       string `json:"after"`  // Store as JSON string
	ActionBy    string `json:"actionBy"`
	ActionTime  string `json:"timestamp"`    // Store as formatted time string
	CreatedTime string `json:"created_time"` // New column to store creation time
}

func main() {
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: Could not load .env file, using environment variables from the host")
	}

	// Initialize database
	db, err := gorm.Open(sqlite.Open("auditlogs.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to the database: %v", err)
	}

	// Migrate the schema
	if err := db.AutoMigrate(&AuditLogModel{}); err != nil {
		log.Fatalf("Failed to migrate database schema: %v", err)
	}

	client, err := auditlogs.NewAuditLogClient()
	if err != nil {
		log.Fatalf("Failed to create audit log client: %v", err)
	}
	defer client.Close()

	err = client.ConsumeAuditLogs(func(log auditlogs.AuditLog) {

		// Format ActionTime as a string
		actionTime := log.ActionTime.Format("2006-01-02 15:04:05")

		// Set CreatedTime to the current time
		createdTime := time.Now().Format("2006-01-02 15:04:05")

		// Save to the database
		auditLogModel := AuditLogModel{
			Module:      log.Module,
			ActionType:  log.ActionType,
			SearchKey:   log.SearchKey,
			Before:      log.Before,
			After:       log.After,
			ActionBy:    log.ActionBy,
			ActionTime:  actionTime,
			CreatedTime: createdTime,
		}

		if err := db.Create(&auditLogModel).Error; err != nil {
			fmt.Printf("Failed to save audit log to the database: %v\n", err)
			ack(false) // Return the message back to the queue
			return
		} else {
			fmt.Printf("Saved audit log to the database: %+v\n", auditLogModel)
			ack(true) // Ack if successful
		}
	})

	if err != nil {
		log.Fatalf("Failed to start consuming audit logs: %v", err)
	}

	// Keep the application running
	select {}
}
```

Run the following command to consume audit logs:
```bash
go run consume.go
```

## Notes
- Make sure RabbitMQ is running when you execute this application.
- You can publish as many audit logs as needed, and the consumer will receive those messages.