package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
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
}

var (
	clients   = make(map[*websocket.Conn]bool)
	clientsMu sync.Mutex
	upgrader  = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

func wsHandler(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Ingester] WebSocket Upgrade Error: %v", err)
		return
	}
	clientsMu.Lock()
	clients[ws] = true
	clientsMu.Unlock()
	fmt.Println("[Ingester] Next.js Server connected to Live Feed.")
}

func main() {
	go func() {
		http.HandleFunc("/ws", wsHandler)
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
	runEndTimes := make(map[string]int64)

	for {
		select {
		case record := <-recordChan:
			subID := record.SubmissionID

			if record.Status == "END_OF_RUN" {
				fmt.Printf("[Ingester] END_OF_RUN received for %s. Calculating final score...\n", subID)

				calculateAndSendFinalScore(
					subID,
					globalLatencies[subID],
					runStartTimes[subID],
					record.Timestamp,
				)

				delete(currentWindows, subID)
				delete(globalLatencies, subID)
				delete(runStartTimes, subID)
				delete(runEndTimes, subID)
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

				clientsMu.Lock()
				for client := range clients {
					if err := client.WriteJSON(metrics); err != nil {
						client.Close()
						delete(clients, client)
					}
				}
				clientsMu.Unlock()

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

func calculateAndSendFinalScore(subID string, latencies []float64, startNs, endNs int64) {
	totalOrders := len(latencies)
	if totalOrders == 0 {
		fmt.Printf("[Ingester] No data collected for %s\n", subID)
		return
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

	payload := map[string]interface{}{
		"submissionId": subID,
		"status":       "SUCCESS",
		"tps":          tps,
		"p50":          p50,
		"p90":          p90,
		"p99":          p99,
		"cv":           cv,
	}

	jsonData, _ := json.Marshal(payload)

	webURL := os.Getenv("WEB_URL")
	if webURL == "" {
		webURL = "http://localhost:3000"
	}

	resp, err := http.Post(webURL+"/api/results", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("[Ingester] Failed to save final score to DB: %v\n", err)
		return
	}
	defer resp.Body.Close()
}
