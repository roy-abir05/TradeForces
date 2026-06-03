package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/segmentio/kafka-go"
)

type TelemetryRecord struct {
	LatencyNs int64  `json:"latency_ns"`
	Status    string `json:"status"`
}

type LiveMetrics struct {
	TPS int64 `json:"tps"`
	P50 int64 `json:"p50"`
	P90 int64 `json:"p90"`
	P99 int64 `json:"p99"`
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
		log.Printf("[Referee] WebSocket Upgrade Error: %v", err)
		return
	}
	clientsMu.Lock()
	clients[ws] = true
	clientsMu.Unlock()
	fmt.Println("[Referee] Next.js Server connected to Live Feed.")
}

func main() {

	go func() {
		http.HandleFunc("/ws", wsHandler)
		log.Fatal(http.ListenAndServe(":8081", nil))
	}()

	fmt.Println("[Referee] WebSocket Server live on ws://localhost:8081/ws")
	fmt.Println("[Referee] Waiting for Redpanda data...")

	kafkaReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{"localhost:9092"},
		Topic:    "telemetry",
		GroupID:  "referee-group",
		MinBytes: 10e3,
		MaxBytes: 10e6,
	})
	defer kafkaReader.Close()

	latenciesChan := make(chan int64, 10000)
	go readKafka(kafkaReader, latenciesChan)

	ticker := time.NewTicker(time.Second)
	var currentWindow []int64

	for {
		select {
		case lat := <-latenciesChan:
			currentWindow = append(currentWindow, lat)
		case <-ticker.C:
			if len(currentWindow) == 0 {
				continue
			}
			slices.Sort(currentWindow)
			requests := int64(len(currentWindow))

			p50 := currentWindow[int64(float64(requests)*0.5)-1] / 1000
			p90 := currentWindow[int64(float64(requests)*0.9)-1] / 1000
			p99 := currentWindow[int64(float64(requests)*0.99)-1] / 1000

			fmt.Printf("[1s TICK] TPS: %d | p50: %dµs | p90: %dµs | p99: %dµs\n", requests, p50, p90, p99)

			metrics := LiveMetrics{TPS: requests, P50: p50, P90: p90, P99: p99}

			clientsMu.Lock()
			for client := range clients {
				err := client.WriteJSON(metrics)
				if err != nil {
					client.Close()
					delete(clients, client)
				}
			}
			clientsMu.Unlock()

			currentWindow = nil
		}
	}
}

func readKafka(kafkaReader *kafka.Reader, latenciesChan chan int64) {
	for {
		msg, err := kafkaReader.ReadMessage(context.Background())
		if err != nil {
			continue
		}
		var record TelemetryRecord
		json.Unmarshal(msg.Value, &record)
		latenciesChan <- record.LatencyNs
	}
}
