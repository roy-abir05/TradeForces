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
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func deployHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := context.Background()

	apiClient, err := client.New(client.FromEnv)
	if err != nil {
		log.Fatalf("Failed to connect to Docker: %v", err)
	}
	defer apiClient.Close()

	fmt.Println("[Orchestrator] Connected to Docker Daemon successfully.")

	fmt.Println("[Orchestrator] Pulling gcc:latest image (this might take a few minutes)...")
	reader, err := apiClient.ImagePull(ctx, "docker.io/library/gcc:latest", client.ImagePullOptions{})
	if err != nil {
		log.Fatalf("Image pull failed: %v", err)
	}
	defer reader.Close()

	io.Copy(os.Stdout, reader)

	hostPath := "/home/abir/Development/TradeForces/orchestrator/submissions/user_123"
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
		log.Fatalf("JSON Parse Error: %v", err)
	}

	fmt.Println("[Orchestrator] Creating Sandboxed Container...")
	resp, err := apiClient.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     &engineConfig,
		HostConfig: &sandboxConfig,
	})
	if err != nil {
		log.Fatalf("Failed to create container: %v", err)
	}

	fmt.Println("[Orchestrator] Booting the Cage...")
	_, err = apiClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{})
	if err != nil {
		log.Fatalf("Failed to start container: %v", err)
	}

	fmt.Printf("[Orchestrator] SUCCESS! Engine is running in Sandbox ID: %s\n", resp.ID[:12])

	go func() {
		targetAddress := "localhost:1337"
		fmt.Printf("[Orchestrator] Polling %s to detect Engine boot...\n", targetAddress)

		// The Health Check Polling
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
			fmt.Println("[Orchestrator] FATAL: Engine failed to bind to port 1337 within 5 seconds. (Compilation Error?)")

			fmt.Println("\n--- CAGE LOGS ---")
			logCmd := exec.Command("docker", "logs", resp.ID)
			logCmd.Stdout = os.Stdout
			logCmd.Stderr = os.Stderr
			logCmd.Run()
			fmt.Println("-----------------\n")

			cleanupCmd := exec.Command("docker", "rm", "-f", resp.ID)
			if err := cleanupCmd.Run(); err != nil {
				fmt.Printf("[Orchestrator] Warning: Failed to clean up container: %v\n", err)
			} else {
				fmt.Println("[Orchestrator] Sandbox destroyed successfully.")
			}
			return
		}

		fmt.Println("[Orchestrator] Target Lock Acquired. Unleashing the Attack Fleet...")

		cmd := exec.Command("go", "run", "main.go")
		cmd.Dir = "../load-generator"
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err := cmd.Run()
		if err != nil {
			fmt.Printf("[Orchestrator] Attack Fleet failed: %v\n", err)
		} else {
			fmt.Println("[Orchestrator] Attack complete.")
		}

		fmt.Println("[Orchestrator] Terminating Sandbox...")
		cleanupCmd := exec.Command("docker", "rm", "-f", resp.ID)
		if err := cleanupCmd.Run(); err != nil {
			fmt.Printf("[Orchestrator] Warning: Failed to clean up container: %v\n", err)
		} else {
			fmt.Println("[Orchestrator] Sandbox destroyed successfully.")
		}
	}()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Deployed and Attack Initiated"))
}

func main() {
	http.HandleFunc("/deploy", deployHandler)
	fmt.Println("[Orchestrator] Microservice listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
