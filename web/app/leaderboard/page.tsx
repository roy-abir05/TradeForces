"use client";

import { useState, useEffect } from "react";

interface LeaderboardEntry {
  id: string;
  tps: number;
  p99: number;
  submission: {
    user: { name: string; image: string };
  };
}

export default function LeaderboardPage() {
  const [leaderboard, setLeaderboard] = useState<LeaderboardEntry[]>([]);

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
    const interval = setInterval(fetchLeaderboard, 10000); // Benefiting from ISR cache
    return () => clearInterval(interval);
  }, []);

  return (
    <main className="min-h-screen bg-black text-zinc-300 font-sans p-8 flex flex-col gap-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tighter text-white">
          GLOBAL EXECUTION RANKS
        </h1>
        <p className="text-xs text-zinc-500 tracking-widest uppercase mt-1">
          Live Deterministic Benchmarking Hall of Fame
        </p>
      </div>

      <div className="bg-zinc-950 border border-zinc-800 p-6 rounded-sm mt-4">
        {leaderboard.length === 0 ? (
          <div className="text-center text-zinc-600 py-8 font-mono text-sm">
            Data tape is empty.
          </div>
        ) : (
          <div className="flex flex-col">
            <div className="grid grid-cols-12 text-[10px] text-zinc-600 uppercase tracking-widest pb-3 px-2">
              <div className="col-span-1">Rnk</div>
              <div className="col-span-5">Operator</div>
              <div className="col-span-3 text-right">Throughput</div>
              <div className="col-span-3 text-right">p99 Latency</div>
            </div>
            <div className="flex flex-col gap-1">
              {leaderboard.map((entry, index) => (
                <div
                  key={entry.id}
                  className="grid grid-cols-12 items-center bg-black border border-zinc-900 p-3 rounded-sm hover:border-zinc-700 transition-colors"
                >
                  <div className="col-span-1 font-mono text-xs text-zinc-500">
                    {index + 1}
                  </div>
                  <div className="col-span-5 flex items-center gap-3">
                    <img
                      src={entry.submission.user.image || "/placeholder.png"}
                      alt="avatar"
                      className="w-5 h-5 rounded-sm opacity-80"
                    />
                    <span className="text-white text-sm font-medium">
                      {entry.submission.user.name}
                    </span>
                  </div>
                  <div className="col-span-3 text-right font-mono text-emerald-400 text-sm">
                    {entry.tps.toLocaleString()} req/s
                  </div>
                  <div className="col-span-3 text-right font-mono text-zinc-300 text-sm">
                    {entry.p99}µs
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </main>
  );
}
