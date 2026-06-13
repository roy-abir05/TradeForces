#!/bin/bash

# Define color codes for clean terminal output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}[TradeForces DX] Initializing boot sequence...${NC}"

# 1. Boot the heavy infrastructure via Docker
echo -e "${GREEN}[1/4] Spinning up Redpanda, PostgreSQL, and Grafana...${NC}"
docker compose up -d

# Wait for databases to accept connections
sleep 3

# 2. Start the Telemetry Ingester (Background)
echo -e "${GREEN}[2/4] Booting Actor-Model Telemetry Ingester...${NC}"
cd ./telemetry-ingester && go run . &
INGESTER_PID=$!

# 3. Start the Go Orchestrator (Background)
echo -e "${GREEN}[3/4] Booting Docker Sandbox Orchestrator...${NC}"
cd ./orchestrator && go run . &
ORCHESTRATOR_PID=$!

# 4. Start the Next.js Frontend (Background)
echo -e "${GREEN}[4/4] Booting Next.js Ingress Firewall & UI...${NC}"
cd ./web && npm run dev &
NEXT_PID=$!

echo -e "\n${BLUE}===================================================${NC}"
echo -e "${GREEN}🚀 TRADEFORCES IS LIVE${NC}"
echo -e "UI: http://localhost:3000"
echo -e "Grafana: http://localhost:3001"
echo -e "WebSocket: ws://localhost:8081/ws"
echo -e "${BLUE}===================================================${NC}"
echo -e "${RED}Press Ctrl+C to safely shutdown all services.${NC}"

# Cleanup Trap: Catches Ctrl+C and gracefully kills all background processes
cleanup() {
    echo -e "\n${RED}[TradeForces DX] Initiating shutdown sequence...${NC}"
    echo "Stopping Next.js (PID: $NEXT_PID)..."
    kill $NEXT_PID
    echo "Stopping Orchestrator (PID: $ORCHESTRATOR_PID)..."
    kill $ORCHESTRATOR_PID
    echo "Stopping Telemetry Ingester (PID: $INGESTER_PID)..."
    kill $INGESTER_PID
    echo "Shutting down Docker infrastructure..."
    docker compose down
    echo -e "${GREEN}Shutdown complete. Goodbye!${NC}"
    exit 0
}

trap cleanup SIGINT SIGTERM

# Wait indefinitely to keep the script running and catch the termination signal
wait