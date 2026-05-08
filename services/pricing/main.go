package main

import (
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PricingRequest represents a pricing lookup.
type PricingRequest struct {
	PropertyID string    `json:"property_id" binding:"required"`
	CheckIn    time.Time `json:"check_in" binding:"required"`
	CheckOut   time.Time `json:"check_out" binding:"required"`
	Guests     int       `json:"guests" binding:"required"`
}

// PricingResponse holds the calculated price.
type PricingResponse struct {
	PropertyID string    `json:"property_id"`
	BasePrice  float64   `json:"base_price"`
	Taxes      float64   `json:"taxes"`
	TotalPrice float64   `json:"total_price"`
	Currency   string    `json:"currency"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// CircuitBreaker is a minimal state-machine for upstream supplier calls.
// Transitions: closed -> open -> half-open -> closed.
type CircuitBreaker struct {
	mu             sync.Mutex
	failureCount   int
	failureThresh  int
	resetTimeout   time.Duration
	lastFailure    time.Time
	state          string // closed | open | half-open
}

func NewCircuitBreaker(threshold int, resetAfter time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		failureThresh: threshold,
		resetTimeout:  resetAfter,
		state:         "closed",
	}
}

// Allow reports whether a call should proceed.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state == "open" && time.Since(cb.lastFailure) > cb.resetTimeout {
		cb.state = "half-open"
	}
	return cb.state != "open"
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount = 0
	cb.state = "closed"
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount++
	cb.lastFailure = time.Now()
	if cb.failureCount >= cb.failureThresh {
		cb.state = "open"
	}
}

var (
	supplierCircuit = NewCircuitBreaker(5, 30*time.Second)

	pricingRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pricing_requests_total",
			Help: "Total number of pricing requests",
		},
		[]string{"status"},
	)

	pricingDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pricing_duration_seconds",
			Help:    "Pricing request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"status"},
	)
)

func init() {
	prometheus.MustRegister(pricingRequests, pricingDuration)
}

// fetchSupplierPrice simulates a 3rd-party supplier API call with retries
// and exponential backoff.
func fetchSupplierPrice(propertyID string) (float64, error) {
	if !supplierCircuit.Allow() {
		return 0, errors.New("supplier circuit open")
	}

	maxAttempts := 3
	backoff := 100 * time.Millisecond
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Simulate ~10% failure rate per call.
		if rand.Float32() < 0.1 {
			lastErr = errors.New("supplier transient error")
			time.Sleep(backoff + time.Duration(rand.Intn(50))*time.Millisecond)
			backoff *= 2
			continue
		}
		// Simulated price.
		basePrice := 100.0 + rand.Float64()*200.0
		supplierCircuit.RecordSuccess()
		return basePrice, nil
	}

	supplierCircuit.RecordFailure()
	return 0, lastErr
}

func priceHandler(c *gin.Context) {
	start := time.Now()
	var req PricingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pricingRequests.WithLabelValues("bad_request").Inc()
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	basePrice, err := fetchSupplierPrice(req.PropertyID)
	if err != nil {
		pricingRequests.WithLabelValues("supplier_error").Inc()
		// Graceful degradation — return a cached/fallback indicator.
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":    "pricing unavailable, please retry",
			"fallback": true,
		})
		return
	}

	nights := int(req.CheckOut.Sub(req.CheckIn).Hours() / 24)
	if nights < 1 {
		nights = 1
	}

	subtotal := basePrice * float64(nights) * float64(req.Guests)
	taxes := subtotal * 0.12
	total := subtotal + taxes

	resp := PricingResponse{
		PropertyID: req.PropertyID,
		BasePrice:  subtotal,
		Taxes:      taxes,
		TotalPrice: total,
		Currency:   "USD",
		ExpiresAt:  time.Now().Add(15 * time.Minute),
	}

	pricingRequests.WithLabelValues("success").Inc()
	pricingDuration.WithLabelValues("success").Observe(time.Since(start).Seconds())
	c.JSON(http.StatusOK, resp)
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "pricing"})
}

func structuredLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		entry := map[string]interface{}{
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
			"service":    "pricing",
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
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	r := gin.New()
	r.Use(gin.Recovery(), structuredLogger())

	r.GET("/health", healthHandler)
	r.POST("/price", priceHandler)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	log.Printf("pricing service listening on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
