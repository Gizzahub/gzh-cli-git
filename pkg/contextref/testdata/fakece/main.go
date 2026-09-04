// Fake CE v2 for contextref tests. Scenario is a sibling file named "scenario".
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	scenario := readScenario()
	args := os.Args[1:]
	if len(args) >= 1 && args[0] == "version" {
		printVersion()
		os.Exit(0)
	}
	if len(args) >= 2 && args[0] == "task" && args[1] == "doctor" {
		if cap := flagValue(args, "--capability"); cap != "" && cap != "ce.task.gate-doctor/v2" {
			os.Exit(2)
		}
		switch scenario {
		case "fault":
			os.Exit(2)
		case "hang":
			time.Sleep(60 * time.Second)
			os.Exit(0)
		case "overflow":
			fmt.Print(strings.Repeat("x", 2<<20))
			os.Exit(0)
		case "extra-json":
			printDoctor("adopted")
			fmt.Println(`{"extra":true}`)
			os.Exit(0)
		case "disagree-0":
			printDoctor("not-adopted")
			os.Exit(0)
		case "disagree-1":
			printDoctor("adopted")
			os.Exit(1)
		case "finding":
			printDoctor("not-adopted")
			os.Exit(1)
		default:
			printDoctor("adopted")
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func readScenario() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "scenario")) //nolint:gosec
	if err != nil {
		return "pass"
	}
	return strings.TrimSpace(string(raw))
}

func flagValue(args []string, name string) string {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(arg, name+"=") {
			return strings.TrimPrefix(arg, name+"=")
		}
	}
	return ""
}

func printVersion() {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"version":      "0.8.3",
		"revision":     "golden",
		"capabilities": []string{"ce.task.gate-doctor/v2"},
	})
}

func printDoctor(status string) {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"capabilityId":      "ce.task.gate-doctor/v2",
		"schemaVersion":     2,
		"observationSource": "ce-task-doctor",
		"symlinkSafe":       true,
		"repositoryId":      "example",
		"commonGitDir":      "/tmp/git",
		"worktrees":         []any{},
		"gates":             []any{},
		"status":            status,
		"remediation":       "none",
		"buildRevision":     "golden",
	})
}
