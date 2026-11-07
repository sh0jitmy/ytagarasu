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

//go:build windows

package audit

import (
	"fmt"
	"net"
	"os"
)

// NewSyslogForwarder on Windows connects to configured syslog endpoint or falls back to stderr/stdout safely.
func NewSyslogForwarder(cfg Config) (Forwarder, error) {
	if cfg.CustomWriter != nil {
		return NewWriterForwarder(cfg.CustomWriter, cfg), nil
	}

	if cfg.Network != "" && cfg.Address != "" {
		conn, err := net.Dial(cfg.Network, cfg.Address)
		if err != nil {
			return nil, fmt.Errorf("failed to dial remote syslog service (%s://%s): %w", cfg.Network, cfg.Address, err)
		}
		return NewWriterForwarder(conn, cfg), nil
	}

	// Windows fallback: write formatted audit frames to os.Stderr
	return NewWriterForwarder(os.Stderr, cfg), nil
}
