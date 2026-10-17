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

package web

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// SupervisionCollector implements prometheus.Collector to dynamically expose process supervision states.
type SupervisionCollector struct {
	manager     *SupervisionManager
	statusDesc  *prometheus.Desc
	restartDesc *prometheus.Desc
	uptimeDesc  *prometheus.Desc
	cpuDesc     *prometheus.Desc
}

func newSupervisionCollector(manager *SupervisionManager) *SupervisionCollector {
	return &SupervisionCollector{
		manager: manager,
		statusDesc: prometheus.NewDesc(
			"ytagarasu_process_status",
			"Current runtime status of supervised process (1=RUNNING, 0=OTHER)",
			[]string{"process"},
			nil,
		),
		restartDesc: prometheus.NewDesc(
			"ytagarasu_process_restart_total",
			"Total number of times the process has been restarted",
			[]string{"process"},
			nil,
		),
		uptimeDesc: prometheus.NewDesc(
			"ytagarasu_process_uptime_seconds",
			"Uptime of the supervised process in seconds",
			[]string{"process"},
			nil,
		),
		cpuDesc: prometheus.NewDesc(
			"ytagarasu_process_cpu_percent",
			"Configured base CPU utilization for supervised process",
			[]string{"process"},
			nil,
		),
	}
}

// Describe sends the super-descriptors to the prometheus channel.
func (c *SupervisionCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.statusDesc
	ch <- c.restartDesc
	ch <- c.uptimeDesc
	ch <- c.cpuDesc
}

// Collect reads dynamic process telemetry and sends metrics to prometheus.
func (c *SupervisionCollector) Collect(ch chan<- prometheus.Metric) {
	if c.manager == nil {
		return
	}
	procs := c.manager.GetProcessesSnapshot()
	now := time.Now()
	for _, p := range procs {
		statusVal := 0.0
		if p.Status == "RUNNING" {
			statusVal = 1.0
		}
		ch <- prometheus.MustNewConstMetric(c.statusDesc, prometheus.GaugeValue, statusVal, p.Name)
		ch <- prometheus.MustNewConstMetric(c.restartDesc, prometheus.CounterValue, float64(p.RestartCount), p.Name)

		uptime := 0.0
		if !p.StartedAt.IsZero() && p.Status == "RUNNING" {
			uptime = now.Sub(p.StartedAt).Seconds()
			if uptime < 0 {
				uptime = 0
			}
		}
		ch <- prometheus.MustNewConstMetric(c.uptimeDesc, prometheus.GaugeValue, uptime, p.Name)
		ch <- prometheus.MustNewConstMetric(c.cpuDesc, prometheus.GaugeValue, p.BaseCPU, p.Name)
	}
}
