package auth

import (
	"fmt"

	"github.com/alexedwards/argon2id"
)

func HashPassword(password string) (string, error) {
	hash, errHash := argon2id.CreateHash(password, argon2id.DefaultParams)
	if errHash != nil {
		return "", fmt.Errorf("Failed to hash password!")
	}

	return hash, nil
}

func CheckPasswordHash(password, hash string) (bool, error) {
	match, errCheck := argon2id.ComparePasswordAndHash(password, hash)

	if errCheck != nil {
		return false, fmt.Errorf("Could not check password w/ hash")
	}

	return match, nil
}
