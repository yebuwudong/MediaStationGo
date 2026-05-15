package service

import (
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestCryptoRoundtrip(t *testing.T) {
	c := NewCryptoService("super-secret-key-1234567890", zap.NewNop())
	cases := []string{
		"",
		"a",
		"hello world",
		"sk-1234567890abcdef1234567890abcdef1234567890abcdef",
	}
	for _, plain := range cases {
		t.Run(plain, func(t *testing.T) {
			cipher := c.Encrypt(plain)
			if plain == "" {
				if cipher != "" {
					t.Fatalf("empty plaintext should round-trip empty, got %q", cipher)
				}
				return
			}
			if cipher == plain {
				t.Fatalf("expected ciphertext to differ from plaintext")
			}
			if !strings.HasPrefix(cipher, "enc:v1:") {
				t.Fatalf("expected enc:v1: prefix, got %q", cipher)
			}
			plain2 := c.Decrypt(cipher)
			if plain2 != plain {
				t.Fatalf("decrypt mismatch: got %q, want %q", plain2, plain)
			}
		})
	}
}

func TestCryptoNoSecret(t *testing.T) {
	c := NewCryptoService("", zap.NewNop())
	if c.Encrypt("x") != "x" {
		t.Fatal("empty-secret crypto should be a pass-through")
	}
	if c.Decrypt("x") != "x" {
		t.Fatal("empty-secret crypto should be a pass-through")
	}
}

func TestCryptoLegacyPlaintext(t *testing.T) {
	c := NewCryptoService("k", zap.NewNop())
	// Decrypt a value that has no enc:v1: prefix — should pass through.
	if got := c.Decrypt("legacy-plain"); got != "legacy-plain" {
		t.Fatalf("legacy plaintext should pass through, got %q", got)
	}
}

func TestMaskAPIKey(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "****"},
		{"abc", "****"},
		{"abcdefgh", "abcd****efgh"},
		{"sk-1234567890abcdef", "sk-1****cdef"},
	}
	for _, tc := range cases {
		got := MaskAPIKey(tc.in)
		if got != tc.want {
			t.Errorf("MaskAPIKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
