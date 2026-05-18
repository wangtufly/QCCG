package account

import (
	"fmt"

	keychain "github.com/zalando/go-keyring"
)

const keychainService = "qccg"

func SaveSecret(accountID, secret string) error {
	return keychain.Set(keychainService, accountID, secret)
}

func GetSecret(accountID string) (string, error) {
	s, err := keychain.Get(keychainService, accountID)
	if err != nil {
		return "", fmt.Errorf("keychain get %s: %w", accountID, err)
	}
	return s, nil
}

func DeleteSecret(accountID string) error {
	return keychain.Delete(keychainService, accountID)
}
