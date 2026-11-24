// Comando atlana: instala packs Atlana (skills, subagents, hooks, MCP) em qualquer agente.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/atlana-ai-hq/atlana/internal/adapter"
	"github.com/atlana-ai-hq/atlana/internal/cli"
)

func main() {
	env, err := adapter.SystemEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "atlana:", err)
		os.Exit(1)
	}
	app := &cli.App{
		Stdout: os.Stdout, Stderr: os.Stderr, Stdin: os.Stdin,
		Env: env, Now: time.Now, Interactive: cli.IsTerminal(os.Stdin),
	}
	os.Exit(app.Run(os.Args[1:]))
}
