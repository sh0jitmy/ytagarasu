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
	"bytes"
	"strings"
	"testing"

	"github.com/sh0jitmy/ytagarasu/pkg/completion"
)

func TestCompletion_OutputGeneration(t *testing.T) {
	t.Parallel()

	shells := []completion.Shell{
		completion.ShellBash,
		completion.ShellZsh,
		completion.ShellPowerShell,
		completion.ShellFish,
	}

	for _, sh := range shells {
		sh := sh
		t.Run(string(sh), func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			if err := completion.Generate(sh, &buf); err != nil {
				t.Fatalf("failed to generate %s completion: %v", sh, err)
			}
			out := buf.String()
			if len(out) < 50 {
				t.Errorf("completion script for %s too short: %d bytes", sh, len(out))
			}
			if !strings.Contains(out, "ytg") {
				t.Errorf("completion script for %s does not contain 'ytg'", sh)
			}
			if !strings.Contains(out, "completion") {
				t.Errorf("completion script for %s does not contain 'completion'", sh)
			}
		})
	}
}
