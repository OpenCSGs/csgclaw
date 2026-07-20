package worklease

import "github.com/google/uuid"

func NewID() string {
	return uuid.NewString()
}

func ValidID(value string) bool {
	if len(value) != 36 {
		return false
	}
	_, err := uuid.Parse(value)
	return err == nil
}
