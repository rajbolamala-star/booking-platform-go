package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// NotificationEvent represents an inbound event from the bus.
type NotificationEvent struct {
	Event     string          `json:"event"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// Counters for observability.
var (
	processed uint64
	failed    uint64
)

// processEvent simulates handling a notification event idempotently.
// Real impl would lookup user, send email/SMS, and write to DB.
func processEvent(e NotificationEvent) error {
	log.Printf("processing event=%s ts=%s", e.Event, e.Timestamp)
	// Simulate processing time.
	time.Sleep(50 * time.Millisecond)
	atomic.AddUint64(&processed, 1)
	return nil
}

// receiveHandler accepts inbound events. In production this would be a
// RabbitMQ consumer or SQS poller — exposed here as HTTP for demo.
func receiveHandler(c *gin.Context) {
	var e NotificationEvent
	if err := c.ShouldBindJSON(&e); err != nil {
		atomic.AddUint64(&failed, 1)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := processEvent(e); err != nil {
		atomic.AddUint64(&failed, 1)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "accepted"})
}

func statsHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"processed": atomic.LoadUint64(&processed),
		"failed":    atomic.LoadUint64(&failed),
	})
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "notification"})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}

	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())

	r.GET("/health", healthHandler)
	r.POST("/events", receiveHandler)
	r.GET("/stats", statsHandler)

	log.Printf("notification service listening on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
