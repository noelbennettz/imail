package password

import (
	"sync"
	"testing"
	"time"
)

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

func TestVerifyConcurrencyIsBounded(t *testing.T) {
	encoded := Encode("concurrency-bound")
	original := derive
	var mu sync.Mutex
	inFlight, peak := 0, 0
	derive = func(password, salt []byte, iterations, keyLength int) []byte {
		mu.Lock()
		inFlight++
		if inFlight > peak {
			peak = inFlight
		}
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
		return original(password, salt, iterations, keyLength)
	}
	t.Cleanup(func() { derive = original })

	var wg sync.WaitGroup
	for i := 0; i < VerifyConcurrency*5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := Verify("concurrency-bound", encoded); !ok {
				t.Error("a correct password was rejected while the verifier was saturated")
			}
		}()
	}
	wg.Wait()

	if peak > VerifyConcurrency {
		t.Fatalf("concurrent verifications peaked at %d, want at most %d", peak, VerifyConcurrency)
	}
	if peak < 2 {
		t.Fatalf("concurrent verifications peaked at %d, want the semaphore to allow parallelism", peak)
	}
}
