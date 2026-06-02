"use client";

import { useState } from "react";

export default function Home() {
  const [file, setFile] = useState<File | null>(null);
  const [status, setStatus] = useState<string>("Waiting for engine.cpp...");

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files[0]) {
      setFile(e.target.files[0]);
      setStatus(`Ready to deploy: ${e.target.files[0].name}`);
    }
  };

  const handleDeploy = async () => {
    if (!file) {
      setStatus("Error: No file selected.");
      return;
    }

    setStatus("Uploading to TradeForces Orchestrator...");
    const formData = new FormData();
    formData.append("engine", file);

    try {
      const response = await fetch("/api/deploy", {
        method: "POST",
        body: formData,
      });

      if (response.ok) {
        setStatus(
          "SUCCESS: Engine deployed to Sandbox. Attack Fleet standing by.",
        );
      } else {
        setStatus("FAILED: Orchestrator rejected the payload.");
      }
    } catch (error) {
      setStatus("FAILED: Network error.");
    }
  };

  return (
    <main className="min-h-screen bg-neutral-950 text-green-400 font-mono p-8 flex flex-col items-center justify-center">
      <div className="max-w-2xl w-full border border-green-800 bg-black p-8 rounded-lg shadow-[0_0_15px_rgba(0,255,0,0.1)]">
        <h1 className="text-3xl font-bold mb-2 tracking-tighter">
          TRADEFORCES // PLATFORM
        </h1>
        <p className="text-neutral-500 mb-8 border-b border-green-900 pb-4">
          High-Frequency Trading Engine Benchmarker
        </p>

        <div className="mb-8">
          <label className="block text-sm mb-2 text-neutral-400">
            Upload C++ Trading Engine
          </label>
          <input
            type="file"
            accept=".cpp"
            onChange={handleFileChange}
            className="block w-full text-sm text-neutral-400
              file:mr-4 file:py-2 file:px-4
              file:border-0 file:text-sm file:font-semibold
              file:bg-green-900 file:text-green-400
              hover:file:bg-green-800 cursor-pointer border border-neutral-800 rounded bg-neutral-900 p-2"
          />
        </div>

        <button
          onClick={handleDeploy}
          disabled={!file}
          className={`w-full py-3 font-bold tracking-widest transition-all ${
            file
              ? "bg-green-600 text-black hover:bg-green-500 hover:shadow-[0_0_20px_rgba(0,255,0,0.4)]"
              : "bg-neutral-800 text-neutral-600 cursor-not-allowed"
          }`}
        >
          DEPLOY ENGINE
        </button>

        <div className="mt-8 p-4 bg-neutral-900 border border-neutral-800 rounded min-h-[60px]">
          <span className="text-green-600 mr-2">{">"}</span>
          <span className="animate-pulse">{status}</span>
        </div>
      </div>
    </main>
  );
}
