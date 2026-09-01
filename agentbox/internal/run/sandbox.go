package run

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"devbox/agentbox/internal/config"
	"devbox/agentbox/internal/credential"
	"devbox/agentbox/internal/egress"
)

type Driver interface {
	CreateUser(name string) error
	DeleteUser(name string) error
	HasUser(name string) bool
	WriteUnit(unit, body string) error
	RemoveUnit(unit string) error
	HasUnit(unit string) bool
	StopCgroup(unit string) error
	CgroupEmpty(unit string) bool
}

type Input struct {
	Root        string
	ControlRoot string
	Host        config.Host
	Profile     *credential.Profile
	Driver      Driver
	Egress      egress.Policy
}

type Sandbox struct {
	RunID          string
	UnixUser       string
	SystemdUnit    string
	Root           string
	Workspace      string
	MetadataRoot   string
	PaseoHome      string
	PaseoSocket    string
	CredentialPath string
	QuotaBytes     int64
	MemoryMax      string
	CPUQuota       string
	TasksMax       int
	Egress         egress.Policy
	Profile        *credential.Profile
}

func Create(in Input) (Sandbox, error) {
	if in.Driver == nil {
		return Sandbox{}, fmt.Errorf("run driver is required")
	}
	if in.Profile == nil {
		return Sandbox{}, fmt.Errorf("credential profile is required")
	}
	id, err := newID()
	if err != nil {
		return Sandbox{}, err
	}
	user := "devbox-run-" + id
	if err := in.Driver.CreateUser(user); err != nil {
		return Sandbox{}, err
	}
	root := filepath.Join(in.Root, id)
	meta := filepath.Join(root, "metadata")
	box := Sandbox{
		RunID:          id,
		UnixUser:       user,
		SystemdUnit:    "devbox-run-" + id + ".service",
		Root:           root,
		Workspace:      filepath.Join(root, "workspace"),
		MetadataRoot:   meta,
		PaseoHome:      filepath.Join(meta, "paseo-home"),
		PaseoSocket:    filepath.Join(meta, "paseo.sock"),
		CredentialPath: filepath.Join(meta, "credential"),
		QuotaBytes:     in.Host.RunMaxBytes,
		MemoryMax:      in.Host.RunMemoryMax,
		CPUQuota:       in.Host.RunCPUQuota,
		TasksMax:       in.Host.RunTasksMax,
		Egress:         in.Egress,
		Profile:        in.Profile,
	}
	for _, dir := range []string{box.Workspace, box.PaseoHome, filepath.Join(meta, "raw-history"), filepath.Join(meta, "git")} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return Sandbox{}, err
		}
	}
	if err := in.Profile.Inject(box.CredentialPath); err != nil {
		return Sandbox{}, err
	}
	if err := in.Driver.WriteUnit(box.SystemdUnit, unitBody(box)); err != nil {
		return Sandbox{}, err
	}
	return box, nil
}

func StopAndArchive(box Sandbox, archiveRoot string, driver Driver) error {
	if err := driver.StopCgroup(box.SystemdUnit); err != nil {
		return err
	}
	if !driver.CgroupEmpty(box.SystemdUnit) {
		return fmt.Errorf("run cgroup is not empty")
	}
	if err := os.MkdirAll(filepath.Join(archiveRoot, box.RunID), 0o700); err != nil {
		return err
	}
	_ = os.Rename(box.Workspace, filepath.Join(archiveRoot, box.RunID, "workspace"))
	_ = os.Rename(box.MetadataRoot, filepath.Join(archiveRoot, box.RunID, "metadata"))
	_ = os.Remove(box.PaseoSocket)
	if err := driver.RemoveUnit(box.SystemdUnit); err != nil {
		retainLock(box)
		return err
	}
	if err := driver.DeleteUser(box.UnixUser); err != nil {
		retainLock(box)
		return err
	}
	if box.Profile != nil {
		return box.Profile.Unlock()
	}
	return nil
}

func retainLock(box Sandbox) {
	// ponytail: lock stays held; receipt state is recorded by the caller.
}

func Allowed(box Sandbox, path string) bool {
	clean := filepath.Clean(path)
	if clean == box.Workspace || strings.HasPrefix(clean, box.Workspace+string(os.PathSeparator)) {
		return true
	}
	if clean == box.MetadataRoot || strings.HasPrefix(clean, box.MetadataRoot+string(os.PathSeparator)) {
		return true
	}
	return false
}

func unitBody(box Sandbox) string {
	return fmt.Sprintf("User=%s\nMemoryMax=%s\nCPUQuota=%s\nTasksMax=%d\n", box.UnixUser, box.MemoryMax, box.CPUQuota, box.TasksMax)
}

func newID() (string, error) {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

type MemoryDriver struct {
	users      map[string]bool
	units      map[string]string
	empty      map[string]bool
	FailDelete bool
}

func NewMemoryDriver() *MemoryDriver {
	return &MemoryDriver{users: map[string]bool{}, units: map[string]string{}, empty: map[string]bool{}}
}

func (d *MemoryDriver) CreateUser(name string) error { d.users[name] = true; return nil }
func (d *MemoryDriver) DeleteUser(name string) error {
	if d.FailDelete {
		return fmt.Errorf("delete user failed")
	}
	delete(d.users, name)
	return nil
}
func (d *MemoryDriver) HasUser(name string) bool { return d.users[name] }
func (d *MemoryDriver) WriteUnit(unit, body string) error {
	d.units[unit] = body
	return nil
}
func (d *MemoryDriver) RemoveUnit(unit string) error { delete(d.units, unit); return nil }
func (d *MemoryDriver) HasUnit(unit string) bool     { return d.units[unit] != "" }
func (d *MemoryDriver) StopCgroup(unit string) error {
	d.empty[unit] = true
	return nil
}
func (d *MemoryDriver) CgroupEmpty(unit string) bool { return d.empty[unit] }
