package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

type TelemetryRecord struct {
	SubmissionID string `json:"submission_id"`
	Timestamp    int64  `json:"timestamp"`
	LatencyNs    int64  `json:"latency_ns"`
	Status       string `json:"status"`
}

func main() {

	submissionID := os.Getenv("SUBMISSION_ID")
	if submissionID == "" {
		log.Fatal("FATAL: SUBMISSION_ID environment variable not set")
	}

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
		go worker(i, requests, kafkaWriter, &wg, submissionID)
	}
	wg.Wait()
	fmt.Println("Fleet attack complete. All telemetry data pushed to Redpanda.")

	endRecord := TelemetryRecord{
		SubmissionID: submissionID,
		Timestamp:    time.Now().UnixNano(),
		LatencyNs:    0,
		Status:       "END_OF_RUN",
	}
	endJSON, _ := json.Marshal(endRecord)

	kafkaWriter.WriteMessages(context.Background(),
		kafka.Message{
			Key:   []byte(fmt.Sprintf("signal-%s", submissionID)),
			Value: endJSON,
		},
	)
}

func worker(workerID int, requests int, kafkaWriter *kafka.Writer, wg *sync.WaitGroup, submissionID string) {
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

		record := TelemetryRecord{
			SubmissionID: submissionID,
			Timestamp:    t0.UnixNano(),
			LatencyNs:    latency,
			Status:       "success",
		}
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
