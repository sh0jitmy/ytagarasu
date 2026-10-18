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

// Package completion generates shell autocompletion scripts for ytg
// supporting bash, zsh, powershell, and fish.
package completion

import (
	"fmt"
	"io"
	"strings"
)

// Shell represents a supported completion shell environment.
type Shell string

const (
	ShellBash       Shell = "bash"
	ShellZsh        Shell = "zsh"
	ShellPowerShell Shell = "powershell"
	ShellFish       Shell = "fish"
)

// SupportedShells lists all recognized completion shells.
var SupportedShells = []Shell{
	ShellBash,
	ShellZsh,
	ShellPowerShell,
	ShellFish,
}

// Generate writes the autocompletion script for the specified shell to the writer.
func Generate(sh Shell, w io.Writer) error {
	switch strings.ToLower(string(sh)) {
	case string(ShellBash):
		_, err := io.WriteString(w, bashCompletion)
		return err
	case string(ShellZsh):
		_, err := io.WriteString(w, zshCompletion)
		return err
	case string(ShellPowerShell), "ps", "pwsh":
		_, err := io.WriteString(w, powerShellCompletion)
		return err
	case string(ShellFish):
		_, err := io.WriteString(w, fishCompletion)
		return err
	default:
		return fmt.Errorf("unsupported shell '%s'. Supported shells: bash, zsh, powershell, fish", sh)
	}
}

const bashCompletion = `# bash completion for ytg
_ytg_completions() {
    local cur prev words cword
    _init_completion -n : 2>/dev/null || {
        cur="${COMP_WORDS[COMP_CWORD]}"
        prev="${COMP_WORDS[COMP_CWORD-1]}"
        words=("${COMP_WORDS[@]}")
        cword=$COMP_CWORD
    }

    local commands="status inspect restart supervisor manifest bundle audit discover version completion help"

    if [ "$cword" -eq 1 ]; then
        COMPREPLY=( $(compgen -W "$commands" -- "$cur") )
        return 0
    fi

    case "${words[1]}" in
        status)
            COMPREPLY=( $(compgen -W "--json" -- "$cur") )
            ;;
        inspect)
            COMPREPLY=( $(compgen -W "--json" -- "$cur") )
            ;;
        restart)
            COMPREPLY=( $(compgen -W "--all" -- "$cur") )
            ;;
        supervisor)
            local subcmds="run status inspect restart rollback help"
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=( $(compgen -W "$subcmds" -- "$cur") )
                return 0
            fi
            case "${words[2]}" in
                run)
                    COMPREPLY=( $(compgen -W "-c --config --manifest" -- "$cur") )
                    ;;
                status|inspect)
                    COMPREPLY=( $(compgen -W "--json" -- "$cur") )
                    ;;
                restart|rollback)
                    COMPREPLY=( $(compgen -W "--all" -- "$cur") )
                    ;;
            esac
            ;;
        manifest)
            local subcmds="init generate validate lint help"
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=( $(compgen -W "$subcmds" -- "$cur") )
                return 0
            fi
            ;;
        bundle)
            local subcmds="export diff verify keygen help"
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=( $(compgen -W "$subcmds" -- "$cur") )
                return 0
            fi
            ;;
        audit)
            local subcmds="verify list help"
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=( $(compgen -W "$subcmds" -- "$cur") )
                return 0
            fi
            ;;
        discover)
            local subcmds="survey generate help"
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=( $(compgen -W "$subcmds" -- "$cur") )
                return 0
            fi
            ;;
        completion)
            local subcmds="bash zsh powershell fish"
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=( $(compgen -W "$subcmds" -- "$cur") )
                return 0
            fi
            ;;
    esac
}

complete -F _ytg_completions ytg
`

