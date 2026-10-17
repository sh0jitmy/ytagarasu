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

package completion

import (
	"bytes"
	"strings"
	"testing"
)

func TestGenerate_AllShells(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		shell      Shell
		expectedIn string
	}{
		{
			name:       "bash completion",
			shell:      ShellBash,
			expectedIn: "complete -F _ytg_completions ytg",
		},
		{
			name:       "zsh completion",
			shell:      ShellZsh,
			expectedIn: "#compdef ytg",
		},
		{
			name:       "powershell completion",
			shell:      ShellPowerShell,
			expectedIn: "Register-ArgumentCompleter -Native -CommandName 'ytg'",
		},
		{
			name:       "pwsh alias completion",
			shell:      "pwsh",
			expectedIn: "Register-ArgumentCompleter -Native -CommandName 'ytg'",
		},
		{
			name:       "fish completion",
			shell:      ShellFish,
			expectedIn: "complete -c ytg",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			if err := Generate(tt.shell, &buf); err != nil {
				t.Fatalf("unexpected error generating completion for %s: %v", tt.shell, err)
			}
			out := buf.String()
			if !strings.Contains(out, tt.expectedIn) {
				t.Errorf("expected completion script to contain %q, but got: %s", tt.expectedIn, out)
			}
			if !strings.Contains(out, "supervisor") {
				t.Errorf("expected completion script to mention 'supervisor', but got: %s", out)
			}
			// Root commands verification
			for _, cmd := range []string{"status", "inspect", "restart"} {
				if !strings.Contains(out, cmd) {
					t.Errorf("expected completion script for %s to contain root command %q", tt.shell, cmd)
				}
			}
		})
	}
}

func TestGenerate_UnsupportedShell(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := Generate("unsupported-shell", &buf)
	if err == nil {
		t.Fatalf("expected error for unsupported shell, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported shell") {
		t.Errorf("expected error message to mention unsupported shell, got: %v", err)
	}
}
