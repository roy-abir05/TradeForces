"use client";

import { useState, useEffect } from "react";
import { signIn, signOut, useSession } from "next-auth/react";

interface LeaderboardEntry {
  id: string;
  tps: number;
  p99: number;
  submission: {
    user: { name: string; image: string };
  };
}

export default function Home() {
  const { data: session, status } = useSession();

  const [file, setFile] = useState<File | null>(null);
  const [deployStatus, setDeployStatus] = useState<string>(
    "Waiting for engine.cpp...",
  );
  const [metrics, setMetrics] = useState({ tps: 0, p50: 0, p90: 0, p99: 0 });

  const [leaderboard, setLeaderboard] = useState<LeaderboardEntry[]>([]);

  // The Polling Loop for Live Metrics
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
    }, 1000);

    return () => clearInterval(interval);
  }, []);

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files[0]) {
      setFile(e.target.files[0]);
      setDeployStatus(`Ready to deploy: ${e.target.files[0].name}`);
    }
  };

  // The Polling Loop for Leaderboard
  useEffect(() => {
    const fetchLeaderboard = async () => {
      try {
        const res = await fetch("/api/leaderboard");
        if (res.ok) {
          const data = await res.json();
          setLeaderboard(data);
        }
      } catch (error) {
        console.error("Failed to fetch leaderboard");
      }
    };

    fetchLeaderboard();
    const interval = setInterval(fetchLeaderboard, 10000); // Refresh every 10s
    return () => clearInterval(interval);
  }, []);

  const handleDeploy = async () => {
    if (!file || !session) return;
    setDeployStatus("Uploading to TradeForces Orchestrator...");
    const formData = new FormData();
    formData.append("engine", file);
    // We will soon pass the user ID here to save to the database!
    formData.append("userId", session.user?.id as string);

    try {
      const response = await fetch("/api/deploy", {
        method: "POST",
        body: formData,
      });
      if (response.ok)
        setDeployStatus(
          "SUCCESS: Engine deployed to Sandbox. Attack Fleet standing by.",
        );
      else setDeployStatus("FAILED: Orchestrator rejected the payload.");
    } catch (error) {
      setDeployStatus("FAILED: Network error.");
    }
  };

  if (status === "loading") {
    return (
      <main className="min-h-screen bg-neutral-950 text-green-400 font-mono flex items-center justify-center">
        Initializing Identity...
      </main>
    );
  }

  return (
    <main className="min-h-screen bg-neutral-950 text-green-400 font-mono p-8 flex flex-col items-center justify-center gap-8">
      {/* AUTH HEADER */}
      <div className="max-w-2xl w-full flex justify-end items-center gap-4">
        {session ? (
          <>
            <span className="text-neutral-400 text-sm">
              Operator: <span className="text-white">{session.user?.name}</span>
            </span>
            {session.user?.image && (
              <img
                src={session.user.image}
                alt="Avatar"
                className="w-8 h-8 rounded-full border border-green-600"
              />
            )}
            <button
              onClick={() => signOut()}
              className="text-xs text-red-500 hover:text-red-400 border border-red-900 px-3 py-1 rounded"
            >
              ABORT SESSION
            </button>
          </>
        ) : (
          <button
            onClick={() => signIn("github")}
            className="bg-white text-black font-bold px-6 py-2 rounded hover:bg-neutral-200 transition-colors"
          >
            AUTHORIZE GITHUB
          </button>
        )}
      </div>

      {/* UPLOAD PORTAL */}
      <div className="max-w-2xl w-full border border-green-800 bg-black p-8 rounded-lg shadow-[0_0_15px_rgba(0,255,0,0.1)]">
        <h1 className="text-3xl font-bold mb-2 tracking-tighter">
          TRADEFORCES // PLATFORM
        </h1>
        <p className="text-neutral-500 mb-8 border-b border-green-900 pb-4">
          General Software Engineering & Scalability Benchmark
        </p>

        {!session ? (
          <div className="text-center py-8 text-neutral-500 border border-dashed border-neutral-800 rounded">
            Awaiting Authorization. Please log in to deploy an engine.
          </div>
        ) : (
          <>
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
            <div className="mt-4 text-sm text-neutral-500">{deployStatus}</div>
          </>
        )}
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

      {/* GLOBAL LEADERBOARD */}
      <div className="max-w-2xl w-full border border-neutral-800 bg-black p-8 rounded-lg mt-8">
        <h2 className="text-xl font-bold text-white mb-6 border-b border-neutral-800 pb-2">TOP OPERATORS (GLOBAL)</h2>
        
        {leaderboard.length === 0 ? (
          <div className="text-center text-neutral-600 py-4 text-sm">No benchmark data available.</div>
        ) : (
          <div className="flex flex-col gap-2">
            {leaderboard.map((entry, index) => (
              <div key={entry.id} className="flex items-center justify-between bg-neutral-900 p-3 rounded border border-neutral-800 hover:border-green-900 transition-colors">
                <div className="flex items-center gap-4">
                  <span className={`font-bold w-6 text-center ${index === 0 ? 'text-yellow-400' : index === 1 ? 'text-gray-400' : index === 2 ? 'text-amber-600' : 'text-neutral-600'}`}>
                    #{index + 1}
                  </span>
                  {entry.submission.user.image ? (
                    <img src={entry.submission.user.image} alt="avatar" className="w-6 h-6 rounded-full" />
                  ) : (
                    <div className="w-6 h-6 rounded-full bg-neutral-700"></div>
                  )}
                  <span className="text-white text-sm">{entry.submission.user.name || "Anonymous"}</span>
                </div>
                
                <div className="flex items-center gap-6 text-sm">
                  <div className="text-right">
                    <span className="text-neutral-500 text-xs mr-2">TPS</span>
                    <span className="text-green-400 font-bold">{entry.tps.toLocaleString()}</span>
                  </div>
                  <div className="text-right w-20">
                    <span className="text-neutral-500 text-xs mr-2">p99</span>
                    <span className="text-white">{entry.p99}µs</span>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
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
