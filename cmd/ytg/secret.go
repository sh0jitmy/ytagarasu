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
	"os"
	"path/filepath"
	"strings"

	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/sh0jitmy/ytagarasu/internal/secret"
)

func handleSecretCommand(args []string) {
	if len(args) == 0 {
		printSecretHelp()
		os.Exit(1)
	}

	subcommand := args[0]
	switch subcommand {
	case "keygen":
		handleSecretKeygen(args[1:])
	case "encrypt":
		handleSecretEncrypt(args[1:])
	case "decrypt":
		handleSecretDecrypt(args[1:])
	case "view":
		handleSecretView(args[1:])
	case "help", "--help", "-h":
		printSecretHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown secret subcommand: %s\n", subcommand)
		printSecretHelp()
		os.Exit(1)
	}
}

func printSecretHelp() {
	fmt.Println("Usage: ytg secret <subcommand> [options] [arguments]")
	fmt.Println("\nPure Go age/X25519 Inline Secret Management for Air-Gapped Environments.")
	fmt.Println("\nAvailable Subcommands:")
	fmt.Println("  keygen    Generate a new age/X25519 public/private keypair")
	fmt.Println("  encrypt   Encrypt a plaintext secret using a recipient public key")
	fmt.Println("  decrypt   Decrypt an inline ENC[age,...] ciphertext using an identity private key")
	fmt.Println("  view      Decrypt and view all secrets defined in a manifest.yaml")
	fmt.Println("\nExamples:")
	fmt.Println("  ytg secret keygen -o deploy.key")
	fmt.Println("  ytg secret encrypt -k deploy.pub \"my-super-secret-password\"")
	fmt.Println("  ytg secret decrypt -k deploy.key \"ENC[age,x25519:abcdef...]\"")
	fmt.Println("  ytg secret view -c manifest.yaml -k deploy.key")
}

func handleSecretKeygen(args []string) {
	fs := flag.NewFlagSet("secret keygen", flag.ExitOnError)
	outputFile := fs.String("o", "", "Output file path to save private key (e.g. deploy.key)")
	_ = fs.Parse(args)

	pubKey, privKey, err := secret.GenerateKeyPair()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating keypair: %v\n", err)
		os.Exit(1)
	}

	if *outputFile != "" {
		keyPath := *outputFile
		pubPath := strings.TrimSuffix(keyPath, filepath.Ext(keyPath)) + ".pub"

		if err := os.WriteFile(keyPath, []byte(privKey+"\n"), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving private key to %s: %v\n", keyPath, err)
			os.Exit(1)
		}
		if err := os.WriteFile(pubPath, []byte(pubKey+"\n"), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving public key to %s: %v\n", pubPath, err)
			os.Exit(1)
		}

		fmt.Printf("✓ Generated age/X25519 keypair\n")
		fmt.Printf("  Public key (recipient)  : %s -> saved to %s\n", pubKey, pubPath)
		fmt.Printf("  Private key (identity)  : [PROTECTED] -> saved to %s\n", keyPath)
		return
	}

	fmt.Printf("Public key (recipient):\n%s\n\n", pubKey)
	fmt.Printf("Private key (identity):\n%s\n", privKey)
}

func handleSecretEncrypt(args []string) {
	fs := flag.NewFlagSet("secret encrypt", flag.ExitOnError)
	key := fs.String("k", "", "Recipient public key or path to public key file (.pub)")
	_ = fs.Parse(args)

	if *key == "" {
		fmt.Fprintln(os.Stderr, "Error: -k <public-key|file> is required")
		os.Exit(1)
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		fmt.Fprintln(os.Stderr, "Error: plaintext string argument is required")
		os.Exit(1)
	}

	plaintext := remaining[0]
	encrypted, err := secret.Encrypt(*key, []byte(plaintext))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Encryption failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(encrypted)
}

func handleSecretDecrypt(args []string) {
	fs := flag.NewFlagSet("secret decrypt", flag.ExitOnError)
	key := fs.String("k", "", "Identity private key or path to private key file (.key)")
	_ = fs.Parse(args)

	if *key == "" {
		fmt.Fprintln(os.Stderr, "Error: -k <private-key|file> is required")
		os.Exit(1)
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		fmt.Fprintln(os.Stderr, "Error: encrypted ENC[...] string argument is required")
		os.Exit(1)
	}

	ciphertext := remaining[0]
	decrypted, err := secret.Decrypt(*key, ciphertext)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Decryption failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(decrypted))
}

func handleSecretView(args []string) {
	fs := flag.NewFlagSet("secret view", flag.ExitOnError)
	configPath := fs.String("c", "manifest.yaml", "Path to manifest.yaml")
	key := fs.String("k", "", "Identity private key or path to private key file (.key)")
	_ = fs.Parse(args)

	if *key == "" {
		fmt.Fprintln(os.Stderr, "Error: -k <private-key|file> is required to view decrypted secrets")
		os.Exit(1)
	}

	m, err := manifest.ParseFile(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse manifest at %s: %v\n", *configPath, err)
		os.Exit(1)
	}

	if len(m.Secrets) == 0 {
		fmt.Println("No secrets defined in manifest.")
		return
	}

	decryptedMap, err := secret.DecryptSecretMap(m.Secrets, *key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to decrypt secrets: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("=== Decrypted Secrets for %s ===\n", *configPath)
	for k, v := range decryptedMap {
		fmt.Printf("  %s = %s\n", k, v)
	}
}
