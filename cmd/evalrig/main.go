// Command evalrig runs deterministic evaluations of LLM coding agents
// inside Docker containers.
package main

import (
	"os"

	"github.com/edxeme/evalrig/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
