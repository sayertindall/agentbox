package enrollment

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Record struct {
	EnrollmentID   string `json:"enrollment_id"`
	EnrollmentHash string `json:"enrollment_hash"`
}

func New() (Record, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return Record{}, fmt.Errorf("generate enrollment ID: %w", err)
	}
	bytes[6] = bytes[6]&0x0f | 0x40
	bytes[8] = bytes[8]&0x3f | 0x80
	id := fmt.Sprintf("%s-%s-%s-%s-%s", hex.EncodeToString(bytes[0:4]), hex.EncodeToString(bytes[4:6]), hex.EncodeToString(bytes[6:8]), hex.EncodeToString(bytes[8:10]), hex.EncodeToString(bytes[10:16]))
	return Record{EnrollmentID: id, EnrollmentHash: Hash(id)}, nil
}

func Hash(enrollmentID string) string {
	digest := sha256.Sum256([]byte(enrollmentID))
	return hex.EncodeToString(digest[:])
}

func (record Record) Validate() error {
	if !isUUID(record.EnrollmentID) {
		return fmt.Errorf("invalid enrollment ID")
	}
	if len(record.EnrollmentHash) != sha256.Size*2 || strings.Trim(record.EnrollmentHash, "0123456789abcdef") != "" {
		return fmt.Errorf("invalid enrollment hash")
	}
	if record.EnrollmentHash != Hash(record.EnrollmentID) {
		return fmt.Errorf("enrollment hash does not match enrollment ID")
	}
	return nil
}

func Save(path string, record Record) error {
	if err := record.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal enrollment record: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create enrollment directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".enrollment-*.tmp")
	if err != nil {
		return fmt.Errorf("create enrollment temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("set enrollment permissions: %w", err)
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		return fmt.Errorf("write enrollment record: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync enrollment record: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close enrollment record: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("install enrollment record: %w", err)
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open enrollment directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync enrollment directory: %w", err)
	}
	return nil
}

func Load(path string) (Record, error) {
	file, err := os.Open(path)
	if err != nil {
		return Record{}, fmt.Errorf("open enrollment record: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var record Record
	if err := decoder.Decode(&record); err != nil {
		return Record{}, fmt.Errorf("decode enrollment record: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Record{}, fmt.Errorf("enrollment record contains trailing data")
		}
		return Record{}, fmt.Errorf("decode enrollment record trailing data: %w", err)
	}
	if err := record.Validate(); err != nil {
		return Record{}, err
	}
	return record, nil
}

func isUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' || value[14] != '4' || value[19] != '8' && value[19] != '9' && value[19] != 'a' && value[19] != 'b' {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if !isHex(char) {
			return false
		}
	}
	return true
}

func isHex(char rune) bool {
	return char >= '0' && char <= '9' || char >= 'a' && char <= 'f'
}
