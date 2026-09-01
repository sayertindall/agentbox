package credential

import (
	"fmt"
	"os"
	"path/filepath"
)

type Profile struct {
	ID                string
	Provider          string
	StoreDir          string
	Adaptor           string
	EgressPolicyID    string
	MaxActiveRuns     int
	Verified          bool
	RevocationChecked bool
	EgressChecked     bool
	Material          []byte
	active            string
}

func New(p Profile) (Profile, error) {
	if p.ID == "" || p.Provider == "" || p.StoreDir == "" {
		return Profile{}, fmt.Errorf("credential profile is incomplete")
	}
	if p.MaxActiveRuns <= 0 {
		return Profile{}, fmt.Errorf("credential profile has no capacity")
	}
	if !p.Verified || !p.RevocationChecked || !p.EgressChecked {
		return Profile{}, fmt.Errorf("credential profile is not ready for a run")
	}
	if p.Adaptor != "systemd-credentials" && p.Adaptor != "bind-mount" {
		return Profile{}, fmt.Errorf("invalid credential profile adaptor")
	}
	if err := os.MkdirAll(p.StoreDir, 0o700); err != nil {
		return Profile{}, err
	}
	if err := os.WriteFile(filepath.Join(p.StoreDir, "material"), p.Material, 0o600); err != nil {
		return Profile{}, err
	}
	return p, nil
}

func (p *Profile) Lock(operationID string) error {
	if p.active != "" {
		return fmt.Errorf("credential profile is locked")
	}
	p.active = operationID
	return nil
}

func (p *Profile) Unlock() error {
	p.active = ""
	return nil
}

func (p Profile) Locked() bool { return p.active != "" }

func (p Profile) Inject(dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	return os.WriteFile(dest, p.Material, 0o600)
}

func (p Profile) StorePath() string {
	return filepath.Join(p.StoreDir, "material")
}
