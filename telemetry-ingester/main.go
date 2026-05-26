package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

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

		fmt.Printf("Worker Id = %s, Latency = %f\n", string(msg.Key), float64(record.LatencyNs)/1e6)
	}
}
