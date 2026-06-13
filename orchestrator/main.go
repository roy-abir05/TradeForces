package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v3"
)

type Orchestrator struct {
	dbPool       *pgxpool.Pool
	dockerClient *client.Client
	queue        chan DeployPayload
	workerSem    chan struct{}
}

type DeployPayload struct {
	SubmissionID string `json:"submissionId"`
	Code         string `json:"code"`
}

func (o *Orchestrator) deployHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload DeployPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if payload.SubmissionID == "" || payload.Code == "" {
		http.Error(w, "Missing submissionId or code", http.StatusBadRequest)
		return
	}

	// Push to the internal queue and instantly free the HTTP connection
	o.queue <- payload

	_, err := o.dbPool.Exec(context.Background(), `UPDATE "Submission" SET status = 'PENDING' WHERE id = $1`, payload.SubmissionID)
	if err != nil {
		fmt.Printf("[Orchestrator][%s] Failed to set PENDING status: %v\n", payload.SubmissionID[:8], err)
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Deployed and Queued"))
}

func (o *Orchestrator) dispatcher() {
	for payload := range o.queue {
		o.workerSem <- struct{}{}
		go o.processSubmission(payload)
	}
}

func (o *Orchestrator) processSubmission(payload DeployPayload) {
	defer func() { <-o.workerSem }()

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	subID := payload.SubmissionID
	fmt.Printf("\n[Orchestrator] Starting processing for Submission: %s\n", subID)

	_, err := o.dbPool.Exec(ctx, `UPDATE "Submission" SET status = 'RUNNING' WHERE id = $1`, subID)
	if err != nil {
		fmt.Printf("[Orchestrator][%s] DB Update Error: %v\n", subID[:16], err)
	}

	tempDir, err := os.MkdirTemp("", fmt.Sprintf("tradeforces-sub-%s-*", subID[:16]))
	if err != nil {
		log.Printf("[%s] Failed to create temp directory: %v", subID[:16], err)
		return
	}

	if err := os.Chmod(tempDir, 0777); err != nil {
		log.Printf("[%s] Failed to chmod temp directory: %v", subID[:16], err)
		return
	}

	sourcePath := filepath.Join(tempDir, "server.cpp")
	if err := os.WriteFile(sourcePath, []byte(payload.Code), 0644); err != nil {
		os.RemoveAll(tempDir)
		log.Printf("[%s] Failed to write code: %v", subID[:16], err)
		return
	}

	// The Port 0 Trick: Ask the OS for a free dynamic port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.RemoveAll(tempDir)
		log.Printf("[%s] Failed to allocate dynamic port: %v", subID[:16], err)
		return
	}
	hostPort := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	listener.Close()

	hostPath := tempDir
	containerPath := "/usr/src/app"

	engineConfigJSON := []byte(`{
        "Image": "gcc:latest",
        "WorkingDir": "/usr/src/app",
        "Cmd": ["sh", "-c", "g++ -O3 -pthread server.cpp -o server && ./server"],
        "ExposedPorts": {
            "1337/tcp": {}
        }
    }`)
	var engineConfig container.Config
	if err := json.Unmarshal(engineConfigJSON, &engineConfig); err != nil {
		os.RemoveAll(tempDir)
		log.Fatalf("JSON Parse Error: %v", err)
	}

	sandboxConfigJSON := []byte(fmt.Sprintf(`{
        "Binds": ["%s:%s"],
        "NetworkMode": "tradeforces-net",
        "CapDrop": ["ALL"],
        "PortBindings": {
            "1337/tcp": [
                {
                    "HostIp": "127.0.0.1",
                    "HostPort": "%s"
                }
            ]
        },
        "NanoCPUs": 500000000,
        "Memory": 134217728,
        "MemorySwap": 134217728
    }`, hostPath, containerPath, hostPort))

	var sandboxConfig container.HostConfig
	if err := json.Unmarshal(sandboxConfigJSON, &sandboxConfig); err != nil {
		os.RemoveAll(tempDir)
		log.Fatalf("JSON Parse Error: %v", err)
	}

	resp, err := o.dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     &engineConfig,
		HostConfig: &sandboxConfig,
	})
	if err != nil {
		os.RemoveAll(tempDir)
		log.Printf("[%s] Failed to create container: %v", subID[:16], err)
		return
	}

	defer func() {
		os.RemoveAll(tempDir)
		exec.Command("docker", "rm", "-f", resp.ID).Run()
		fmt.Printf("[Orchestrator][%s] Cleanup complete.\n", subID[:16])
	}()

	if _, err := o.dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		log.Printf("[%s] Failed to start container: %v", subID[:16], err)
		return
	}

	fmt.Printf("[Orchestrator][%s] Engine running on dynamically allocated port %s\n", subID[:16], hostPort)

	targetAddress := fmt.Sprintf("127.0.0.1:%s", hostPort)
	engineReady := false

	for range 50 {
		conn, err := net.DialTimeout("tcp", targetAddress, 100*time.Millisecond)
		if err == nil {
			conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			var buf [1]byte
			_, readErr := conn.Read(buf[:])

			if readErr != nil && !os.IsTimeout(readErr) {
				conn.Close()
				time.Sleep(100 * time.Millisecond)
				continue
			}

			conn.Close()
			engineReady = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !engineReady {
		fmt.Printf("[Orchestrator][%s] FATAL: Engine failed to bind to port %s within 5 seconds.\n", subID[:16], hostPort)
		logCmd := exec.Command("docker", "logs", resp.ID)
		logCmd.Stdout = os.Stdout
		logCmd.Stderr = os.Stderr
		logCmd.Run()

		o.dbPool.Exec(ctx, `UPDATE "Submission" SET status = 'FAILED' WHERE id = $1`, subID)
		return
	}

	fmt.Printf("[Orchestrator][%s] Target Lock Acquired. Unleashing load generator...\n", subID[:16])

	entries, err := os.ReadDir("./profiles")
	if err != nil {
		log.Printf("[%s] Failed to read profiles directory: %v", subID[:16], err)
		return
	}
	wait := o.dockerClient.ContainerWait(ctx, resp.ID, client.ContainerWaitOptions{
		Condition: container.WaitConditionNotRunning,
	})

	for _, entry := range entries {
		if entry.IsDir() || (filepath.Ext(entry.Name()) != ".yml" && filepath.Ext(entry.Name()) != ".yaml") {
			continue
		}

		// fmt.Printf("[Orchestrator][%s] Running Test Case: %s\n", subID[:16], entry.Name())

		yamlData, err := os.ReadFile(filepath.Join("./profiles", entry.Name()))
		if err != nil {
			log.Printf("[%s] Failed to read profile %s: %v", subID[:16], entry.Name(), err)
			continue
		}

		var profileMap map[string]interface{}
		yaml.Unmarshal(yamlData, &profileMap)

		jsonProfileBytes, _ := json.Marshal(profileMap)

		cmd := exec.CommandContext(ctx, "go", "run", "main.go")
		cmd.Dir = "../load-generator"
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("SUBMISSION_ID=%s", subID),
			fmt.Sprintf("CHAOS_PROFILE=%s", string(jsonProfileBytes)),
			fmt.Sprintf("TARGET_PORT=%s", hostPort), // Inform generator of the correct dynamic port
		)

		attackDone := make(chan error, 1)
		go func() {
			attackDone <- cmd.Run()
		}()

		timeout := time.After(60 * time.Second)
		testFailed := false

		select {
		case err := <-attackDone:
			if err != nil {
				fmt.Printf("[Orchestrator][%s] Load generator failed on %s: %v\n", subID[:16], entry.Name(), err)
				testFailed = true
				o.dbPool.Exec(context.Background(), `UPDATE "Submission" SET status = 'FAILED' WHERE id = $1`, subID)
			} else {
				fmt.Printf("[Orchestrator][%s] Passed: %s\n", subID[:16], entry.Name())
			}

		case waitResp := <-wait.Result:
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			verdict := "RE"
			if waitResp.StatusCode == 137 {
				verdict = "MLE"
				fmt.Printf("[Orchestrator][%s] FATAL: Memory Limit Exceeded on %s.\n", subID[:16], entry.Name())
			} else {
				fmt.Printf("[Orchestrator][%s] FATAL: Runtime Error (Exit Code %d) on %s.\n", subID[:16], waitResp.StatusCode, entry.Name())
			}
			testFailed = true
			o.dbPool.Exec(context.Background(), `UPDATE "Submission" SET status = $1 WHERE id = $2`, verdict, subID)

		case <-timeout:
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			fmt.Printf("[Orchestrator][%s] FATAL: Time Limit Exceeded (Test Profile) on %s.\n", subID[:16], entry.Name())
			testFailed = true
			o.dbPool.Exec(context.Background(), `UPDATE "Submission" SET status = 'TLE' WHERE id = $1`, subID)

		case err := <-wait.Error:
			if cmd.Process != nil {
				cmd.Process.Kill()
			}

			if ctx.Err() == context.DeadlineExceeded {
				fmt.Printf("[Orchestrator][%s] FATAL: 5-MINUTE GLOBAL TIMEOUT. Worker slot forcefully reclaimed.\n", subID[:16])
				o.dbPool.Exec(context.Background(), `UPDATE "Submission" SET status = 'TLE' WHERE id = $1`, subID)
			} else {
				fmt.Printf("[Orchestrator][%s] Docker API Error: %v\n", subID[:16], err)
				o.dbPool.Exec(context.Background(), `UPDATE "Submission" SET status = 'FAILED' WHERE id = $1`, subID)
			}
			testFailed = true
		}

		if testFailed {
			return
		}
	}

	fmt.Printf("[Orchestrator][%s] All test cases passed.\n", subID[:16])
}

