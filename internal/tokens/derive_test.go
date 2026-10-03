package tokens

import "testing"

func TestIsTokenAward(t *testing.T) {
	if !IsTokenAward("Weiheabzeichen des Beschützers") {
		t.Fatal("should match")
	}
	if !IsTokenAward("weiheabzeichen des bezwingers") {
		t.Fatal("should match lower")
	}
	if !IsTokenAward("Weiheabzeichen des Eroberers") {
		t.Fatal("should match")
	}
	if IsTokenAward("Blutfall") {
		t.Fatal("no")
	}
}

func TestDeriveTokenAmount(t *testing.T) {
	if DeriveTokenAmount(0) != 1 {
		t.Fatal("default 1")
	}
	if DeriveTokenAmount(2) != 2 {
		t.Fatal("count")
	}
}
