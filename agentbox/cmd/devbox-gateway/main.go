package main

import (
	"fmt"
	"os"

	"devbox/agentbox/internal/gateway"
)

func main() {
	socket := os.Getenv("AGENTBOXD_SOCKET")
	if socket == "" {
		socket = "/run/agentboxd/agentboxd.sock"
	}
	if err := gateway.Serve(os.Stdin, os.Stdout, socket); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
