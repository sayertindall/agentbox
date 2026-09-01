package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"devbox/agentbox/internal/protocol"
)

var ErrUnavailable = errors.New("control transport unavailable")

type Config struct {
	Host        string
	Key         string
	Transfer    string
	TransferKey string
	Proxy       string
}

func FromEnv() (Config, error) {
	c := loadFile()
	if v := os.Getenv("DEVBOX_CONTROL"); v != "" {
		c.Host = v
	}
	if v := os.Getenv("DEVBOX_CONTROL_KEY"); v != "" {
		c.Key = v
	}
	if v := os.Getenv("DEVBOX_TRANSFER"); v != "" {
		c.Transfer = v
	}
	if v := os.Getenv("DEVBOX_TRANSFER_KEY"); v != "" {
		c.TransferKey = v
	}
	if v := os.Getenv("DEVBOX_SSH_PROXY"); v != "" {
		c.Proxy = v
	}
	if c.Host == "" {
		return Config{}, ErrUnavailable
	}
	if c.Transfer == "" {
		c.Transfer = "devbox-transfer"
	}
	return c, nil
}

func loadFile() Config {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "agentbox", "config.toml"))
	if err != nil {
		return Config{}
	}
	var c Config
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"`)
		switch key {
		case "control":
			c.Host = value
		case "transfer":
			c.Transfer = value
		case "control_key":
			c.Key = value
		case "transfer_key":
			c.TransferKey = value
		case "proxy":
			c.Proxy = value
		}
	}
	return c
}

func (c Config) Call(req protocol.Request) (protocol.Response, error) {
	raw, err := json.Marshal(req)
	if err != nil {
		return protocol.Response{}, err
	}
	args := []string{"-T", "-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes", "-o", "ConnectTimeout=10"}
	if c.Proxy != "" {
		args = append(args, "-o", "ProxyCommand="+c.Proxy)
	}
	if c.Key != "" {
		args = append(args, "-i", c.Key)
	}
	args = append(args, c.Host)
	cmd := exec.Command("ssh", args...)
	cmd.Stdin = bytes.NewReader(append(raw, '\n'))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return protocol.Response{}, fmt.Errorf("control ssh: %w: %s", err, bytes.TrimSpace(out))
	}
	resp, err := protocol.DecodeResponse(out)
	if err != nil {
		return protocol.Response{}, fmt.Errorf("control response: %w: %s", err, bytes.TrimSpace(out))
	}
	if !resp.OK {
		return resp, fmt.Errorf("%s", resp.Error)
	}
	return resp, nil
}

func (c Config) Rsync(src, dest string) error {
	if c.Transfer == "" {
		return fmt.Errorf("DEVBOX_TRANSFER is required")
	}
	ssh := "ssh -o BatchMode=yes -o IdentitiesOnly=yes -o ConnectTimeout=10"
	if c.TransferKey != "" {
		ssh += " -i " + c.TransferKey
	}
	cmd := exec.Command("rsync", "-rlt", "--omit-dir-times", "-e", ssh, src+"/", c.Transfer+":"+dest+"/")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rsync %s: %w: %s", dest, err, bytes.TrimSpace(out))
	}
	return nil
}
