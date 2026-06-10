package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/segmentio/kafka-go"
)

type TelemetryRecord struct {
	SubmissionID string `json:"submission_id"`
	Timestamp    int64  `json:"timestamp"`
	LatencyNs    int64  `json:"latency_ns"`
	Status       string `json:"status"`
}

type LiveMetrics struct {
	SubmissionID string `json:"submission_id"`
	TPS          int64  `json:"tps"`
	P50          int64  `json:"p50"`
	P90          int64  `json:"p90"`
	P99          int64  `json:"p99"`
	Status       string `json:"status,omitempty"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	ip := r.RemoteAddr

	// Enforce Token-Bucket Flood Protection before completing protocol upgrade
	if !hub.AllowIP(ip) {
		http.Error(w, "Too Many Connection Requests", http.StatusTooManyRequests)
		log.Printf("[Security] Rejected connections flood from IP: %s", ip)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Ingester] WebSocket Upgrade Error: %v", err)
		return
	}

	client := &Client{
		hub:  hub,
		conn: ws,
		send: make(chan LiveMetrics, 256),
		ip:   ip,
	}

	client.hub.register <- client

	// Start reading and writing asynchronously using dedicated client Goroutines
	go client.WritePump()
	go client.ReadPump()
}

func main() {
	// Initialize Postgres Context Connection
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://tradeforces:supersecretpassword@localhost:5433/tradeforces_db?sslmode=disable"
	}
	dbConn, err := pgx.Connect(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("[Database] Unable to connect to PostgreSQL: %v", err)
	}
	defer dbConn.Close(context.Background())
	fmt.Println("[Database] Connected directly to PostgreSQL database.")

	// Spin up Actor Hub
	hub := NewHub()
	go hub.Run()

	// Launch HTTP Server binding endpoint directly to our stateful hub router
	go func() {
		http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
			serveWs(hub, w, r)
		})
		log.Fatal(http.ListenAndServe(":8081", nil))
	}()

	fmt.Println("[Ingester] WebSocket Server live on ws://localhost:8081/ws")
	fmt.Println("[Ingester] Waiting for Redpanda data...")

	kafkaReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{"localhost:9092"},
		Topic:    "telemetry",
		GroupID:  "ingester-group",
		MinBytes: 10e3,
		MaxBytes: 10e6,
	})
	defer kafkaReader.Close()

	recordChan := make(chan TelemetryRecord, 50000)
	go readKafka(kafkaReader, recordChan)

	ticker := time.NewTicker(time.Second)

	currentWindows := make(map[string][]int64)
	globalLatencies := make(map[string][]float64)
	runStartTimes := make(map[string]int64)

	for {
		select {
		case record := <-recordChan:
			subID := record.SubmissionID

			if record.Status == "END_OF_RUN" {
				fmt.Printf("[Ingester] END_OF_RUN received for %s. Calculating final score...\n", subID)

				finalMetrics, err := calculateAndPersistFinalScore(
					dbConn,
					subID,
					globalLatencies[subID],
					runStartTimes[subID],
					record.Timestamp,
				)

				if err != nil {
					fmt.Printf("[Ingester Error][%s] %v\n", subID, err)
				} else {
					finalMetrics.Status = "COMPLETE"
					hub.broadcast <- finalMetrics
				}

				delete(currentWindows, subID)
				delete(globalLatencies, subID)
				delete(runStartTimes, subID)
				continue
			}

			if _, exists := runStartTimes[subID]; !exists || record.Timestamp < runStartTimes[subID] {
				runStartTimes[subID] = record.Timestamp
			}

			currentWindows[subID] = append(currentWindows[subID], record.LatencyNs)
			globalLatencies[subID] = append(globalLatencies[subID], float64(record.LatencyNs))

		case <-ticker.C:
			for subID, window := range currentWindows {
				if len(window) == 0 {
					continue
				}

				sort.Slice(window, func(i, j int) bool { return window[i] < window[j] })
				requests := int64(len(window))

				p50 := window[int(float64(requests)*0.50)] / 1000
				p90 := window[int(float64(requests)*0.90)] / 1000
				p99 := window[int(float64(requests)*0.99)] / 1000

				metrics := LiveMetrics{
					SubmissionID: subID,
					TPS:          requests,
					P50:          p50,
					P90:          p90,
					P99:          p99,
				}

				// Ship metric payload straight down the Actor Model Hub channel
				hub.broadcast <- metrics

				currentWindows[subID] = nil
			}
		}
	}
}

func readKafka(kafkaReader *kafka.Reader, recordChan chan TelemetryRecord) {
	for {
		msg, err := kafkaReader.ReadMessage(context.Background())
		if err != nil {
			continue
		}
		var record TelemetryRecord
		if err := json.Unmarshal(msg.Value, &record); err == nil {
			recordChan <- record
		}
	}
}

func calculateAndPersistFinalScore(dbConn *pgx.Conn, subID string, latencies []float64, startNs, endNs int64) (LiveMetrics, error) {
	totalOrders := len(latencies)
	if totalOrders == 0 {
		return LiveMetrics{}, fmt.Errorf("no telemetry data collected for %s", subID)
	}

	sort.Float64s(latencies)

	durationSec := float64(endNs-startNs) / 1e9
	tps := float64(totalOrders) / durationSec

	p50Idx := int(float64(totalOrders) * 0.50)
	if p50Idx >= totalOrders {
		p50Idx = totalOrders - 1
	}
	p50 := latencies[p50Idx] / 1e3

	p90Idx := int(float64(totalOrders) * 0.90)
	if p90Idx >= totalOrders {
		p90Idx = totalOrders - 1
	}
	p90 := latencies[p90Idx] / 1e3

	p99Idx := int(float64(totalOrders) * 0.99)
	if p99Idx >= totalOrders {
		p99Idx = totalOrders - 1
	}
	p99 := latencies[p99Idx] / 1e3

	var sum float64
	for _, l := range latencies {
		sum += l
	}
	mean := sum / float64(totalOrders)

	var varianceSum float64
	for _, l := range latencies {
		varianceSum += math.Pow(l-mean, 2)
	}
	stdDev := math.Sqrt(varianceSum / float64(totalOrders))

	cv := 0.0
	if mean > 0 {
		cv = stdDev / mean
	}

	fmt.Printf("[Final Score][%s] TPS: %.2f | p99: %.2fµs | CV: %.4f\n", subID, tps, p99, cv)

	resultQuery := `
		INSERT INTO "Result" (id, "submissionId", tps, p50, p90, p99, cv, "createdAt")
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT ("submissionId") DO UPDATE SET
			tps = EXCLUDED.tps,
			p50 = EXCLUDED.p50,
			p90 = EXCLUDED.p90,
			p99 = EXCLUDED.p99,
			cv = EXCLUDED.cv;`

	_, err := dbConn.Exec(context.Background(), resultQuery, subID, int(tps), p50, p90, p99, cv)
	if err != nil {
		return LiveMetrics{}, fmt.Errorf("DB Result write failed: %w", err)
	}

	submissionQuery := `UPDATE "Submission" SET status = 'SUCCESS' WHERE id = $1`
	_, err = dbConn.Exec(context.Background(), submissionQuery, subID)
	if err != nil {
		return LiveMetrics{}, fmt.Errorf("DB Submission update failed: %w", err)
	}

	fmt.Printf("[Database] Final benchmark results securely committed for run: %s\n", subID)

	return LiveMetrics{
		SubmissionID: subID,
		TPS:          int64(tps),
		P50:          int64(p50),
		P90:          int64(p90),
		P99:          int64(p99),
	}, nil
}
