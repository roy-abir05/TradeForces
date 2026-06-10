package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
)

type TelemetryRecord struct {
	SubmissionID string `json:"submission_id"`
	Timestamp    int64  `json:"timestamp"`
	LatencyNs    int64  `json:"latency_ns"`
	Status       string `json:"status"`
}

type Profile struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Phases      []Phase `json:"phases"`
}

type Phase struct {
	Name             string       `json:"name"`
	DurationSec      int          `json:"duration_sec"`
	TargetTPS        int          `json:"target_tps"`
	Distribution     Distribution `json:"distribution"`
	PriceVariancePct float64      `json:"price_variance_pct"`
}

type Distribution struct {
	Buy    int `json:"buy"`
	Sell   int `json:"sell"`
	Cancel int `json:"cancel"`
}

type AtomicPhaseState struct {
	BuyWt    int
	SellWt   int
	CancelWt int
}

var EntropySequence [10000]int
var currentPhaseState atomic.Pointer[AtomicPhaseState]

func main() {
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 10000; i++ {
		EntropySequence[i] = rng.Intn(100)
	}

	submissionID := os.Getenv("SUBMISSION_ID")
	if submissionID == "" {
		log.Fatal("FATAL: SUBMISSION_ID environment variable not set")
	}

	chaosJSON := os.Getenv("CHAOS_PROFILE")
	if chaosJSON == "" {
		log.Fatal("FATAL: CHAOS_PROFILE environment variable not set")
	}

	var currentProfile Profile
	err := json.Unmarshal([]byte(chaosJSON), &currentProfile)
	if err != nil {
		log.Fatalf("FATAL: Could not parse chaos profile: %v", err)
	}

	var kafkaWriter *kafka.Writer = &kafka.Writer{
		Addr:     kafka.TCP("localhost:9092"),
		Topic:    "telemetry",
		Balancer: &kafka.LeastBytes{},
	}
	defer kafkaWriter.Close()
	var wg = sync.WaitGroup{}

	var kafkaWG = sync.WaitGroup{}

	workers := 100

	fmt.Printf("Deploying %d workers for Gauntlet: %s\n", workers, currentProfile.Name)

	attackTokens := make(chan int, 100000)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker(i, attackTokens, kafkaWriter, &wg, &kafkaWG, submissionID)
	}

	tokenID := 0
	for _, phase := range currentProfile.Phases {
		fmt.Printf("Starting Phase: %s\n", phase.Name)

		state := &AtomicPhaseState{
			BuyWt:    phase.Distribution.Buy,
			SellWt:   phase.Distribution.Sell,
			CancelWt: phase.Distribution.Cancel,
		}
		currentPhaseState.Store(state)

		if phase.TargetTPS == 0 {
			time.Sleep(time.Duration(phase.DurationSec) * time.Second)
			continue
		}

		delay := time.Second / time.Duration(phase.TargetTPS)
		ticker := time.NewTicker(delay)
		timeout := time.After(time.Duration(phase.DurationSec) * time.Second)

	PhaseLoop:
		for {
			select {
			case <-ticker.C:
				attackTokens <- tokenID
				tokenID++
			case <-timeout:
				ticker.Stop()
				break PhaseLoop
			}
		}
	}
	close(attackTokens)
	wg.Wait()
	kafkaWG.Wait()

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
			Key:   []byte(submissionID),
			Value: endJSON,
		},
	)
}

func worker(workerID int, attackTokens <-chan int, kafkaWriter *kafka.Writer, wg *sync.WaitGroup, kafkaWG *sync.WaitGroup, submissionID string) {
	defer wg.Done()

	targetPort := os.Getenv("TARGET_PORT")
	if targetPort == "" {
		targetPort = "1337"
	}

	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%s", targetPort))
	if err != nil {
		log.Printf("Worker %d failed to connect: %v\n", workerID, err)
		return
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	for tokenID := range attackTokens {

		state := currentPhaseState.Load()
		if state == nil {
			continue
		}

		entropy := EntropySequence[tokenID%10000]

		var orderType string
		if entropy < state.BuyWt {
			orderType = "BUY"
		} else if entropy < state.BuyWt+state.SellWt {
			orderType = "SELL"
		} else {
			orderType = "CANCEL"
		}

		var orderStr string
		targetPrice := 60000

		if orderType == "CANCEL" {
			targetCancelID := (tokenID % 5000) + 1
			orderStr = fmt.Sprintf("CANCEL %d\n", targetCancelID)
		} else {
			priceOffset := (tokenID % 50)
			if tokenID%2 == 0 {
				targetPrice += priceOffset
			} else {
				targetPrice -= priceOffset
			}
			orderStr = fmt.Sprintf("%s 100 BTC @ %d ID=%d\n", orderType, targetPrice, tokenID+1)
		}

		t0 := time.Now()

		fmt.Fprint(conn, orderStr)

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
			Status:       "SUCCESS",
		}
		jsonData, err := json.Marshal(record)
		if err != nil {
			continue
		}

		kafkaWG.Add(1)

		go func(val []byte, wID int) {
			defer kafkaWG.Done()
			err = kafkaWriter.WriteMessages(context.Background(),
				kafka.Message{
					Key:   []byte(submissionID),
					Value: val,
				},
			)
			if err != nil {
				log.Printf("Worker %d failed to write to Kafka: %v\n", wID, err)
			}
		}(jsonData, workerID)
	}
}
