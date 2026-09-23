package vpp

import (
	"errors"
	"strings"
	"testing"
)

func TestOwnerTagRoundTrip(t *testing.T) {
	tag, err := OwnerTag("w2", "loop200")
	if err != nil || tag != "w2:loop200" {
		t.Fatalf("OwnerTag = %q, %v", tag, err)
	}
	id, ok := ParseOwnerTag(tag+"\x00\x00", "w2")
	if !ok || id != "loop200" {
		t.Fatalf("ParseOwnerTag = %q, %v", id, ok)
	}
	if _, ok := ParseOwnerTag(tag, "w3"); ok {
		t.Fatal("tag of another owner must not parse")
	}
	if _, ok := ParseOwnerTag("w2:", "w2"); ok {
		t.Fatal("empty id must not parse")
	}
	if _, ok := ParseOwnerTag("w20:x", "w2"); ok {
		t.Fatal("owner prefix must match up to the colon")
	}
	if _, ok := ParseOwnerTag("anything", ""); ok {
		t.Fatal("empty owner must never match")
	}
}

func TestOwnerTagRejects(t *testing.T) {
	if _, err := OwnerTag("", "x"); err == nil {
		t.Fatal("empty owner accepted")
	}
	if _, err := OwnerTag("w2", ""); err == nil {
		t.Fatal("empty id accepted")
	}
	if _, err := OwnerTag("w:2", "x"); err == nil {
		t.Fatal("colon in owner accepted")
	}
	if _, err := OwnerTag("w2", "a\nb"); err == nil {
		t.Fatal("newline in id accepted")
	}
	_, err := OwnerTag("w2", strings.Repeat("x", 61))
	if !errors.Is(err, ErrTagTooLong) {
		t.Fatalf("64-byte tag: got %v, want ErrTagTooLong", err)
	}
	if tag, err := OwnerTag("w2", strings.Repeat("x", 60)); err != nil || len(tag) != MaxTagLen {
		t.Fatalf("63-byte tag must be accepted: %q %v", tag, err)
	}
}
