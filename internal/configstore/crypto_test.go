package configstore

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	cases := []string{"", "sk-1234567890abcdef", "appid|secret-key", "pässwörd/secret!@#€"}
	for _, plain := range cases {
		enc, err := EncryptSecret(plain)
		if err != nil {
			t.Fatalf("encrypt %q: %v", plain, err)
		}
		if plain == "" {
			if enc != "" {
				t.Fatalf("empty input should return empty, got %q", enc)
			}
			continue
		}
		if enc == plain {
			t.Fatalf("ciphertext should not equal plaintext: %q", enc)
		}
		if !strings.HasPrefix(enc, cipherPrefix) {
			t.Fatalf("ciphertext should carry the prefix: %q", enc)
		}
		dec, err := DecryptSecret(enc)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if dec != plain {
			t.Fatalf("roundtrip mismatch: want %q got %q", plain, dec)
		}
	}
}

func TestDecryptLegacyPlaintext(t *testing.T) {
	// Legacy plaintext data (no prefix) should be returned as-is, keeping pre-migration
	// data readable.
	plain := "old-plain-secret"
	dec, err := DecryptSecret(plain)
	if err != nil {
		t.Fatalf("decrypt legacy: %v", err)
	}
	if dec != plain {
		t.Fatalf("legacy data should be returned as-is, got %q", dec)
	}
}

func TestDeriveKeyStable(t *testing.T) {
	k1, err := deriveKey()
	if err != nil {
		t.Fatalf("deriveKey: %v", err)
	}
	k2, err := deriveKey()
	if err != nil {
		t.Fatalf("deriveKey: %v", err)
	}
	if len(k1) != 32 {
		t.Fatalf("derived key length should be 32, got %d", len(k1))
	}
	for i := range k1 {
		if k1[i] != k2[i] {
			t.Fatalf("derived key should be stable on the same device")
		}
	}
}
