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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sh0jitmy/ytagarasu/internal/manifest"
)

// ManifestKeygenReport represents JSON output for ytg manifest keygen.
type ManifestKeygenReport struct {
	PublicKeyPath  string `json:"public_key_path"`
	PrivateKeyPath string `json:"private_key_path"`
	PublicKey      string `json:"public_key"`
}

// ManifestSignReport represents JSON output for ytg manifest sign.
type ManifestSignReport struct {
	ManifestPath string `json:"manifest_path"`
	Signature    string `json:"signature"`
	OutputPath   string `json:"output_path,omitempty"`
}

// ManifestVerifyReport represents JSON output for ytg manifest verify.
type ManifestVerifyReport struct {
	ManifestPath string `json:"manifest_path"`
	Verified     bool   `json:"verified"`
	Error        string `json:"error,omitempty"`
}

func handleManifestKeygen(args []string) {
	isJSON, remaining := hasJSONFlag(args)
	prefix := "ytagarasu-manifest"

	for i := 0; i < len(remaining); i++ {
		if remaining[i] == "-o" || remaining[i] == "--output" {
			if i+1 < len(remaining) {
				prefix = remaining[i+1]
				i++
			}
		} else if !strings.HasPrefix(remaining[i], "-") {
			prefix = remaining[i]
		}
	}

	pubPath, privPath, err := manifest.SaveKeyPair(prefix)
	if err != nil {
		if isJSON {
			data, _ := json.MarshalIndent(map[string]string{"error": err.Error()}, "", "  ")
			fmt.Println(string(data))
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "[ytg manifest error] Failed to generate keypair: %v\n", err)
		os.Exit(1)
	}

	pubContent, _ := os.ReadFile(filepath.Clean(pubPath)) //nolint:gosec // Reading generated public key file
	pubHex := strings.TrimSpace(string(pubContent))

	if isJSON {
		report := ManifestKeygenReport{
			PublicKeyPath:  pubPath,
			PrivateKeyPath: privPath,
			PublicKey:      pubHex,
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Println("[ytg manifest] Generated offline Ed25519 signing keypair:")
	fmt.Printf("  Public Key:  %s\n", pubPath)
	fmt.Printf("  Private Key: %s (mode 0600)\n", privPath)
	fmt.Printf("  Key Fingerprint: %s...\n", pubHex[:16])
}

func handleManifestSign(args []string) {
	isJSON, remaining := hasJSONFlag(args)

	var targetFile string
	var keyPath string
	var outputPath string

	for i := 0; i < len(remaining); i++ {
		arg := remaining[i]
		switch {
		case arg == "-k" || arg == "--key":
			if i+1 < len(remaining) {
				keyPath = remaining[i+1]
				i++
			}
		case arg == "-o" || arg == "--output":
			if i+1 < len(remaining) {
				outputPath = remaining[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--key="):
			keyPath = strings.TrimPrefix(arg, "--key=")
		case strings.HasPrefix(arg, "--output="):
			outputPath = strings.TrimPrefix(arg, "--output=")
		case !strings.HasPrefix(arg, "-") && targetFile == "":
			targetFile = arg
		}
	}

	if targetFile == "" || keyPath == "" {
		if isJSON {
			data, _ := json.MarshalIndent(map[string]string{
				"error": "Usage: ytg manifest sign <manifest-file> --key <key-file> [-o <sig-file>] [--json]",
			}, "", "  ")
			fmt.Println(string(data))
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "Usage: ytg manifest sign <manifest-file> --key <key-file> [-o <sig-file>] [--json]")
		os.Exit(1)
	}

	sigHex, err := manifest.SignFile(keyPath, targetFile)
	if err != nil {
		if isJSON {
			data, _ := json.MarshalIndent(map[string]string{"error": err.Error()}, "", "  ")
			fmt.Println(string(data))
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "[ytg manifest error] Failed to sign manifest: %v\n", err)
		os.Exit(1)
	}

	if outputPath != "" {
		cleanOut := filepath.Clean(outputPath)
		if err := os.WriteFile(cleanOut, []byte(sigHex+"\n"), 0644); err != nil { //nolint:gosec // Signature is public
			if isJSON {
				data, _ := json.MarshalIndent(map[string]string{"error": err.Error()}, "", "  ")
				fmt.Println(string(data))
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "[ytg manifest error] Failed to write signature file: %v\n", err)
			os.Exit(1)
		}
	}

	if isJSON {
		report := ManifestSignReport{
			ManifestPath: targetFile,
			Signature:    sigHex,
			OutputPath:   outputPath,
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
		return
	}

	if outputPath != "" {
		fmt.Printf("[ytg manifest] Signed %s -> %s\n", targetFile, outputPath)
	} else {
		fmt.Printf("[ytg manifest] Signature: %s\n", sigHex)
	}
}

func handleManifestVerify(args []string) {
	isJSON, remaining := hasJSONFlag(args)

	var targetFile string
	var keyPath string
	var sigArg string

	for i := 0; i < len(remaining); i++ {
		arg := remaining[i]
		switch {
		case arg == "-k" || arg == "--key":
			if i+1 < len(remaining) {
				keyPath = remaining[i+1]
				i++
			}
		case arg == "-s" || arg == "--sig" || arg == "--signature":
			if i+1 < len(remaining) {
				sigArg = remaining[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--key="):
			keyPath = strings.TrimPrefix(arg, "--key=")
		case strings.HasPrefix(arg, "--sig=") || strings.HasPrefix(arg, "--signature="):
			sigArg = strings.TrimPrefix(strings.TrimPrefix(arg, "--sig="), "--signature=")
		case !strings.HasPrefix(arg, "-") && targetFile == "":
			targetFile = arg
		}
	}

	if targetFile == "" || keyPath == "" {
		if isJSON {
			data, _ := json.MarshalIndent(map[string]string{
				"error": "Usage: ytg manifest verify <manifest-file> --key <pubkey-file> [--sig <sig-file>] [--json]",
			}, "", "  ")
			fmt.Println(string(data))
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "Usage: ytg manifest verify <manifest-file> --key <pubkey-file> [--sig <sig-file>] [--json]")
		os.Exit(1)
	}

	// Default signature to <manifest-file>.sig if not specified
	if sigArg == "" {
		defaultSigFile := targetFile + ".sig"
		if _, err := os.Stat(defaultSigFile); err == nil {
			sigArg = defaultSigFile
		} else {
			if isJSON {
				data, _ := json.MarshalIndent(map[string]string{
					"error": fmt.Sprintf("no signature specified and default '%s' not found", defaultSigFile),
				}, "", "  ")
				fmt.Println(string(data))
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "[ytg manifest error] No signature specified and default '%s' not found\n", defaultSigFile)
			os.Exit(1)
		}
	}

	err := manifest.VerifyFile(keyPath, targetFile, sigArg)
	if err != nil {
		if isJSON {
			report := ManifestVerifyReport{
				ManifestPath: targetFile,
				Verified:     false,
				Error:        err.Error(),
			}
			data, _ := json.MarshalIndent(report, "", "  ")
			fmt.Println(string(data))
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "[ytg manifest error] Signature verification FAILED: %v\n", err)
		os.Exit(1)
	}

	if isJSON {
		report := ManifestVerifyReport{
			ManifestPath: targetFile,
			Verified:     true,
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("[ytg manifest] Cryptographic signature VERIFIED successfully for %s\n", targetFile)
}
