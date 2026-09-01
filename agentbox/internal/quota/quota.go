package quota

import "fmt"

type Backend interface {
	Assign(root string, maxBytes int64) error
	Charge(root string, bytes int64) error
}

type Memory struct {
	max    map[string]int64
	used   map[string]int64
}

func NewMemory() *Memory {
	return &Memory{max: map[string]int64{}, used: map[string]int64{}}
}

func (m *Memory) Assign(root string, maxBytes int64) error {
	if root == "" || maxBytes <= 0 {
		return fmt.Errorf("project quota assignment requires a root and positive limit")
	}
	m.max[root] = maxBytes
	m.used[root] = 0
	return nil
}

func (m *Memory) Charge(root string, bytes int64) error {
	max, ok := m.max[root]
	if !ok {
		return fmt.Errorf("no project quota assigned")
	}
	if bytes < 0 {
		return fmt.Errorf("negative write")
	}
	if m.used[root]+bytes > max {
		return fmt.Errorf("project quota exceeded")
	}
	m.used[root] += bytes
	return nil
}
