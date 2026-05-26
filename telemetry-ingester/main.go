package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/segmentio/kafka-go"
)

type TelemetryRecord struct {
	LatencyNs int64  `json:"latency_ns"`
	Status    string `json:"status"`
}

func main() {
	var kafkaReader *kafka.Reader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{"localhost:9092"},
		Topic:    "telemetry",
		GroupID:  "referee-group", // Setting this enables consumer groups
		MinBytes: 10e3,            // 10KB
		MaxBytes: 10e6,            // 10MB
	})

	defer kafkaReader.Close()

	fmt.Println("[Telemetry Ingester] Referee is online. Waiting for Redpanda data...")

	var latenciesChan chan int64 = make(chan int64, 10000)

	go readKafka(kafkaReader, latenciesChan)

	ticker := time.NewTicker(time.Second)
	var currentWindow []int64

	for {
		select{
		case lat := <-latenciesChan:
			currentWindow = append(currentWindow, lat)
		case <- ticker.C:
			if len(currentWindow)==0{
				continue
			}
			slices.Sort(currentWindow)
			var requests int64 = int64(len(currentWindow))
			var tps int64 = requests
			var p50, p90, p99 int64
			p50 = currentWindow[int64(float64(requests)*0.5) - 1]
			p90 = currentWindow[int64(float64(requests)*0.9) - 1]
			p99 = currentWindow[int64(float64(requests)*0.99) - 1]

			fmt.Printf("[Telemetry Ingester: 1s TICK] TPS: %d | p50: %dµs | p90: %dµs | p99: %dµs\n", tps, p50/1000, p90/1000, p99/1000)

			currentWindow = nil
		}
	}
}

func readKafka(kafkaReader *kafka.Reader, latenciesChan chan int64){
	for {
		msg, err := kafkaReader.ReadMessage(context.Background())
		if err != nil {
			log.Printf("[Telemetry Ingester] Failed to Read message from Kafka")
			continue
		}

		var record TelemetryRecord
		err = json.Unmarshal(msg.Value, &record)
		if err != nil {
			log.Printf("[Telemetry Ingester] Failed to Parse message into json")
			continue
		}

		latenciesChan <- record.LatencyNs
	}
}