const zshCompletion = `#compdef ytg

_ytg() {
    local line state

    _arguments -C \
        "1: :->command" \
        "*::arg:->args"

    case $state in
        command)
            local -a commands
            commands=(
                'status:Display process status, PID, and health'
                'inspect:Display detailed process metrics and paths'
                'restart:Restart a specific process or all processes'
                'supervisor:Manage runtime process supervision daemon'
                'manifest:Manage deployment bundle manifests'
                'bundle:Manage air-gapped deployment packages'
                'audit:Verify cryptographic hashchain logs'
                'discover:Interactive server survey and automated manifest discovery'
                'completion:Generate shell autocompletion script'
                'version:Show binary version and build metadata'
                'help:Show help information'
            )
            _describe -t commands 'ytg command' commands
            ;;
        args)
            case $line[1] in
                status|inspect)
                    _arguments \
                        '--json[Output status in JSON format]'
                    ;;
                restart)
                    _arguments \
                        '--all[Apply to all processes in manifest]'
                    ;;
                supervisor)
                    local -a supervisor_cmds
                    supervisor_cmds=(
                        'run:Start supervisor daemon'
                        'status:Display process status'
                        'inspect:Display detailed process metrics'
                        'restart:Gracefully restart supervised process'
                        'rollback:Trigger rollback to previous version'
                        'help:Show supervisor help'
                    )
                    _arguments -C \
                        "1: :->subcommand" \
                        "*::arg:->subargs"
                    case $state in
                        subcommand)
                            _describe -t supervisor_cmds 'supervisor subcommand' supervisor_cmds
                            ;;
                        subargs)
                            case $line[2] in
                                run)
                                    _arguments \
                                        {-c,--config,--manifest}'[Path to manifest file]:manifest file:_files'
                                    ;;
                                status|inspect)
                                    _arguments \
                                        '--json[Output status in JSON format]'
                                    ;;
                                restart|rollback)
                                    _arguments \
                                        '--all[Apply to all processes in manifest]'
                                    ;;
                            esac
                            ;;
                    esac
                    ;;
                completion)
                    local -a shells
                    shells=('bash:Generate Bash completion script' 'zsh:Generate Zsh completion script' 'powershell:Generate PowerShell completion script' 'fish:Generate Fish completion script')
                    _describe -t shells 'shell' shells
                    ;;
                manifest)
                    local -a manifest_cmds
                    manifest_cmds=('init:Initialize a new manifest' 'generate:Generate manifest' 'validate:Validate manifest schema' 'lint:Lint manifest syntax')
                    _describe -t manifest_cmds 'manifest subcommand' manifest_cmds
                    ;;
                bundle)
                    local -a bundle_cmds
                    bundle_cmds=('export:Export bundle archive' 'diff:Diff bundle contents' 'verify:Verify cryptographic signature' 'keygen:Generate signing keys')
                    _describe -t bundle_cmds 'bundle subcommand' bundle_cmds
                    ;;
                audit)
                    local -a audit_cmds
                    audit_cmds=('verify:Verify audit log integrity' 'list:List audit events')
                    _describe -t audit_cmds 'audit subcommand' audit_cmds
                    ;;
                discover)
                    local -a discover_cmds
                    discover_cmds=('survey:Survey local system environment' 'generate:Generate manifest from discovered services')
                    _describe -t discover_cmds 'discover subcommand' discover_cmds
                    ;;
            esac
            ;;
    esac
}

_ytg "$@"
`

const powerShellCompletion = `# PowerShell completion for ytg
Register-ArgumentCompleter -Native -CommandName 'ytg' -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    $elements = $commandAst.Elements
    $count = $elements.Count

    $topCommands = @('status', 'inspect', 'restart', 'supervisor', 'manifest', 'bundle', 'audit', 'discover', 'version', 'completion', 'help')
    $supervisorCommands = @('run', 'status', 'inspect', 'restart', 'rollback', 'help')
    $manifestCommands = @('init', 'generate', 'validate', 'lint', 'help')
    $bundleCommands = @('export', 'diff', 'verify', 'keygen', 'help')
    $auditCommands = @('verify', 'list', 'help')
    $discoverCommands = @('survey', 'generate', 'help')
    $completionShells = @('bash', 'zsh', 'powershell', 'fish')

    if ($count -le 2) {
        $topCommands | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
        }
        return
    }

    $sub = $elements[1].Extent.Text
    if ($count -eq 3) {
        switch ($sub) {
            'status' {
                @('--json') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
                }
            }
            'inspect' {
                @('--json') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
                }
            }
            'restart' {
                @('--all') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
                }
            }
            'supervisor' {
                $supervisorCommands | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
                }
            }
            'manifest' {
                $manifestCommands | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
                }
            }
            'bundle' {
                $bundleCommands | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
                }
            }
            'audit' {
                $auditCommands | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
                }
            }
            'discover' {
                $discoverCommands | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
                }
            }
            'completion' {
                $completionShells | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
                }
            }
        }
        return
    }

    if ($count -gt 3 -and $sub -eq 'supervisor') {
        $action = $elements[2].Extent.Text
        switch ($action) {
            'run' {
                @('-c', '--config', '--manifest') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
                }
            }
            'status' {
                @('--json') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
                }
            }
            'inspect' {
                @('--json') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
                }
            }
            'restart' {
                @('--all') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
                }
            }
            'rollback' {
                @('--all') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
                }
            }
        }
    }
}
`

