package configstore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"cnb.cool/dtapp/kai/internal/i18n"
)

// Sensitive fields (api_key / secret) are AES-GCM encrypted before persisting and decrypted
// on read.
// Design goal: even if the config.db file alone is copied/leaked, the plaintext credentials
// cannot be recovered (the key is derived from the local device fingerprint; it is neither in
// the db nor in the repo).
//
// Ciphertext is stored in TEXT columns marked with the "kai:cipher:" prefix; legacy
// plaintext data has no prefix. Decryption branches on the prefix — both staying compatible
// with historical plaintext and avoiding double-encrypting already-encrypted data.

const cipherPrefix = "kai:cipher:"

// Fixed salt: keeps the HKDF derivation stable and bound to this app (not a secret,
// publicly fine).
var hkdfSalt = []byte("kai-configstore-aes-key-salt-v1")

// deriveKey derives a 32-byte AES-256 key from the device fingerprint (HKDF-SHA256,
// Extract+Expand).
// Prefers macOS's IOPlatformUUID (stable and unique); other platforms fall back to
// hostname+machine-id.
// The key is never persisted nor committed — it is derived live on this machine from the
// device fingerprint, so a leaked config.db alone cannot yield the plaintext.
func deriveKey() ([]byte, error) {
	secret, err := deviceSecret()
	if err != nil {
		return nil, err
	}
	// HKDF-Extract: PRK = HMAC-Hash(salt, secret)
	prk := hmacSHA256(hkdfSalt, secret)
	// HKDF-Expand: OKM = T(1) || T(2) ..., fixed info, 32-byte output
	const info = "kai-config-key"
	t := make([]byte, 0, 32)
	block := make([]byte, 32)
	var counter byte = 1
	for len(t) < 32 {
		h := hmacSHA256(prk, append(append([]byte{}, counter), info...))
		copy(block, h)
		t = append(t, block[:]...)
		counter++
	}
	key := make([]byte, 32)
	copy(key, t[:32])
	return key, nil
}

// hmacSHA256 returns HMAC-SHA256(secret, msg).
func hmacSHA256(secret, msg []byte) []byte {
	h := hmac.New(sha256.New, secret)
	h.Write(msg)
	return h.Sum(nil)
}

// deviceSecret returns this machine's stable fingerprint.
func deviceSecret() ([]byte, error) {
	var raw string
	switch runtime.GOOS {
	case "darwin":
		// IOPlatformUUID stays stable even across OS reinstalls — an ideal device-binding
		// source.
		out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").
			Output()
		if err == nil {
			s := string(out)
			if idx := strings.Index(s, "\"IOPlatformUUID\""); idx >= 0 {
				rest := s[idx:]
				if _, after, ok := strings.Cut(rest, "\""); ok {
					rest2 := after
					if before, _, ok := strings.Cut(rest2, "\""); ok {
						candidate := before
						if candidate != "" {
							raw = candidate
						}
					}
				}
			}
		}
	case "linux":
		if b, err := os.ReadFile("/etc/machine-id"); err == nil {
			raw = strings.TrimSpace(string(b))
		}
	}
	if raw == "" {
		// Fallback: hostname (works cross-platform; less stable than UUID but never crashes).
		if h, err := exec.Command("hostname").Output(); err == nil {
			raw = strings.TrimSpace(string(h))
		}
	}
	if raw == "" {
		return nil, errors.New(i18n.T("err.configstore_device_fp"))
	}
	return []byte(raw), nil
}

// EncryptSecret encrypts a sensitive field; an empty string returns empty (no encrypting
// empty values).
func EncryptSecret(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	key, err := deriveKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return cipherPrefix + base64.StdEncoding.EncodeToString(ct), nil
}

// DecryptSecret decrypts a sensitive field.
//   - empty string returns empty;
//   - without cipherPrefix it is treated as legacy plaintext, returned as-is (compatible with
//     pre-migration data);
//   - on decryption failure (corrupted data or device change) an error is returned for the
//     caller to log.
func DecryptSecret(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, cipherPrefix) {
		// Legacy plaintext data: keep compatible, return as-is.
		return stored, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, cipherPrefix))
	if err != nil {
		return "", fmt.Errorf("%s: %w", i18n.T("err.configstore_cipher_base64"), err)
	}
	key, err := deriveKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return "", errors.New(i18n.T("err.configstore_cipher_too_short"))
	}
	nonce, ct := raw[:ns], raw[ns:]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("%s: %w", i18n.T("err.configstore_cipher_decrypt"), err)
	}
	return string(plain), nil
}
