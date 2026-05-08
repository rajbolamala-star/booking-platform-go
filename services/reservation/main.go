package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Reservation represents a booking record.
type Reservation struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	PropertyID string    `json:"property_id"`
	CheckIn    time.Time `json:"check_in"`
	CheckOut   time.Time `json:"check_out"`
	Guests     int       `json:"guests"`
	TotalPrice float64   `json:"total_price"`
	Status     string    `json:"status"` // pending | confirmed | cancelled
	CreatedAt  time.Time `json:"created_at"`
}

// CreateReservationRequest is the input contract for booking creation.
type CreateReservationRequest struct {
	UserID     string    `json:"user_id" binding:"required"`
	PropertyID string    `json:"property_id" binding:"required"`
	CheckIn    time.Time `json:"check_in" binding:"required"`
	CheckOut   time.Time `json:"check_out" binding:"required"`
	Guests     int       `json:"guests" binding:"required,min=1"`
}

// In-memory store for demo. In production this would be Postgres.
var (
	store    = make(map[string]Reservation)
	storeMu  sync.RWMutex
	pricing  = getEnv("PRICING_URL", "http://pricing:8082")

	reservationRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reservation_requests_total",
			Help: "Total number of reservation requests",
		},
		[]string{"status"},
	)

	reservationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "reservation_duration_seconds",
			Help:    "Reservation request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"status"},
	)
)

func init() {
	prometheus.MustRegister(reservationRequests, reservationDuration)
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// fetchPriceWithRetry calls pricing service synchronously with retry/backoff.
// Synchronous because pricing must be confirmed before reservation creation.
func fetchPriceWithRetry(req CreateReservationRequest) (float64, error) {
	body, err := json.Marshal(map[string]interface{}{
		"property_id": req.PropertyID,
		"check_in":    req.CheckIn,
		"check_out":   req.CheckOut,
		"guests":      req.Guests,
	})
	if err != nil {
		return 0, err
	}

	maxAttempts := 3
	backoff := 100 * time.Millisecond
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Post(pricing+"/price", "application/json", bytes.NewReader(body))
		if err != nil {
			lastErr = err
			time.Sleep(backoff)
			backoff *= 2
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 500 {
			lastErr = errors.New("pricing service 5xx")
			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		raw, _ := io.ReadAll(resp.Body)
		var p struct {
			TotalPrice float64 `json:"total_price"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return 0, err
		}
		return p.TotalPrice, nil
	}
	return 0, lastErr
}

// publishEvent represents an async publish to message broker.
// In production this writes to RabbitMQ or AWS SNS/SQS.
func publishEvent(eventType string, payload interface{}) {
	out, _ := json.Marshal(map[string]interface{}{
		"event":     eventType,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"payload":   payload,
	})
	// Demo: log structured event. Wire to broker in production.
	log.Printf("[EVENT] %s", string(out))
}

func createReservationHandler(c *gin.Context) {
	start := time.Now()
	var req CreateReservationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		reservationRequests.WithLabelValues("bad_request").Inc()
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !req.CheckOut.After(req.CheckIn) {
		reservationRequests.WithLabelValues("bad_request").Inc()
		c.JSON(http.StatusBadRequest, gin.H{"error": "check_out must be after check_in"})
		return
	}

	totalPrice, err := fetchPriceWithRetry(req)
	if err != nil {
		reservationRequests.WithLabelValues("pricing_error").Inc()
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "pricing service unavailable"})
		return
	}

	r := Reservation{
		ID:         uuid.New().String(),
		UserID:     req.UserID,
		PropertyID: req.PropertyID,
		CheckIn:    req.CheckIn,
		CheckOut:   req.CheckOut,
		Guests:     req.Guests,
		TotalPrice: totalPrice,
		Status:     "confirmed",
		CreatedAt:  time.Now().UTC(),
	}

	storeMu.Lock()
	store[r.ID] = r
	storeMu.Unlock()

	// Async event for downstream notification, audit log, etc.
	publishEvent("ReservationCreated", r)

	reservationRequests.WithLabelValues("success").Inc()
	reservationDuration.WithLabelValues("success").Observe(time.Since(start).Seconds())
	c.JSON(http.StatusCreated, r)
}

func getReservationHandler(c *gin.Context) {
	id := c.Param("id")
	storeMu.RLock()
	r, ok := store[id]
	storeMu.RUnlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "reservation not found"})
		return
	}
	c.JSON(http.StatusOK, r)
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "reservation"})
}

func structuredLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		entry := map[string]interface{}{
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
			"service":    "reservation",
			"method":     c.Request.Method,
			"path":       c.Request.URL.Path,
			"status":     c.Writer.Status(),
			"latency_ms": time.Since(start).Milliseconds(),
		}
		out, _ := json.Marshal(entry)
		log.Println(string(out))
	}
}

func main() {
	port := getEnv("PORT", "8083")

	r := gin.New()
	r.Use(gin.Recovery(), structuredLogger())

	r.GET("/health", healthHandler)
	r.POST("/reservations", createReservationHandler)
	r.GET("/reservations/:id", getReservationHandler)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	log.Printf("reservation service listening on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
