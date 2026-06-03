import { NextResponse } from "next/server";
import WebSocket from "ws";

declare global {
  var _wsClient: WebSocket | undefined;
  var _cachedMetrics: unknown;
}

if (!globalThis._cachedMetrics) {
  globalThis._cachedMetrics = { tps: 0, p50: 0, p90: 0, p99: 0 };
}

if (!globalThis._wsClient) {
  console.log("[Next.js Backend] Connecting to Go Referee WebSocket...");
  const ws = new WebSocket("ws://localhost:8081/ws");

  ws.on("open", () => {
    console.log("[Next.js Backend] Locked into Go Telemetry Feed.");
  });

  ws.on("message", (data) => {
    globalThis._cachedMetrics = JSON.parse(data.toString());
  });

  ws.on("error", (err) => console.error("[Next.js Backend] WS Error:", err));

  globalThis._wsClient = ws;
}

export async function GET() {
  return NextResponse.json(globalThis._cachedMetrics);
}
