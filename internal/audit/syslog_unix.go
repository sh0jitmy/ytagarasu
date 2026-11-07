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

//go:build !windows

package audit

import (
	"fmt"
	"net"
	"os"
)

// Standard unix syslog socket paths.
var standardSyslogSockets = []string{
	"/dev/log",        // Standard Linux (systemd, rsyslog, syslog-ng)
	"/var/run/syslog", // macOS / BSD
	"/var/run/log",    // OpenBSD / Alternative
}

// NewSyslogForwarder establishes a connection to the native Unix syslog daemon (Cgo-free Pure Go).
func NewSyslogForwarder(cfg Config) (Forwarder, error) {
	if cfg.CustomWriter != nil {
		return NewWriterForwarder(cfg.CustomWriter, cfg), nil
	}

	// If explicit network & address provided
	if cfg.Network != "" && cfg.Address != "" {
		conn, err := net.Dial(cfg.Network, cfg.Address)
		if err != nil {
			return nil, fmt.Errorf("failed to dial configured syslog endpoint (%s://%s): %w", cfg.Network, cfg.Address, err)
		}
		return NewWriterForwarder(conn, cfg), nil
	}

	// Auto-discovery of local unix syslog daemon sockets
	for _, socketPath := range standardSyslogSockets {
		if fi, err := os.Stat(socketPath); err == nil && (fi.Mode()&os.ModeSocket != 0) {
			// Try unixgram first (Linux default datagram socket)
			conn, err := net.Dial("unixgram", socketPath)
			if err == nil {
				return NewWriterForwarder(conn, cfg), nil
			}

			// Try stream unix socket next (macOS / BSD default)
			conn, err = net.Dial("unix", socketPath)
			if err == nil {
				return NewWriterForwarder(conn, cfg), nil
			}
		}
	}

	return nil, fmt.Errorf("no operational OS native syslog socket detected in %v; verify local rsyslog/syslogd", standardSyslogSockets)
}
