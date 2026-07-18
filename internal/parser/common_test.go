package parser

import "testing"

func TestConversionsAndIdentity(t *testing.T) {
	if v := Float("3,5"); v == nil || *v != 3.5 {
		t.Fatal(v)
	}
	if FIDE("0") != nil {
		t.Fatal("zero FIDE ID must be absent")
	}
	id := PlayerKey(" Example, Player ", nil, StringPtr("LAT"))
	if id == "" || id[:6] != "local:" {
		t.Fatal(id)
	}
}
