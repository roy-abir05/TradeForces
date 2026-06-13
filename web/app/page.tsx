"use client";

import { useState, useEffect } from "react";
import { signIn, signOut, useSession } from "next-auth/react";
import {
  ComposedChart,
  Line,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import DocsModal from "@/components/DocsModal";

interface LeaderboardEntry {
  id: string;
  tps: number;
  p99: number;
  submission: {
    user: { name: string; image: string };
  };
}

interface MySubmission {
  id: string;
  status: string;
  createdAt: string;
  result: {
    tps: number;
    p50: number;
    p90: number;
    p99: number;
  } | null;
}

interface ChartDataPoint {
  time: string;
  tps: number;
  p99: number;
}

export default function Home() {
  const { data: session, status } = useSession();

  const [file, setFile] = useState<File | null>(null);
  const [deployStatus, setDeployStatus] = useState<string>(
    "AWAITING ENGINE PAYLOAD",
  );
  const [metrics, setMetrics] = useState({ tps: 0, p50: 0, p90: 0, p99: 0 });
  const [chartData, setChartData] = useState<ChartDataPoint[]>([]);
  const [activeSubmissionId, setActiveSubmissionId] = useState<string | null>(
    null,
  );

  const [isOffline, setIsOffline] = useState<boolean>(false);

  const [leaderboard, setLeaderboard] = useState<LeaderboardEntry[]>([]);
  const [myHistory, setMyHistory] = useState<MySubmission[]>([]);

  // The Direct WebSocket Connection to Go Hub
  useEffect(() => {
    if (!activeSubmissionId) return;

    const currentRun = myHistory.find((sub) => sub.id === activeSubmissionId);
    if (
      currentRun &&
      (currentRun.status === "SUCCESS" || currentRun.status === "FAILED")
    ) {
      console.log("[UI] Run complete. Freezing telemetry board.");
      return;
    }

    const ws = new WebSocket("ws://localhost:8081/ws");
    let isNormalClose = false; // NEW: Track intentional closures

    ws.onopen = () => {
      setIsOffline(false); // NEW: Reset offline status on reconnect
      ws.send(
        JSON.stringify({
          action: "subscribe",
          submission_id: activeSubmissionId,
        }),
      );
    };

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        const now = new Date().toLocaleTimeString("en-US", {
          hour12: false,
          hour: "2-digit",
          minute: "2-digit",
          second: "2-digit",
        });

        const newPoint = { time: now, tps: data.tps, p99: data.p99 };

        if (data.status === "COMPLETE") {
          isNormalClose = true; // NEW: Mark closure as intentional
          setMetrics({
            tps: data.tps,
            p50: data.p50,
            p90: data.p90,
            p99: data.p99,
          });
          setChartData((prev) => [...prev.slice(-59), newPoint]);
          ws.close();
          setDeployStatus("SYSTEM LOCKED: BENCHMARK COMPLETE");
          return;
        }

        setMetrics({
          tps: data.tps,
          p50: data.p50,
          p90: data.p90,
          p99: data.p99,
        });
        setChartData((prev) => [...prev.slice(-59), newPoint]);
      } catch (err) {
        console.error("[WebSocket] Parse Error:", err);
      }
    };

    ws.onerror = (err) => {
      console.error("[WebSocket] Connection Error:", err);
      setIsOffline(true); // NEW: Trigger offline state
    };

    ws.onclose = () => {
      console.log("[WebSocket] Connection Closed.");
      // NEW: If it closed but wasn't COMPLETE, the server dropped.
      if (!isNormalClose) {
        setIsOffline(true);
        setDeployStatus("FATAL: WEBSOCKET DISCONNECT");
      }
    };

    return () => {
      isNormalClose = true; // Prevent unmount from triggering error state
      ws.close();
    };
  }, [activeSubmissionId]);

  // Polling Loops
  useEffect(() => {
    const fetchLeaderboard = async () => {
      try {
        const res = await fetch("/api/leaderboard");
        if (res.ok) setLeaderboard(await res.json());
      } catch (error) {
        console.error("Failed to fetch leaderboard");
      }
    };
    fetchLeaderboard();
    const interval = setInterval(fetchLeaderboard, 10000);
    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    const fetchHistory = async () => {
      if (session?.user?.id) {
        try {
          const res = await fetch("/api/submissions");
          if (res.ok) setMyHistory(await res.json());
        } catch (error) {
          console.error("Failed to fetch history");
        }
      }
    };
    fetchHistory();
    const interval = setInterval(fetchHistory, 10000);
    return () => clearInterval(interval);
  }, [session, deployStatus]);

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files[0]) {
      // 500KB Size Limit Check
      if (e.target.files[0].size > 500 * 1024) {
        setDeployStatus("REJECTED: FILE EXCEEDS 50KB LIMIT");
        setFile(null);
        return;
      }

      setFile(e.target.files[0]);
      setDeployStatus(`READY: ${e.target.files[0].name.toUpperCase()}`);
    }
  };

  const handleDeploy = async () => {
    if (!file || !session) return;
    setDeployStatus("UPLOADING TO ORCHESTRATOR...");
    setMetrics({ tps: 0, p50: 0, p90: 0, p99: 0 });
    setChartData([]);
    setActiveSubmissionId(null);

    const formData = new FormData();
    formData.append("engine", file);
    formData.append("userId", session.user?.id as string);

    try {
      const response = await fetch("/api/deploy", {
        method: "POST",
        headers: {
          "x-user-id": session.user?.id as string,
        },
        body: formData,
      });
      if (response.ok) {
        const data = await response.json();
        if (data.submissionId) setActiveSubmissionId(data.submissionId);
        setDeployStatus("EXECUTING: TELEMETRY STREAM ACTIVE");
      } else {
        setDeployStatus("REJECTED: ORCHESTRATOR ERROR");
      }
    } catch (error) {
      setDeployStatus("FATAL: NETWORK DISCONNECT");
    }
  };

  // Global Keyboard Shortcut (The "Trader" Experience)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Listen for Ctrl+Enter (Windows/Linux) or Cmd+Enter (Mac)
      if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
        e.preventDefault();

        const isCurrentlyRunning =
          deployStatus.includes("EXECUTING") ||
          deployStatus.includes("UPLOADING");

        if (file && session && !isOffline && !isCurrentlyRunning) {
          handleDeploy();
        }
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [file, session, isOffline, deployStatus]);

  if (status === "loading") {
    return (
      <main className="min-h-screen bg-black text-white font-sans flex items-center justify-center">
        <div className="animate-pulse tracking-widest text-sm text-zinc-500">
          INITIALIZING SECURE SESSION...
        </div>
      </main>
    );
  }

  const isRunning = deployStatus.includes("EXECUTING");

  return (
    <main className="min-h-screen bg-black text-zinc-300 font-sans p-4 md:p-8 flex flex-col gap-8 selection:bg-zinc-800">
      {/* OFFLINE TOAST */}
      {isOffline && (
        <div className="fixed top-4 right-4 z-50 bg-rose-950 border border-rose-900 text-rose-400 px-4 py-2 rounded-sm text-xs font-mono tracking-widest flex items-center gap-2 shadow-xl animate-pulse">
          <div className="w-2 h-2 rounded-full bg-rose-500"></div>
          Reconnecting to TradeForces Hub...
        </div>
      )}
      {/* HEADER NAVIGATION */}
      <header className="flex justify-between items-end border-b border-zinc-800 pb-4">
        <div>
          <h1 className="text-2xl md:text-3xl font-bold tracking-tighter text-white">
            TRADEFORCES
          </h1>
          <p className="text-xs text-zinc-500 tracking-widest uppercase mt-1">
            Institutional Execution Benchmark
          </p>
        </div>
        <div className="flex items-center gap-4">
          <DocsModal />
          <a
            href="/leaderboard"
            className="text-xs text-zinc-400 hover:text-white transition-colors uppercase tracking-wider font-mono"
          >
            [ View Full Leaderboard ]
          </a>
          {session ? (
            <div className="flex items-center gap-4">
              <div className="text-right hidden md:block">
                <div className="text-xs text-zinc-500 uppercase tracking-wider">
                  Operator
                </div>
                <div className="text-sm font-medium text-white">
                  {session.user?.name}
                </div>
              </div>
              {session.user?.image && (
                <img
                  src={session.user.image}
                  alt="Avatar"
                  className="w-9 h-9 rounded bg-zinc-800"
                />
              )}
              <button
                onClick={() => signOut()}
                className="text-xs text-zinc-500 hover:text-white transition-colors uppercase tracking-wider ml-2 border-l border-zinc-800 pl-4"
              >
                Disconnect
              </button>
            </div>
          ) : (
            <button
              onClick={() => signIn("github")}
              className="bg-white text-black text-sm font-bold uppercase tracking-wider px-6 py-2 rounded-sm hover:bg-zinc-200 transition-colors"
            >
              Authorize GitHub
            </button>
          )}
        </div>
      </header>

      <div className="grid grid-cols-1 lg:grid-cols-12 gap-8 items-start">
        {/* LEFT COLUMN: CONTROLS & HISTORY */}
        <div className="lg:col-span-4 flex flex-col gap-8">
          {/* UPLOAD PORTAL */}
          <section className="bg-zinc-950 border border-zinc-800 p-6 rounded-sm">
            <h2 className="text-xs text-zinc-500 tracking-widest uppercase mb-6 border-b border-zinc-800 pb-2">
              Deployment Control
            </h2>

            {!session ? (
              <div className="text-center py-8 text-zinc-600 text-sm border border-dashed border-zinc-800 rounded-sm">
                Authorization required to access execution plane.
              </div>
            ) : (
              <div className="flex flex-col gap-4">
                <div className="relative group">
                  <input
                    type="file"
                    accept=".cpp"
                    onChange={handleFileChange}
                    className="absolute inset-0 w-full h-full opacity-0 cursor-pointer z-10"
                  />
                  <div className="border border-zinc-800 bg-black group-hover:border-zinc-600 transition-colors rounded-sm p-4 text-center">
                    <span className="text-sm text-zinc-400 font-mono">
                      {file ? file.name : "[ SELECT ENGINE.CPP ]"}
                    </span>
                  </div>
                </div>

                <button
                  onClick={handleDeploy}
                  disabled={!file || isOffline}
                  className={`w-full flex items-center justify-center gap-2 py-3 text-sm font-bold tracking-widest uppercase rounded-sm transition-all ${
                    file && !isOffline
                      ? "bg-white text-black hover:bg-zinc-200"
                      : "bg-zinc-900 text-zinc-600 cursor-not-allowed"
                  }`}
                >
                  <span>
                    {isOffline ? "SYSTEM OFFLINE" : "Initiate Attack"}
                  </span>
                  {!isOffline && file && (
                    <span className="text-[10px] bg-zinc-200/50 text-black px-1.5 py-0.5 rounded-sm font-mono ml-2">
                      Ctrl/⌘ + ↵
                    </span>
                  )}
                </button>

                <div className="flex items-center gap-2 mt-2">
                  <div
                    className={`w-2 h-2 rounded-full ${isRunning ? "bg-amber-500 animate-pulse" : deployStatus.includes("SUCCESS") ? "bg-emerald-500" : "bg-zinc-700"}`}
                  ></div>
                  <div className="text-xs text-zinc-500 font-mono tracking-wider">
                    {deployStatus}
                  </div>
                </div>
              </div>
            )}
          </section>

          {/* MY HISTORY */}
          {session && (
            <section className="bg-zinc-950 border border-zinc-800 p-6 rounded-sm">
              <h2 className="text-xs text-zinc-500 tracking-widest uppercase mb-4 border-b border-zinc-800 pb-2">
                My Operations
              </h2>

              {myHistory.length === 0 ? (
                <div className="text-center text-zinc-600 py-4 text-sm font-mono">
                  No telemetry on record.
                </div>
              ) : (
                <div className="flex flex-col gap-2 max-h-80 overflow-y-auto pr-2 custom-scrollbar">
                  {myHistory.map((sub) => (
                    <HistoryCard key={sub.id} sub={sub} />
                  ))}
                </div>
              )}
            </section>
          )}
        </div>

        {/* RIGHT COLUMN: LIVE TELEMETRY & GLOBAL LEADERBOARD */}
        <div className="lg:col-span-8 flex flex-col gap-8">
          {/* LIVE TELEMETRY DASHBOARD */}
          <section className="bg-zinc-950 border border-zinc-800 p-6 rounded-sm">
            <div className="flex justify-between items-center mb-6 border-b border-zinc-800 pb-2">
              <h2 className="text-xs text-zinc-500 tracking-widest uppercase">
                Live Telemetry Feed
              </h2>
              {isRunning && (
                <span className="text-xs font-mono text-amber-500 animate-pulse">
                  STREAMING DATA
                </span>
              )}
            </div>

            {/* METRIC CARDS */}
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-8">
              <MetricCard
                title="TPS"
                value={metrics.tps}
                unit="req/s"
                highlight
              />
              <MetricCard title="p50 Latency" value={metrics.p50} unit="µs" />
              <MetricCard title="p90 Latency" value={metrics.p90} unit="µs" />
              <MetricCard
                title="p99 Latency"
                value={metrics.p99}
                unit="µs"
                highlight
              />
            </div>

            {/* LIVE GRAPH */}
            <div className="h-64 w-full bg-black border border-zinc-900 p-4 rounded-sm relative">
              {chartData.length === 0 ? (
                <div className="absolute inset-0 flex items-center justify-center text-zinc-700 font-mono text-xs tracking-widest">
                  AWAITING DATAPOINTS...
                </div>
              ) : (
                <ResponsiveContainer width="100%" height="100%">
                  <ComposedChart
                    data={chartData}
                    margin={{ top: 5, right: 0, left: 0, bottom: 0 }}
                  >
                    <CartesianGrid
                      strokeDasharray="3 3"
                      stroke="#27272a"
                      vertical={false}
                    />
                    <XAxis
                      dataKey="time"
                      stroke="#52525b"
                      fontSize={10}
                      tickMargin={10}
                      minTickGap={30}
                    />
                    <YAxis
                      yAxisId="left"
                      stroke="#52525b"
                      fontSize={10}
                      tickFormatter={(val) => `${(val / 1000).toFixed(1)}k`}
                    />
                    <YAxis
                      yAxisId="right"
                      orientation="right"
                      stroke="#52525b"
                      fontSize={10}
                    />
                    <Tooltip
                      contentStyle={{
                        backgroundColor: "#09090b",
                        borderColor: "#27272a",
                        borderRadius: "2px",
                      }}
                      itemStyle={{ fontFamily: "monospace", fontSize: "12px" }}
                      labelStyle={{
                        color: "#a1a1aa",
                        fontSize: "10px",
                        marginBottom: "4px",
                      }}
                    />
                    <Area
                      yAxisId="left"
                      type="stepAfter"
                      dataKey="tps"
                      fill="#059669"
                      fillOpacity={0.1}
                      stroke="#10b981"
                      strokeWidth={2}
                      isAnimationActive={false}
                    />
                    <Line
                      yAxisId="right"
                      type="monotone"
                      dataKey="p99"
                      stroke="#e4e4e7"
                      strokeWidth={2}
                      dot={false}
                      isAnimationActive={false}
                    />
                  </ComposedChart>
                </ResponsiveContainer>
              )}
            </div>
          </section>

          {/* GLOBAL LEADERBOARD */}
          <section className="bg-zinc-950 border border-zinc-800 p-6 rounded-sm">
            <h2 className="text-xs text-zinc-500 tracking-widest uppercase mb-6 border-b border-zinc-800 pb-2">
              Global Execution Ranks
            </h2>

            {leaderboard.length === 0 ? (
              <div className="text-center text-zinc-600 py-8 text-sm font-mono">
                Data tape is empty.
              </div>
            ) : (
              <div className="flex flex-col">
                {/* Table Header */}
                <div className="grid grid-cols-12 text-[10px] text-zinc-600 uppercase tracking-widest pb-3 px-2">
                  <div className="col-span-1">Rnk</div>
                  <div className="col-span-5">Operator</div>
                  <div className="col-span-3 text-right">Throughput</div>
                  <div className="col-span-3 text-right">p99 Latency</div>
                </div>

                {/* Table Rows */}
                <div className="flex flex-col gap-1">
                  {leaderboard.map((entry, index) => (
                    <div
                      key={entry.id}
                      className="grid grid-cols-12 items-center bg-black border border-zinc-900 p-2 rounded-sm hover:border-zinc-700 transition-colors"
                    >
                      <div className="col-span-1 font-mono text-xs text-zinc-500">
                        {index + 1}
                      </div>
                      <div className="col-span-5 flex items-center gap-3">
                        {entry.submission.user.image ? (
                          <img
                            src={entry.submission.user.image}
                            alt="avatar"
                            className="w-5 h-5 rounded-sm opacity-80"
                          />
                        ) : (
                          <div className="w-5 h-5 rounded-sm bg-zinc-800"></div>
                        )}
                        <span className="text-white text-sm truncate">
                          {entry.submission.user.name || "Anonymous"}
                        </span>
                      </div>
                      <div className="col-span-3 text-right font-mono text-emerald-400 text-sm">
                        {entry.tps.toLocaleString()}
                      </div>
                      <div className="col-span-3 text-right font-mono text-zinc-300 text-sm">
                        {entry.p99}µs
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </section>
        </div>
      </div>
    </main>
  );
}

// Sub-components

function MetricCard({
  title,
  value,
  unit,
  highlight = false,
}: {
  title: string;
  value: number;
  unit: string;
  highlight?: boolean;
}) {
  return (
    <div className="bg-black border border-zinc-800 p-4 rounded-sm flex flex-col justify-between h-24">
      <span className="text-zinc-500 text-[10px] tracking-widest uppercase">
        {title}
      </span>
      <div className="flex items-baseline gap-1 mt-auto">
        <span
          className={`text-2xl font-mono ${highlight && value > 0 ? "text-white" : "text-zinc-300"}`}
        >
          {value === 0 ? "0.00" : value.toLocaleString()}
        </span>
        <span className="text-zinc-600 text-xs font-mono">{unit}</span>
      </div>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  let colorClass = "bg-zinc-900 text-zinc-400 border-zinc-800";

  if (status === "SUCCESS")
    colorClass = "bg-emerald-950/30 text-emerald-400 border-emerald-900/50";
  if (
    status === "FAILED" ||
    status === "MLE" ||
    status === "TLE" ||
    status === "RE"
  )
    colorClass = "bg-rose-950/30 text-rose-400 border-rose-900/50";
  if (status === "RUNNING" || status === "QUEUED")
    colorClass = "bg-amber-950/30 text-amber-400 border-amber-900/50";

  return (
    <span
      className={`px-2 py-[2px] text-[10px] font-mono border rounded-sm tracking-wider ${colorClass}`}
    >
      {status}
    </span>
  );
}

function HistoryCard({ sub }: { sub: MySubmission }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    if (!sub.result) return;

    // The Gamified Flex String
    const text = `🏆 TRADEFORCES GAUNTLET CLEARED 🏆
⏱️ Throughput: ${sub.result.tps.toLocaleString()} TPS
⚡ p99 Latency: ${sub.result.p99}µs

Can your engine survive the onslaught?`;

    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000); // Reset after 2 seconds
  };

  const isFailed = ["FAILED", "MLE", "TLE", "RE", "WA"].includes(sub.status);

  return (
    <div className="flex flex-col p-3 rounded-sm border border-zinc-800/50 bg-black hover:border-zinc-700 transition-colors gap-2">
      <div className="flex justify-between items-center">
        <span className="text-zinc-500 text-xs font-mono">
          {new Date(sub.createdAt).toLocaleTimeString()}
        </span>
        <StatusBadge status={sub.status} />
      </div>

      {sub.result && (
        <div className="flex justify-between items-end border-t border-zinc-900 pt-2 mt-1">
          <div>
            <div className="text-[10px] text-zinc-600 uppercase">TPS</div>
            <div className="text-sm text-white font-mono">
              {sub.result.tps.toLocaleString()}
            </div>
          </div>
          <div className="text-right">
            <div className="text-[10px] text-zinc-600 uppercase">p99</div>
            <div className="text-sm text-zinc-300 font-mono">
              {sub.result.p99}µs
            </div>
          </div>
        </div>
      )}

      {(sub.status === "SUCCESS" || isFailed) && (
        <div className="flex justify-end pt-2 border-t border-zinc-900/50 mt-1">
          {sub.status === "SUCCESS" && sub.result ? (
            <button
              onClick={handleCopy}
              className={`text-[10px] uppercase tracking-widest font-bold px-3 py-1.5 rounded-sm transition-colors flex items-center gap-1 ${
                copied
                  ? "bg-emerald-950/50 text-emerald-400 border border-emerald-900/50"
                  : "bg-zinc-900 hover:bg-zinc-800 text-zinc-300"
              }`}
            >
              {copied ? "COPIED TO CLIPBOARD ✓" : "COPY METRICS"}
            </button>
          ) : isFailed ? (
            <button
              onClick={() => {
                // NEW: Context-Aware Log Routing
                if (sub.status === "WA") {
                  alert(
                    "❌ MOCK SANDBOX LOGS:\n\n[FATAL] Correctness Violation.\n[ERROR] Engine failed deterministic order book audit.\nExpected strict Price-Time Priority fill sequence, received incorrect payload.",
                  );
                } else if (sub.status === "MLE") {
                  alert(
                    "⚠️ MOCK SANDBOX LOGS:\n\n[FATAL] Container crashed (Exit Code 137).\n[ERROR] Memory Limit Exceeded. Your engine leaked beyond the 512MB limit.",
                  );
                } else if (sub.status === "TLE") {
                  alert(
                    "⏱️ MOCK SANDBOX LOGS:\n\n[FATAL] Worker execution halted.\n[ERROR] Time Limit Exceeded. Engine froze or failed to process the load generator queue within the 5-minute absolute threshold.",
                  );
                } else {
                  alert(
                    "💥 MOCK SANDBOX LOGS:\n\n[FATAL] Process Terminated.\n[ERROR] Socket closed unexpectedly or segmentation fault occurred during execution.",
                  );
                }
              }}
              className="text-[10px] uppercase tracking-widest font-bold px-3 py-1.5 bg-rose-950/20 hover:bg-rose-900/40 text-rose-400 border border-rose-900/30 rounded-sm transition-colors"
            >
              VIEW LOGS
            </button>
          ) : null}
        </div>
      )}
    </div>
  );
}
