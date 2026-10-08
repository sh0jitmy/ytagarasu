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
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	pid := os.Getpid()
	log.Printf("[DEMO-WORKER] Background queue worker started (PID: %d)", pid)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	jobID := 0
	for {
		select {
		case sig := <-sigChan:
			log.Printf("[DEMO-WORKER] Received signal %v, gracefully shutting down...", sig)
			time.Sleep(200 * time.Millisecond)
			log.Printf("[DEMO-WORKER] Worker stopped cleanly.")
			return
		case <-ticker.C:
			jobID++
			log.Printf("[DEMO-WORKER] Processed async job #%d (Worker PID: %d, System: Healthy)", jobID, pid)
		}
	}
}
