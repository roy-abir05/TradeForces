package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

type TelemetryRecord struct {
	LatencyNs int64  `json:"latency_ns"`
	Status    string `json:"status"`
}

func main() {
	var kafkaWriter *kafka.Writer = &kafka.Writer{
		Addr:     kafka.TCP("localhost:9092"),
		Topic:    "telemetry",
		Balancer: &kafka.LeastBytes{},
	}
	defer kafkaWriter.Close()
	var wg = sync.WaitGroup{}

	workers, requests := 100, 100

	fmt.Printf("Deploying %d workers to fire %d requests each...\n", workers, requests)

	for i := range workers {
		wg.Add(1)
		go worker(i, requests, kafkaWriter, &wg)
	}
	wg.Wait()
	fmt.Println("Fleet attack complete. All telemetry data pushed to Redpanda.")
}

func worker(workerID int, requests int, kafkaWriter *kafka.Writer, wg *sync.WaitGroup) {
	defer wg.Done()

	conn, err := net.Dial("tcp", "localhost:1337")
	if err != nil {
		log.Printf("Worker %d failed to connect: %v\n", workerID, err)
		return
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	order := "BUY 100 BTC @ 60000\n"

	for i := 0; i < requests; i++ {
		t0 := time.Now()

		fmt.Fprint(conn, order)

		_, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Worker %d connection dropped.\n", workerID)
			break
		}

		latency := time.Since(t0).Nanoseconds()
		var record TelemetryRecord = TelemetryRecord{latency, "success"}
		jsonData, err := json.Marshal(record)
		if err != nil {
			continue
		}

		err = kafkaWriter.WriteMessages(context.Background(),
			kafka.Message{
				Key:   []byte(fmt.Sprintf("worker-%d", workerID)),
				Value: jsonData,
			},
		)

		if err != nil {
			log.Printf("Worker %d failed to write to Kafka: %v\n", workerID, err)
		}
	}
}
