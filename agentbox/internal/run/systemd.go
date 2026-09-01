package run

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type SystemdDriver struct {
	UnitDir string
}

func (d SystemdDriver) unitPath(unit string) string {
	dir := d.UnitDir
	if dir == "" {
		dir = "/etc/systemd/system"
	}
	return filepath.Join(dir, unit)
}

func (d SystemdDriver) CreateUser(name string) error {
	if d.HasUser(name) {
		return nil
	}
	cmd := exec.Command("useradd", "--system", "--no-create-home", "--shell", "/usr/sbin/nologin", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("useradd %s: %s: %w", name, out, err)
	}
	return nil
}

func (d SystemdDriver) DeleteUser(name string) error {
	cmd := exec.Command("userdel", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("userdel %s: %s: %w", name, out, err)
	}
	return nil
}

func (d SystemdDriver) HasUser(name string) bool {
	return exec.Command("getent", "passwd", name).Run() == nil
}

func (d SystemdDriver) WriteUnit(unit, body string) error {
	return os.WriteFile(d.unitPath(unit), []byte(body), 0o644)
}

func (d SystemdDriver) RemoveUnit(unit string) error {
	_ = exec.Command("systemctl", "disable", "--now", unit).Run()
	_ = os.Remove(d.unitPath(unit))
	return exec.Command("systemctl", "daemon-reload").Run()
}

func (d SystemdDriver) HasUnit(unit string) bool {
	_, err := os.Stat(d.unitPath(unit))
	return err == nil
}

func (d SystemdDriver) StopCgroup(unit string) error {
	cmd := exec.Command("systemctl", "stop", unit)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("stop %s: %s: %w", unit, out, err)
	}
	return nil
}

func (d SystemdDriver) CgroupEmpty(unit string) bool {
	out, err := exec.Command("systemctl", "show", "-p", "ActiveState", "--value", unit).Output()
	if err != nil {
		return false
	}
	state := strings.TrimSpace(string(out))
	return state == "inactive" || state == "failed" || state == "dead"
}
