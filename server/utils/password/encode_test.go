package password

import "testing"

func TestEncodeVerifyAndLegacyMigration(t *testing.T) {
	encoded := Encode("correct horse battery staple")
	if encoded == "" || len(encoded) <= 32 {
		t.Fatalf("unexpected hash %q", encoded)
	}
	ok, rehash := Verify("correct horse battery staple", encoded)
	if !ok || rehash {
		t.Fatalf("Verify() = %v, %v; want true, false", ok, rehash)
	}
	if ok, _ := Verify("wrong password", encoded); ok {
		t.Fatal("wrong password accepted")
	}

	legacy := EncodeLegacy("legacy-password")
	ok, rehash = Verify("legacy-password", legacy)
	if !ok || !rehash {
		t.Fatalf("legacy Verify() = %v, %v; want true, true", ok, rehash)
	}
	if ok, _ := Verify("wrong password", legacy); ok {
		t.Fatal("wrong legacy password accepted")
	}
}
