# TradeForces

> A Distributed Online Judge and Remote Code Execution Environment for High-Frequency Trading Engines.

TradeForces is a specialized Online Judge built for the IICPC Summer Trading Hackathon. While traditional competitive programming platforms evaluate algorithms for basic time and space complexity, TradeForces is designed to evaluate distributed system limits, lock-free concurrency, and deterministic latency under extreme network load.

It provides an isolated Remote Code Execution (RCE) sandbox to benchmark C++ matching engines, streaming microsecond-level telemetry to calculate an Exchange Reliability Score.

## System Architecture

The platform is divided into four primary microservices, built using Next.js, Go, and Kafka (Redpanda).

### 1. API Gateway & UI (`web`)

Built with **Next.js**, **Prisma**, and **PostgreSQL**. It serves as the primary gateway for users to submit their C++ code. It manages the PostgreSQL database state (PENDING, SUCCESS, MLE, TLE, RE) and hosts the live telemetry dashboard.

### 2. The Orchestrator (`orchestrator`)

A **Go** service utilizing the Docker API SDK. It acts as the RCE lifecycle manager and resource observer.

- **Sandboxing:** Compiles user submissions (`g++ -O3`) inside an isolated Docker bridge network.
- **Resource Limits:** Enforces strict `cgroup` limitations (128MB Memory, 128MB MemorySwap) to prevent OS thrashing.
- **Event-Driven Observer:** Monitors container exit codes to accurately report competitive programming verdicts:
  - `MLE` (Memory Limit Exceeded): Caught via Docker OOM Killer (Exit Code 137).
  - `RE` (Runtime Error): Caught via unhandled exceptions/segfaults.
  - `TLE` (Time Limit Exceeded): Caught via a strict 60-second execution timeout.

### 3. Load Generator (`load-generator`)

A highly concurrent **Go** service deployed dynamically by the Orchestrator. It connects to the compiled C++ engine over a private Docker network, spawning hundreds of goroutine workers to flood the matching engine with TCP orders. It streams raw microsecond latency timestamps directly into a Redpanda message queue.

### 4. Telemetry Ingester (`telemetry-ingester`)

A **Go** microservice that acts as the core evaluation engine.

- **Kafka Consumer:** Consumes the raw telemetry stream from the Load Generator.
- **Live Feed:** Calculates rolling 1-second metrics (p50, p90, p99, TPS) and broadcasts them to the Next.js frontend via WebSockets.
- **Final Scoring:** Upon receiving an `END_OF_RUN` signal, it calculates the engine's final Exchange Reliability Score and posts the payload to the Next.js API Gateway.

## The Exchange Reliability Score (ERS)

In high-frequency trading, median latency (p50) is a vanity metric; engines are judged on their worst-case scenarios and jitter. TradeForces scores submissions based on:

1.  **Throughput (TPS):** Total orders processed per second.
2.  **Tail Latency (p99):** Ensuring the slowest 1% of orders remain highly performant.
3.  **Coefficient of Variation (CV):** Calculated as the standard deviation divided by the mean ($\frac{\sigma}{\mu}$). This explicitly measures latency stability, penalizing engines that rely on unpredictable locking mechanisms instead of deterministic, lock-free data structures.

## Installation & Setup

### Prerequisites

- Docker & Docker Compose
- Go 1.21+
- Node.js 18+
- Redpanda (Kafka compatible broker)

### 1. Infrastructure Setup

Start the Redpanda broker and PostgreSQL database using Docker Compose.

```bash
docker-compose up -d
```

### 2. Start the Telemetry Ingester

```bash
cd telemetry-ingester
go mod tidy
go run main.go
```

### 3. Start the API Gateway (Next.js)

Apply database migrations and start the UI.

```bash
cd web
npm install
npx prisma db push
npm run dev
```

### 4. Start the Orchestrator

Ensure the Docker daemon is running, as the Orchestrator requires API access to spin up the RCE sandboxes.

```bash
cd orchestrator
go mod tidy
go run main.go
```

## Testing the Evaluation Pipeline

Users can submit code through the Next.js frontend running at `http://localhost:3000`.

To verify the Observer's strict resource monitoring, the following minimal C++ submissions will trigger specific verdicts:

**Triggering TLE (Time Limit Exceeded):**

```C++
#include <sys/socket.h>
#include <netinet/in.h>

int main() {
    int server_fd = socket(AF_INET, SOCK_STREAM, 0);
    struct sockaddr_in address;
    address.sin_family = AF_INET;
    address.sin_addr.s_addr = INADDR_ANY;
    address.sin_port = htons(1337);
    bind(server_fd, (struct sockaddr *)&address, sizeof(address));
    listen(server_fd, 3);

    while(true) {} // Infinite loop triggers 60s timeout
    return 0;
}
```

**Triggering MLE (Memory Limit Exceeded):**

```C++
#include <sys/socket.h>
#include <netinet/in.h>

int main() {
    int server_fd = socket(AF_INET, SOCK_STREAM, 0);
    struct sockaddr_in address;
    address.sin_family = AF_INET;
    address.sin_addr.s_addr = INADDR_ANY;
    address.sin_port = htons(1337);
    bind(server_fd, (struct sockaddr *)&address, sizeof(address));
    listen(server_fd, 3);

    while(true) {
        // volatile prevents GCC -O3 from optimizing the unread memory leak
        volatile char* p = new char[100000];
        for(int i = 0; i < 100000; i += 4096) {
            p[i] = 'X';
        }
    }
    return 0;
}
```

## Hackathon Submission Details

**Event:** IICPC Summer Trading Hackathon

<!--
**Team:** [Your Team Name]

**Documentation:** [Link to Google Drive Design Document]

**Video Demo:** [Link to YouTube/Drive Video]
-->
