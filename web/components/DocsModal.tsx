"use client";

import { useState } from "react";

export default function DocsModal() {
  const [isOpen, setIsOpen] = useState(false);
  const [copied, setCopied] = useState(false);

  const cppBoilerplate = `#include <iostream>
#include <string>
#include <netinet/in.h>
#include <unistd.h>
#include <cstring>

#define PORT 1337

int main() {
    int server_fd = socket(AF_INET, SOCK_STREAM, 0);
    int opt = 1;
    setsockopt(server_fd, SOL_SOCKET, SO_REUSEADDR | SO_REUSEPORT, &opt, sizeof(opt));

    sockaddr_in address;
    address.sin_family = AF_INET;
    address.sin_addr.s_addr = INADDR_ANY;
    address.sin_port = htons(PORT);

    if (bind(server_fd, (struct sockaddr*)&address, sizeof(address)) < 0) {
        std::cerr << "Bind failed" << std::endl;
        return 1;
    }
    
    listen(server_fd, SOMAXCONN);
    std::cout << "[Engine] Listening on TCP Port 1337..." << std::endl;

    char buffer[256];
    const char* ack = "ACK\\n";

    while (true) {
        int client_sock = accept(server_fd, nullptr, nullptr);
        if (client_sock < 0) continue;

        while (true) {
            int bytes_read = read(client_sock, buffer, sizeof(buffer) - 1);
            if (bytes_read <= 0) {
                close(client_sock);
                break; // Client disconnected
            }
            
            buffer[bytes_read] = '\\0';
            std::string input(buffer);
            
            // TODO: Implement your Limit Order Book logic here.
            // Parse the 'input' string, match orders, and generate the correct FILL or ACK.
            
            // For now, just send a basic ACK to pass the connection test
            write(client_sock, ack, strlen(ack));
        }
    }
    return 0;
}`;

  const handleCopy = () => {
    navigator.clipboard.writeText(cppBoilerplate);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <>
      <button
        onClick={() => setIsOpen(true)}
        className="text-xs text-zinc-400 hover:text-white transition-colors uppercase tracking-wider font-mono border border-zinc-800 hover:border-zinc-600 px-3 py-1.5 rounded-sm bg-black"
      >
        [ SPEC DOCS ]
      </button>

      {isOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm">
          <div className="bg-zinc-950 border border-zinc-800 rounded-sm w-full max-w-4xl max-h-[90vh] overflow-y-auto shadow-2xl flex flex-col">
            {/* Modal Header */}
            <div className="sticky top-0 bg-zinc-950 border-b border-zinc-800 p-4 flex justify-between items-center z-10">
              <div>
                <h2 className="text-xl font-bold tracking-tighter text-white">TRADEFORCES ENGINE SPECIFICATION</h2>
                <p className="text-[10px] text-zinc-500 tracking-widest uppercase mt-1">Integration & Protocol Guide</p>
              </div>
              <button
                onClick={() => setIsOpen(false)}
                className="text-zinc-500 hover:text-white text-2xl leading-none font-light px-2"
              >
                &times;
              </button>
            </div>

            {/* Modal Body */}
            <div className="p-6 text-sm text-zinc-300 space-y-8 font-sans">
              
              <section>
                <p className="text-zinc-400 mb-4">
                  Welcome to the TradeForces Gauntlet. Your task is to build a high-performance matching engine capable of surviving extreme network loads. To ensure the Orchestrator can communicate with your engine, you must strictly adhere to the following protocols.
                </p>
              </section>

              <section>
                <h3 className="text-emerald-400 font-mono uppercase tracking-wider text-xs mb-3 font-bold border-b border-zinc-800 pb-2">1. Network Connection</h3>
                <ul className="list-disc list-inside space-y-1 text-zinc-400 ml-1">
                  <li><span className="text-zinc-200">Host:</span> <code className="bg-zinc-900 px-1 py-0.5 rounded text-rose-300">0.0.0.0</code> (or INADDR_ANY)</li>
                  <li><span className="text-zinc-200">Port:</span> <code className="bg-zinc-900 px-1 py-0.5 rounded text-rose-300">1337</code></li>
                  <li><span className="text-zinc-200">Protocol:</span> Raw TCP (Do not use HTTP, WebSockets, or gRPC)</li>
                </ul>
              </section>

              <section>
                <h3 className="text-emerald-400 font-mono uppercase tracking-wider text-xs mb-3 font-bold border-b border-zinc-800 pb-2">2. Inbound Protocol (What we send)</h3>
                <p className="text-zinc-400 mb-2">The Load Generator blasts raw strings over the TCP socket, terminated by a newline character (<code className="text-rose-300">\n</code>). You must parse these instantly.</p>
                <ul className="space-y-2 font-mono text-xs">
                  <li className="bg-black border border-zinc-900 p-2 rounded-sm"><span className="text-zinc-500 mr-2">Buy:</span> <span className="text-blue-400">BUY &lt;qty&gt; &lt;price&gt; ID=&lt;id&gt;\n</span> <span className="text-zinc-600 ml-2">// e.g., BUY 100 10.00 ID=1\n</span></li>
                  <li className="bg-black border border-zinc-900 p-2 rounded-sm"><span className="text-zinc-500 mr-2">Sell:</span> <span className="text-rose-400">SELL &lt;qty&gt; &lt;price&gt; ID=&lt;id&gt;\n</span> <span className="text-zinc-600 ml-2">// e.g., SELL 50 11.00 ID=2\n</span></li>
                  <li className="bg-black border border-zinc-900 p-2 rounded-sm"><span className="text-zinc-500 mr-2">Cancel:</span> <span className="text-amber-400">CANCEL &lt;id&gt;\n</span> <span className="text-zinc-600 ml-2">// e.g., CANCEL 1\n</span></li>
                </ul>
              </section>

              <section>
                <h3 className="text-emerald-400 font-mono uppercase tracking-wider text-xs mb-3 font-bold border-b border-zinc-800 pb-2">3. Outbound Protocol (What you reply)</h3>
                <p className="text-zinc-400 mb-2">Your engine must respond to every single inbound packet. Dropped packets equal immediate failure. Terminate responses with a newline (<code className="text-rose-300">\n</code>).</p>
                <ul className="space-y-3 font-mono text-xs">
                  <li className="bg-black border border-zinc-900 p-3 rounded-sm">
                    <div className="text-zinc-300 mb-1 font-sans font-semibold">Acknowledge (Resting/Canceled)</div>
                    <div className="text-zinc-500 font-sans text-xs mb-2">If an order is added to the book or a cancel is processed, reply exactly:</div>
                    <span className="text-emerald-300">ACK\n</span>
                  </li>
                  <li className="bg-black border border-zinc-900 p-3 rounded-sm">
                    <div className="text-zinc-300 mb-1 font-sans font-semibold">Partial or Full Match</div>
                    <div className="text-zinc-500 font-sans text-xs mb-2">If an inbound order matches against resting liquidity, reply with fills:</div>
                    <span className="text-emerald-300">FILL &lt;qty&gt; &lt;price&gt;\n</span> <span className="text-zinc-600 ml-2">// e.g., FILL 40 10.00\n</span>
                  </li>
                  <li className="bg-black border border-zinc-900 p-3 rounded-sm">
                    <div className="text-zinc-300 mb-1 font-sans font-semibold">Multi-Level Book Sweep</div>
                    <div className="text-zinc-500 font-sans text-xs mb-2">If a market order sweeps multiple levels, comma-separate fills (best price first):</div>
                    <span className="text-emerald-300">FILL 60 10.00, FILL 40 9.00\n</span>
                  </li>
                </ul>
                <div className="mt-4 p-3 bg-rose-950/20 border border-rose-900/30 rounded-sm">
                  <p className="text-rose-400 text-xs uppercase tracking-wider font-bold">⚠️ Strict Deterministic Audit</p>
                  <p className="text-zinc-400 text-xs mt-1">Your engine will undergo a strict correctness audit before the stress test begins. Failure to honor exact Price-Time Priority or string formatting will result in an immediate Wrong Answer (WA) termination.</p>
                </div>
              </section>

              <section>
                <div className="flex justify-between items-end border-b border-zinc-800 pb-2 mb-3">
                  <h3 className="text-emerald-400 font-mono uppercase tracking-wider text-xs font-bold">4. C++ Quick Start Boilerplate</h3>
                  <button
                    onClick={handleCopy}
                    className={`text-[10px] uppercase tracking-widest font-bold px-3 py-1.5 rounded-sm transition-colors flex items-center gap-1 ${
                      copied 
                        ? "bg-emerald-950/50 text-emerald-400 border border-emerald-900/50" 
                        : "bg-zinc-900 hover:bg-zinc-800 text-zinc-300"
                    }`}
                  >
                    {copied ? "COPIED TO CLIPBOARD ✓" : "COPY CODE"}
                  </button>
                </div>
                <p className="text-zinc-400 mb-3">Use this minimal TCP skeleton to guarantee your networking is set up correctly before writing your lock-free data structures.</p>
                <pre className="bg-black border border-zinc-900 p-4 rounded-sm overflow-x-auto">
                  <code className="text-xs font-mono text-zinc-300">{cppBoilerplate}</code>
                </pre>
              </section>

            </div>
          </div>
        </div>
      )}
    </>
  );
}