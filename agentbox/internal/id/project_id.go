package id

import "fmt"

type ProjectID string

func ParseProjectID(value string) (ProjectID, error) {
	if len(value) == 0 || len(value) > 64 {
		return "", fmt.Errorf("project ID must be 1 to 64 characters")
	}
	for index := 0; index < len(value); index++ {
		char := value[index]
		if index == 0 {
			if !isLowerAlpha(char) && !isDigit(char) {
				return "", fmt.Errorf("project ID must start with a lowercase letter or digit")
			}
			continue
		}
		if !isLowerAlpha(char) && !isDigit(char) && char != '-' {
			return "", fmt.Errorf("project ID contains an invalid character")
		}
	}
	return ProjectID(value), nil
}

func (id ProjectID) String() string { return string(id) }

func isLowerAlpha(char byte) bool { return char >= 'a' && char <= 'z' }
func isDigit(char byte) bool      { return char >= '0' && char <= '9' }
