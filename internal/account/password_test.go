package account

import (
	"bytes"
	"testing"
)

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := CheckPassword(h, "correct horse battery"); !ok || err != nil {
		t.Errorf("right password: %v %v", ok, err)
	}
	if ok, _ := CheckPassword(h, "wrong horse battery"); ok {
		t.Error("wrong password accepted")
	}
	if h2, _ := HashPassword("correct horse battery"); h2 == h {
		t.Error("two hashes of one password are equal; salt not random")
	}
	if _, err := CheckPassword("md5$abc", "x"); err == nil {
		t.Error("unknown format accepted")
	}
}

func TestSessionToken(t *testing.T) {
	token, hash, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 40 || !bytes.Equal(hash, hashToken(token)) {
		t.Errorf("token %q hash %x", token, hash)
	}
}
