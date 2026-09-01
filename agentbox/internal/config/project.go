package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"devbox/agentbox/internal/enrollment"
	"devbox/agentbox/internal/id"
)

type Project struct {
	ID           id.ProjectID
	LocalRoot    string
	SSHAliases   []string
	SourcePolicy string
	Providers    []string
}

func Init(root, projectID string) (Project, error) {
	parsedID, err := id.ParseProjectID(projectID)
	if err != nil {
		return Project{}, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return Project{}, fmt.Errorf("resolve local root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return Project{}, fmt.Errorf("stat local root: %w", err)
	}
	if !info.IsDir() {
		return Project{}, fmt.Errorf("local root is not a directory")
	}
	if err := os.MkdirAll(agentboxDir(root), 0o700); err != nil {
		return Project{}, fmt.Errorf("create .agentbox: %w", err)
	}
	projectPath := filepath.Join(agentboxDir(root), "project.toml")
	if _, statErr := os.Stat(projectPath); statErr == nil {
		existingProject, loadErr := Load(root)
		if loadErr != nil {
			return Project{}, loadErr
		}
		if existingProject.ID != parsedID {
			return Project{}, fmt.Errorf("project ID does not match existing configuration")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return Project{}, fmt.Errorf("stat project configuration: %w", statErr)
	}
	enrollmentPath := filepath.Join(agentboxDir(root), "enrollment.json")
	existingEnrollment, enrollmentErr := enrollment.Load(enrollmentPath)
	hasEnrollment := enrollmentErr == nil
	if enrollmentErr != nil && !errors.Is(enrollmentErr, os.ErrNotExist) {
		return Project{}, enrollmentErr
	}
	project := Project{ID: parsedID, LocalRoot: root, SourcePolicy: "allowlist"}
	if err := Save(root, project); err != nil {
		return Project{}, err
	}
	if !hasEnrollment {
		existingEnrollment, err = enrollment.New()
		if err != nil {
			return Project{}, err
		}
		if err := enrollment.Save(enrollmentPath, existingEnrollment); err != nil {
			return Project{}, err
		}
	}
	return project, nil
}

func Load(root string) (Project, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Project{}, fmt.Errorf("resolve local root: %w", err)
	}
	file, err := os.Open(filepath.Join(agentboxDir(root), "project.toml"))
	if err != nil {
		return Project{}, fmt.Errorf("open project configuration: %w", err)
	}
	defer file.Close()
	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Project{}, fmt.Errorf("invalid project configuration line")
		}
		key = strings.TrimSpace(key)
		if key != "project_id" && key != "local_root" && key != "ssh_aliases" && key != "source_policy" && key != "providers" {
			return Project{}, fmt.Errorf("unknown project configuration key %q", key)
		}
		if _, exists := values[key]; exists {
			return Project{}, fmt.Errorf("duplicate project configuration key %q", key)
		}
		values[key] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return Project{}, fmt.Errorf("read project configuration: %w", err)
	}
	projectID, ok := values["project_id"]
	if !ok {
		return Project{}, fmt.Errorf("project configuration has no project_id")
	}
	projectID, err = strconv.Unquote(projectID)
	if err != nil {
		return Project{}, fmt.Errorf("invalid project_id: %w", err)
	}
	parsedID, err := id.ParseProjectID(projectID)
	if err != nil {
		return Project{}, err
	}
	configuredRoot, ok := values["local_root"]
	if !ok {
		return Project{}, fmt.Errorf("project configuration has no local_root")
	}
	configuredRoot, err = strconv.Unquote(configuredRoot)
	if err != nil {
		return Project{}, fmt.Errorf("invalid local_root: %w", err)
	}
	if !sameRoot(configuredRoot, root) {
		return Project{}, fmt.Errorf("project local_root does not match local root")
	}
	project := Project{ID: parsedID, LocalRoot: root, SourcePolicy: "allowlist"}
	if value, ok := values["source_policy"]; ok {
		project.SourcePolicy, err = strconv.Unquote(value)
		if err != nil || project.SourcePolicy != "allowlist" {
			return Project{}, fmt.Errorf("invalid source_policy")
		}
	}
	if value, ok := values["ssh_aliases"]; ok {
		project.SSHAliases, err = parseArray(value)
		if err != nil {
			return Project{}, fmt.Errorf("invalid ssh_aliases: %w", err)
		}
	}
	if value, ok := values["providers"]; ok {
		project.Providers, err = parseArray(value)
		if err != nil {
			return Project{}, fmt.Errorf("invalid providers: %w", err)
		}
	}
	return project, nil
}

func Save(root string, project Project) error {
	if _, err := id.ParseProjectID(project.ID.String()); err != nil {
		return err
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve local root: %w", err)
	}
	if project.LocalRoot == "" {
		project.LocalRoot = root
	}
	if !sameRoot(project.LocalRoot, root) {
		return fmt.Errorf("project local_root does not match local root")
	}
	if project.SourcePolicy == "" {
		project.SourcePolicy = "allowlist"
	}
	if project.SourcePolicy != "allowlist" {
		return fmt.Errorf("invalid source_policy")
	}
	contents := fmt.Sprintf("project_id = %s\nlocal_root = %s\nssh_aliases = %s\nsource_policy = %s\nproviders = %s\n", strconv.Quote(project.ID.String()), strconv.Quote(root), formatArray(project.SSHAliases), strconv.Quote(project.SourcePolicy), formatArray(project.Providers))
	return atomicWrite(filepath.Join(agentboxDir(root), "project.toml"), []byte(contents), 0o600)
}

func agentboxDir(root string) string { return filepath.Join(root, ".agentbox") }

func sameRoot(a, b string) bool {
	left, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	right, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	if filepath.Clean(left) == filepath.Clean(right) {
		return true
	}
	left, err = filepath.EvalSymlinks(left)
	if err != nil {
		return false
	}
	right, err = filepath.EvalSymlinks(right)
	if err != nil {
		return false
	}
	return left == right
}

func atomicWrite(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create configuration directory: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".project-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary configuration: %w", err)
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return fmt.Errorf("set configuration permissions: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		return fmt.Errorf("write configuration: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync configuration: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close configuration: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("install configuration: %w", err)
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open configuration directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync configuration directory: %w", err)
	}
	return nil
}

func formatArray(values []string) string {
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = strconv.Quote(value)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func parseArray(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 2 || value[0] != '[' || value[len(value)-1] != ']' {
		return nil, fmt.Errorf("expected array")
	}
	value = strings.TrimSpace(value[1 : len(value)-1])
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		item, err := strconv.Unquote(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}
