package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

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

	hostPath := "/home/abir/Development/demo_iicpc/submissions/user_123"
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
}

func main() {
	http.HandleFunc("/deploy", deployHandler)
	fmt.Println("[Orchestrator] Microservice listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
