"use client";

import { useState, useEffect } from "react";

export default function Home() {
  const [file, setFile] = useState<File | null>(null);
  const [status, setStatus] = useState<string>("Waiting for engine.cpp...");
  const [metrics, setMetrics] = useState({ tps: 0, p50: 0, p90: 0, p99: 0 });

  // The Polling Loop
  useEffect(() => {
    const interval = setInterval(async () => {
      try {
        const res = await fetch("/api/live");
        if (res.ok) {
          const data = await res.json();
          setMetrics(data);
        }
      } catch (error) {
        console.error("Failed to fetch live metrics");
      }
    }, 1000); // Poll every 1 second

    return () => clearInterval(interval);
  }, []);

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files[0]) {
      setFile(e.target.files[0]);
      setStatus(`Ready to deploy: ${e.target.files[0].name}`);
    }
  };

  const handleDeploy = async () => {
    if (!file) return;
    setStatus("Uploading to TradeForces Orchestrator...");
    const formData = new FormData();
    formData.append("engine", file);

    try {
      const response = await fetch("/api/deploy", {
        method: "POST",
        body: formData,
      });
      if (response.ok)
        setStatus(
          "SUCCESS: Engine deployed to Sandbox. Attack Fleet standing by.",
        );
      else setStatus("FAILED: Orchestrator rejected the payload.");
    } catch (error) {
      setStatus("FAILED: Network error.");
    }
  };

  return (
    <main className="min-h-screen bg-neutral-950 text-green-400 font-mono p-8 flex flex-col items-center justify-center gap-8">
      {/* UPLOAD PORTAL */}
      <div className="max-w-2xl w-full border border-green-800 bg-black p-8 rounded-lg shadow-[0_0_15px_rgba(0,255,0,0.1)]">
        <h1 className="text-3xl font-bold mb-2 tracking-tighter">
          TRADEFORCES // PLATFORM
        </h1>

        <div className="mb-8 mt-6">
          <input
            type="file"
            accept=".cpp"
            onChange={handleFileChange}
            className="block w-full text-sm text-neutral-400 file:mr-4 file:py-2 file:px-4 file:bg-green-900 file:text-green-400 hover:file:bg-green-800 cursor-pointer border border-neutral-800 rounded bg-neutral-900 p-2"
          />
        </div>

        <button
          onClick={handleDeploy}
          disabled={!file}
          className={`w-full py-3 font-bold tracking-widest transition-all ${file ? "bg-green-600 text-black hover:bg-green-500 hover:shadow-[0_0_20px_rgba(0,255,0,0.4)]" : "bg-neutral-800 text-neutral-600 cursor-not-allowed"}`}
        >
          DEPLOY ENGINE
        </button>
        <div className="mt-4 text-sm text-neutral-500">{status}</div>
      </div>

      {/* LIVE LEADERBOARD DASHBOARD */}
      <div className="max-w-2xl w-full border border-neutral-800 bg-black p-8 rounded-lg">
        <div className="flex justify-between items-center mb-6">
          <h2 className="text-xl font-bold text-white">
            LIVE TELEMETRY (POLLING)
          </h2>
        </div>

        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          <MetricCard title="TPS" value={metrics.tps} unit="req/s" />
          <MetricCard title="p50 Latency" value={metrics.p50} unit="µs" />
          <MetricCard title="p90 Latency" value={metrics.p90} unit="µs" />
          <MetricCard title="p99 Latency" value={metrics.p99} unit="µs" />
        </div>
      </div>
    </main>
  );
}

function MetricCard({
  title,
  value,
  unit,
}: {
  title: string;
  value: number;
  unit: string;
}) {
  return (
    <div className="bg-neutral-900 border border-neutral-800 p-4 rounded flex flex-col items-center justify-center">
      <span className="text-neutral-500 text-xs mb-2 tracking-widest">
        {title}
      </span>
      <div className="flex items-baseline gap-1">
        <span className="text-3xl font-bold text-white">
          {value === 0 ? "--" : value}
        </span>
        <span className="text-green-600 text-sm">{unit}</span>
      </div>
    </div>
  );
}
