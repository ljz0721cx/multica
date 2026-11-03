package auth

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("secret-pass")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	ok, err := CheckPassword("secret-pass", hash)
	if err != nil || !ok {
		t.Fatalf("CheckPassword want true, got ok=%v err=%v", ok, err)
	}
	ok, err = CheckPassword("wrong-pass!", hash)
	if err != nil {
		t.Fatalf("CheckPassword: %v", err)
	}
	if ok {
		t.Fatal("CheckPassword should reject wrong password")
	}
}

func TestValidatePasswordStrength(t *testing.T) {
	if err := ValidatePasswordStrength("short"); err == nil {
		t.Fatal("expected short password error")
	}
	if err := ValidatePasswordStrength("long-enough"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}
