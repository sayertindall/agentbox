package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"devbox/agentbox/internal/config"
	"devbox/agentbox/internal/daemon"
	"devbox/agentbox/internal/run"
	"devbox/agentbox/internal/store"
)

func main() {
	if err := runDaemon(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runDaemon(args []string) error {
	if len(args) != 2 || args[0] != "-config" {
		return fmt.Errorf("usage: agentboxd -config <host.toml>")
	}
	host, err := config.LoadHost(args[1])
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(host.Root, "control"), 0o700); err != nil {
		return err
	}
	db, err := store.Open(filepath.Join(host.Root, "control", "devbox.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	for _, profile := range host.CredentialProfiles {
		if err := db.PutProfile(profile.ID, profile.MaxActiveRuns); err != nil {
			return err
		}
	}
	srv := &daemon.Server{
		Host:         host,
		DB:           db,
		Socket:       daemon.DefaultSocket,
		Driver:       run.NewMemoryDriver(),
		FakeProvider: "/usr/local/libexec/devbox-fake-provider",
		ControlGroup: "devbox-control",
	}
	if runtime.GOOS == "linux" {
		srv.Driver = run.SystemdDriver{}
	}
	ln, err := srv.Listen()
	if err != nil {
		return err
	}
	defer ln.Close()
	fmt.Printf("agentboxd listening socket=%s root=%s\n", srv.Socket, host.Root)
	return srv.Serve(ln)
}
