package auth

import (
	"strings"
	"testing"
	"time"
)

func TestHashAndVerify(t *testing.T) {
	pw := "Correct-Horse-9-Battery"
	h, err := HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=32768,t=3,p=2$") {
		t.Fatalf("unexpected PHC prefix: %s", h)
	}
	if len(strings.Split(h, "$")) != 6 {
		t.Fatalf("unexpected PHC shape: %s", h)
	}
	ok, err := VerifyPassword(pw, h)
	if err != nil || !ok {
		t.Fatalf("verify correct: ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword("Wrong-Horse-9-Battery", h)
	if err != nil || ok {
		t.Fatalf("verify wrong: ok=%v err=%v", ok, err)
	}
}

func TestHashSaltsDiffer(t *testing.T) {
	a, _ := HashPassword("Correct-Horse-9-Battery")
	b, _ := HashPassword("Correct-Horse-9-Battery")
	if a == b {
		t.Fatal("two hashes of the same password must differ")
	}
}

func TestHashRejectsWeak(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("expected weak password error")
	}
}

func TestVerifyInvalidHash(t *testing.T) {
	for _, h := range []string{
		"",
		"plain",
		"$argon2i$v=19$m=32768,t=3,p=2$c2FsdA$a2V5",
		"$argon2id$v=18$m=32768,t=3,p=2$c2FsdA$a2V5",
		"$argon2id$v=19$m=nope$c2FsdA$a2V5",
		"$argon2id$v=19$m=999999999,t=3,p=2$c2FsdA$a2V5",
		"$argon2id$v=19$m=32768,t=3,p=2$!!!$a2V5",
		"$argon2id$v=19$m=32768,t=3,p=2$c2FsdA$!!!",
	} {
		if ok, err := VerifyPassword("whatever", h); err == nil || ok {
			t.Errorf("hash %q: want error, got ok=%v err=%v", h, ok, err)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		pw string
		ok bool
	}{
		{"Abcdefgh1234!", true},
		{"Abcdefgh123!", true},
		{"Abcdefgh12!", false},
		{"abcdefgh1234!", false},
		{"ABCDEFGH1234!", false},
		{"Abcdefghijkl!", false},
		{"Abcdefgh12345", false},
		{"Pässwörter-12345", true},
		{strings.Repeat("Aa1!", 100), false},
		{"", false},
	}
	for _, c := range cases {
		err := ValidatePassword(c.pw)
		if (err == nil) != c.ok {
			t.Errorf("ValidatePassword(%q) err=%v, want ok=%v", c.pw, err, c.ok)
		}
	}
}

func TestDummyVerify(t *testing.T) {
	DummyVerify("anything")
}

func TestHashingConcurrencyIsBounded(t *testing.T) {
	h, err := HashPassword("Correct-Horse-9-Battery")
	if err != nil {
		t.Fatal(err)
	}
	if cap(hashSlots) != 4 {
		t.Fatalf("cap = %d, want 4", cap(hashSlots))
	}
	for i := 0; i < cap(hashSlots); i++ {
		hashSlots <- struct{}{}
	}
	done := make(chan struct{})
	go func() {
		_, _ = VerifyPassword("Correct-Horse-9-Battery", h)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("verify must wait while all hashing slots are taken")
	case <-time.After(150 * time.Millisecond):
	}
	<-hashSlots
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("verify must proceed once a slot is released")
	}
	for i := 1; i < cap(hashSlots); i++ {
		<-hashSlots
	}
	if len(hashSlots) != 0 {
		t.Fatalf("slots leaked: %d", len(hashSlots))
	}
}
