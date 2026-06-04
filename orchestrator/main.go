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
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type DeployPayload struct {
	SubmissionID string `json:"submissionId"`
	Code         string `json:"code"`
}

func deployHandler(w http.ResponseWriter, r *http.Request) {
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

	tempDir, err := os.MkdirTemp("", "tradeforces-sub-*")
	if err != nil {
		http.Error(w, "Failed to create temp directory", http.StatusInternalServerError)
		return
	}

	sourcePath := filepath.Join(tempDir, "server.cpp")
	if err := os.WriteFile(sourcePath, []byte(payload.Code), 0644); err != nil {
		os.RemoveAll(tempDir)
		http.Error(w, "Failed to write code to temp file", http.StatusInternalServerError)
		return
	}

	ctx := context.Background()
	apiClient, err := client.New(client.FromEnv)
	if err != nil {
		os.RemoveAll(tempDir)
		log.Fatalf("Failed to connect to Docker: %v", err)
	}
	defer apiClient.Close()

	fmt.Printf("\n[Orchestrator] Received deployment request for Submission: %s\n", payload.SubmissionID)

	reader, err := apiClient.ImagePull(ctx, "docker.io/library/gcc:latest", client.ImagePullOptions{})
	if err != nil {
		os.RemoveAll(tempDir)
		log.Fatalf("Image pull failed: %v", err)
	}
	defer reader.Close()
	io.Copy(os.Stdout, reader)

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
        "PortBindings": {
            "1337/tcp": [
                {
                    "HostIp": "0.0.0.0",
                    "HostPort": "1337"
                }
            ]
        },
        "NanoCPUs": 500000000,
        "Memory": 134217728
    }`, hostPath, containerPath))
	var sandboxConfig container.HostConfig
	if err := json.Unmarshal(sandboxConfigJSON, &sandboxConfig); err != nil {
		os.RemoveAll(tempDir)
		log.Fatalf("JSON Parse Error: %v", err)
	}

	resp, err := apiClient.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     &engineConfig,
		HostConfig: &sandboxConfig,
	})
	if err != nil {
		os.RemoveAll(tempDir)
		log.Fatalf("Failed to create container: %v", err)
	}

	_, err = apiClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{})
	if err != nil {
		os.RemoveAll(tempDir)
		log.Fatalf("Failed to start container: %v", err)
	}

	fmt.Printf("[Orchestrator] Engine running in Sandbox ID: %s\n", resp.ID[:12])

	go func(subID string, ephemeralDir string, containerID string) {
		defer func() {
			os.RemoveAll(ephemeralDir)
			cleanupCmd := exec.Command("docker", "rm", "-f", containerID)
			cleanupCmd.Run()
			fmt.Printf("[Orchestrator][%s] Cleanup complete.\n", subID[:8])
		}()

		targetAddress := "localhost:1337"
		engineReady := false

		for range 50 {
			conn, err := net.DialTimeout("tcp", targetAddress, 100*time.Millisecond)
			if err == nil {
				conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
				var buf [1]byte
				_, readErr := conn.Read(buf[:])

				if readErr != nil {
					if !os.IsTimeout(readErr) {
						conn.Close()
						time.Sleep(100 * time.Millisecond)
						continue
					}
				}

				conn.Close()
				engineReady = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		if !engineReady {
			fmt.Printf("[Orchestrator][%s] FATAL: Engine failed to bind to port 1337 within 5 seconds.\n", subID[:8])
			logCmd := exec.Command("docker", "logs", containerID)
			logCmd.Stdout = os.Stdout
			logCmd.Stderr = os.Stderr
			logCmd.Run()
			return
		}

		fmt.Printf("[Orchestrator][%s] Target Lock Acquired. Unleashing load generator...\n", subID[:8])

		cmd := exec.Command("go", "run", "main.go")
		cmd.Dir = "../load-generator"
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			fmt.Printf("[Orchestrator][%s] Load generator failed: %v\n", subID[:8], err)
		} else {
			fmt.Printf("[Orchestrator][%s] Attack complete.\n", subID[:8])
		}

	}(payload.SubmissionID, tempDir, resp.ID)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Deployed and Attack Initiated"))
}

func main() {
	http.HandleFunc("/deploy", deployHandler)
	fmt.Println("[Orchestrator] Listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
