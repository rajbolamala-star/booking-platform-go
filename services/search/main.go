package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// SearchResult represents a hotel/property search result.
type SearchResult struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Location    string  `json:"location"`
	PricePerDay float64 `json:"price_per_day"`
	Rating      float64 `json:"rating"`
	Available   bool    `json:"available"`
}

var (
	searchRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "search_requests_total",
			Help: "Total number of search requests",
		},
		[]string{"status"},
	)

	searchDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "search_duration_seconds",
			Help:    "Search request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"status"},
	)
)

func init() {
	prometheus.MustRegister(searchRequests, searchDuration)
}

// searchHandler handles /search?location=...&checkin=...&checkout=...
func searchHandler(c *gin.Context) {
	start := time.Now()
	location := c.Query("location")
	if location == "" {
		searchRequests.WithLabelValues("bad_request").Inc()
		c.JSON(http.StatusBadRequest, gin.H{"error": "location is required"})
		return
	}

	// In production this would query MongoDB with proper indexing.
	// Demo returns mocked results.
	results := []SearchResult{
		{ID: "p1", Name: "Beach Resort", Location: location, PricePerDay: 150.0, Rating: 4.5, Available: true},
		{ID: "p2", Name: "City Hotel", Location: location, PricePerDay: 220.0, Rating: 4.2, Available: true},
		{ID: "p3", Name: "Mountain Lodge", Location: location, PricePerDay: 180.0, Rating: 4.7, Available: false},
	}

	searchRequests.WithLabelValues("success").Inc()
	searchDuration.WithLabelValues("success").Observe(time.Since(start).Seconds())

	c.JSON(http.StatusOK, gin.H{
		"location": location,
		"count":    len(results),
		"results":  results,
	})
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "search"})
}

// structuredLogger logs requests in JSON format for downstream aggregation.
func structuredLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		entry := map[string]interface{}{
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
			"service":    "search",
			"method":     c.Request.Method,
			"path":       c.Request.URL.Path,
			"status":     c.Writer.Status(),
			"latency_ms": time.Since(start).Milliseconds(),
			"client_ip":  c.ClientIP(),
		}
		out, _ := json.Marshal(entry)
		log.Println(string(out))
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	r := gin.New()
	r.Use(gin.Recovery(), structuredLogger())

	r.GET("/health", healthHandler)
	r.GET("/search", searchHandler)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("search service listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s", err)
		}
	}()

	// Graceful shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
