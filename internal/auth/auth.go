package auth

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/zalando/go-keyring"
)

const serviceName = "band-cli"

// StorePassword stores password for username in the OS keychain.
func StorePassword(username, password string) error {
	return keyring.Set(serviceName, username, password)
}

// GetPassword retrieves the password for username from the OS keychain.
func GetPassword(username string) (string, error) {
	return keyring.Get(serviceName, username)
}

// DeletePassword removes the credential for username from the OS keychain.
func DeletePassword(username string) error {
	return keyring.Delete(serviceName, username)
}

// EncodeBasicAuth returns the Base64 encoding of "username:password".
func EncodeBasicAuth(username, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}

// DecodeBasicAuth decodes a Base64-encoded "username:password" string and
// returns the username and password separately.
func DecodeBasicAuth(encoded string) (string, string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", fmt.Errorf("decoding basic auth: %w", err)
	}

	parts := strings.SplitN(string(data), ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid basic auth format: missing colon separator")
	}

	return parts[0], parts[1], nil
}
