// Copyright 2026 [Copyright Holder]
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: [YOUR_NAME]

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

func main() {
	port := flag.Int("port", 8080, "Listen port")
	flag.Parse()

	startTime := time.Now()
	var reqCount uint64
	pid := os.Getpid()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddUint64(&reqCount, 1)
		uptime := time.Since(startTime).Round(time.Second)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
  <title>ytagarasu On-Premise Demo API</title>
  <style>
    body { font-family: -apple-system, sans-serif; background: #0f172a; color: #f8fafc; padding: 40px; }
    .card { background: #1e293b; border-radius: 12px; padding: 24px; max-width: 600px; box-shadow: 0 10px 25px rgba(0,0,0,0.5); border: 1px solid #334155; }
    h1 { color: #38bdf8; margin-top: 0; }
    .badge { display: inline-block; padding: 4px 12px; border-radius: 9999px; background: #10b981; color: #fff; font-weight: bold; font-size: 14px; }
    .metric { margin: 16px 0; font-family: monospace; font-size: 16px; }
    .btn { display: inline-block; background: #ef4444; color: white; padding: 10px 20px; border-radius: 6px; text-decoration: none; font-weight: bold; margin-top: 10px; cursor: pointer; border: none; }
    .btn:hover { background: #dc2626; }
  </style>
</head>
<body>
  <div class="card">
    <div style="display: flex; justify-content: space-between; align-items: center;">
      <h1>🦅 ytagarasu Web API</h1>
      <span class="badge">RUNNING</span>
    </div>
    <p>Supervisor（プロセス看取り層）配下で常駐監視されている実機デモプロセスです。</p>
    <div class="metric">📍 PID: <strong>%d</strong></div>
    <div class="metric">⏱️ Uptime: <strong>%s</strong></div>
    <div class="metric">📊 Total Requests: <strong>%d</strong></div>
    <hr style="border: 0; border-top: 1px solid #334155; margin: 20px 0;">
    <h3>💥 障害注入テスト (Chaos Test)</h3>
    <p>下のボタンを押すとプロセスが即座にクラッシュします。Supervisor がミリ秒で検知して自動再起動（Self-Healing）する様子をリロードして確認してください（PIDが新しくなります）。</p>
    <form action="/crash" method="POST">
      <button type="submit" class="btn">⚡ プロセスをクラッシュさせる (os.Exit)</button>
    </form>
  </div>
</body>
</html>`, pid, uptime, atomic.LoadUint64(&reqCount))
	})

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","pid":%d,"uptime":"%s"}`+"\n", pid, time.Since(startTime).Round(time.Second))
	})

	http.HandleFunc("/crash", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[DEMO-API] 💥 CRASH REQUEST RECEIVED! Terminating process (PID: %d)...", pid)
		go func() {
			time.Sleep(100 * time.Millisecond)
			os.Exit(1)
		}()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintln(w, "<h3>Process killed! Reloading in 2 seconds...</h3><script>setTimeout(() => window.location.href='/', 2000);</script>")
	})

	log.Printf("[DEMO-API] Server started on :%d (PID: %d)", *port, pid)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
