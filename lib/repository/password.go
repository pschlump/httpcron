package repository

import "golang.org/x/crypto/bcrypt"

func HashPasswordBcrypt(password string) (string, error) {
	if len(password) > 72 {
		password = password[0:72]
	}
	// A cost of 14 is a strong default for modern hardware
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

// CheckPasswordHash compares a plain-text password with a bcrypt hash.
// It returns true if they match, false otherwise.
func CheckPasswordHash(password, hash string) bool {
	if len(password) > 72 {
		password = password[0:72]
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