func (o *Orchestrator) startJanitor() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	fmt.Println("[Janitor] Scheduled garbage collection initialized (1-hour intervals).")

	for range ticker.C {
		fmt.Println("[Janitor] Waking up. Executing routine Docker system prune...")

		cmd := exec.Command("docker", "system", "prune", "-f", "--volumes")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			fmt.Printf("[Janitor] Warning: Prune command failed: %v\n", err)
		} else {
			fmt.Println("[Janitor] Host machine sanitized. Going back to sleep.")
		}
	}
}

func main() {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://tradeforces:supersecretpassword@localhost:5433/tradeforces_db?sslmode=disable"
	}

	dbPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("[Database] Unable to create connection pool: %v", err)
	}
	defer dbPool.Close()
	fmt.Println("[Orchestrator] Connected to PostgreSQL connection pool.")

	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("[Docker] Failed to initialize Docker client: %v", err)
	}
	defer dockerClient.Close()
	fmt.Println("[Orchestrator] Docker client initialized.")

	fmt.Println("[Orchestrator] Pre-flight check: Verifying gcc:latest image is cached locally...")
	pullCtx := context.Background()
	reader, err := dockerClient.ImagePull(pullCtx, "docker.io/library/gcc:latest", client.ImagePullOptions{})
	if err != nil {
		log.Fatalf("[Docker] Failed to pull sandbox image: %v", err)
	}
	io.Copy(os.Stdout, reader)
	reader.Close()
	fmt.Println("[Orchestrator] Sandbox image verified. Ready for execution.")

	maxWorkers := 1
	if mwStr := os.Getenv("MAX_WORKERS"); mwStr != "" {
		if val, err := strconv.Atoi(mwStr); err == nil && val > 0 {
			maxWorkers = val
		}
	}

	orch := &Orchestrator{
		dbPool:       dbPool,
		dockerClient: dockerClient,
		queue:        make(chan DeployPayload, 10000),
		workerSem:    make(chan struct{}, maxWorkers),
	}

	go orch.dispatcher()
	go orch.startJanitor()

	http.HandleFunc("/deploy", orch.deployHandler)
	fmt.Printf("[Orchestrator] Listening on http://localhost:8080 (Max Workers: %d)\n", maxWorkers)
	log.Fatal(http.ListenAndServe(":8080", nil))

	http.Handle("/metrics", promhttp.Handler())
}
