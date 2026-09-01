package gateway

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"

	"devbox/agentbox/internal/protocol"
)

func Serve(r io.Reader, w io.Writer, socketPath string) error {
	if socketPath == "" || strings.Contains(socketPath, "://") && !strings.HasPrefix(socketPath, "unix:") {
		return fmt.Errorf("agentboxd unix socket is required")
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, protocol.MaxRequestBytes), protocol.MaxRequestBytes)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return err
		}
		return fmt.Errorf("empty request")
	}
	line := scanner.Bytes()
	if _, err := protocol.Decode(line); err != nil {
		return err
	}
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("dial agentboxd: %w", err)
	}
	defer conn.Close()
	if _, err := conn.Write(append(append([]byte{}, line...), '\n')); err != nil {
		return fmt.Errorf("forward request: %w", err)
	}
	if _, err := io.Copy(w, conn); err != nil {
		return fmt.Errorf("read agentboxd response: %w", err)
	}
	return nil
}