const fishCompletion = `# Fish completion for ytg
complete -c ytg -f

# Top level commands
complete -c ytg -n "__fish_use_subcommand" -a status -d "Display process status"
complete -c ytg -n "__fish_use_subcommand" -a inspect -d "Display detailed process metrics"
complete -c ytg -n "__fish_use_subcommand" -a restart -d "Restart a process or all"
complete -c ytg -n "__fish_use_subcommand" -a supervisor -d "Manage runtime process supervision and rollback"
complete -c ytg -n "__fish_use_subcommand" -a manifest -d "Manage deployment bundle manifests"
complete -c ytg -n "__fish_use_subcommand" -a bundle -d "Manage air-gapped deployment packages"
complete -c ytg -n "__fish_use_subcommand" -a audit -d "Verify cryptographic hashchain logs"
complete -c ytg -n "__fish_use_subcommand" -a discover -d "Interactive server survey and automated manifest discovery"
complete -c ytg -n "__fish_use_subcommand" -a version -d "Show binary version and build metadata"
complete -c ytg -n "__fish_use_subcommand" -a completion -d "Generate shell autocompletion script"
complete -c ytg -n "__fish_use_subcommand" -a help -d "Show help information"

# root commands options
complete -c ytg -n "__fish_seen_subcommand_from status" -l json -d "Output status as JSON"
complete -c ytg -n "__fish_seen_subcommand_from inspect" -l json -d "Output detailed inspection as JSON"
complete -c ytg -n "__fish_seen_subcommand_from restart" -l all -d "Restart all processes"

# supervisor subcommands
complete -c ytg -n "__fish_seen_subcommand_from supervisor" -a run -d "Start supervisor daemon"
complete -c ytg -n "__fish_seen_subcommand_from supervisor" -a status -d "Display process status"
complete -c ytg -n "__fish_seen_subcommand_from supervisor" -a inspect -d "Display detailed process metrics"
complete -c ytg -n "__fish_seen_subcommand_from supervisor" -a restart -d "Restart a process or all"
complete -c ytg -n "__fish_seen_subcommand_from supervisor" -a rollback -d "Rollback a process or all"
complete -c ytg -n "__fish_seen_subcommand_from supervisor; and __fish_seen_subcommand_from status" -l json -d "Output status as JSON"
complete -c ytg -n "__fish_seen_subcommand_from supervisor; and __fish_seen_subcommand_from inspect" -l json -d "Output detailed inspection as JSON"
complete -c ytg -n "__fish_seen_subcommand_from supervisor; and __fish_seen_subcommand_from restart" -l all -d "Restart all processes"
complete -c ytg -n "__fish_seen_subcommand_from supervisor; and __fish_seen_subcommand_from rollback" -l all -d "Rollback all processes"
complete -c ytg -n "__fish_seen_subcommand_from supervisor; and __fish_seen_subcommand_from run" -s c -l config -l manifest -d "Manifest file path"

# completion subcommands
complete -c ytg -n "__fish_seen_subcommand_from completion" -a bash -d "Generate Bash completion"
complete -c ytg -n "__fish_seen_subcommand_from completion" -a zsh -d "Generate Zsh completion"
complete -c ytg -n "__fish_seen_subcommand_from completion" -a powershell -d "Generate PowerShell completion"
complete -c ytg -n "__fish_seen_subcommand_from completion" -a fish -d "Generate Fish completion"
`
