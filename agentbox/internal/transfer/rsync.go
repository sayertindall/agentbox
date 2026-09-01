package transfer

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"devbox/agentbox/internal/manifest"
)

type Token struct {
	ID         string
	QuotaBytes int64
	ExpiresAt  time.Time
	used       bool
}

func Issue(sourceBytes, baselineBytes, overhead int64, ttl time.Duration) (Token, error) {
	if sourceBytes < 0 || baselineBytes < 0 || overhead < 0 {
		return Token{}, fmt.Errorf("staging quota inputs must be nonnegative")
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return Token{}, err
	}
	return Token{
		ID:         hex.EncodeToString(bytes[:]),
		QuotaBytes: sourceBytes + baselineBytes + overhead,
		ExpiresAt:  time.Now().Add(ttl),
	}, nil
}

func (t *Token) Consume() error {
	if t.used {
		return fmt.Errorf("staging token already used")
	}
	if !time.Now().Before(t.ExpiresAt) {
		return fmt.Errorf("staging token expired")
	}
	t.used = true
	return nil
}

func FilesFrom(m manifest.Manifest) []string {
	out := make([]string, 0, len(m.Entries))
	for _, entry := range m.Entries {
		out = append(out, entry.Path)
	}
	return out
}

func Command(filesFrom, src, dest string) []string {
	return []string{"rsync", "-a", "--files-from=" + filesFrom, src + "/", dest + "/"}
}

func Allow(args []string, tokenRoot string) error {
	if len(args) == 0 {
		return fmt.Errorf("empty transfer command")
	}
	base := filepath.Base(args[0])
	if base != "rsync" {
		return fmt.Errorf("transfer wrapper permits rsync only")
	}
	hasServer := false
	dest := ""
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--server" {
			hasServer = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		dest = arg
	}
	if !hasServer {
		return fmt.Errorf("transfer wrapper permits rsync server mode only")
	}
	if dest == "" || dest == "." {
		return fmt.Errorf("staging destination is required")
	}
	cleanRoot := filepath.Clean(tokenRoot)
	cleanDest := filepath.Clean(dest)
	if !strings.HasPrefix(cleanDest, cleanRoot+string(filepath.Separator)) && cleanDest != cleanRoot {
		return fmt.Errorf("staging destination is outside the token root")
	}
	return nil
}
