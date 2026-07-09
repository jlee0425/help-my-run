package auth

import (
	"strings"
	"testing"
)

func TestHashVerifyRoundTrip(t *testing.T) {
	enc, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(enc, "$argon2id$v=19$") {
		t.Fatalf("encoding = %q, want $argon2id$v=19$ prefix", enc)
	}
	if !VerifyPassword("correct horse battery staple", enc) {
		t.Fatalf("VerifyPassword(correct) = false")
	}
	if VerifyPassword("wrong password", enc) {
		t.Fatalf("VerifyPassword(wrong) = true")
	}
	// Two hashes of the same password differ (random salt).
	enc2, _ := HashPassword("correct horse battery staple")
	if enc == enc2 {
		t.Fatalf("two hashes identical — salt not random")
	}
}

func TestVerifyPasswordMalformed(t *testing.T) {
	for _, enc := range []string{"", "plaintext", "$argon2id$v=19$m=65536,t=1,p=4$notb64!!$x", "$argon2i$v=19$m=65536,t=1,p=4$YWJj$YWJj"} {
		if VerifyPassword("x", enc) {
			t.Errorf("VerifyPassword(malformed %q) = true, want false", enc)
		}
	}
}

func TestNewSecret(t *testing.T) {
	plain, hash := NewSecret("hmr_")
	if !strings.HasPrefix(plain, "hmr_") {
		t.Fatalf("plain = %q, want hmr_ prefix", plain)
	}
	if len(plain) != len("hmr_")+64 {
		t.Fatalf("plain len = %d, want prefix+64 hex chars", len(plain))
	}
	if len(hash) != 64 {
		t.Fatalf("hash len = %d, want 64 (sha256 hex)", len(hash))
	}
	if HashSecret(plain) != hash {
		t.Fatalf("HashSecret(plain) != returned hash")
	}
	plain2, _ := NewSecret("hmr_")
	if plain == plain2 {
		t.Fatalf("two secrets identical")
	}
	// No-prefix variant (session ids).
	p3, h3 := NewSecret("")
	if len(p3) != 64 || len(h3) != 64 {
		t.Fatalf("unprefixed secret lens = %d,%d want 64,64", len(p3), len(h3))
	}
}
