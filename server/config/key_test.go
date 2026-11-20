package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadPrivateKeyRegeneratesACorruptAccountKey(t *testing.T) {
	oldRoot := ROOT_PATH
	ROOT_PATH = t.TempDir() + string(os.PathSeparator)
	t.Cleanup(func() { ROOT_PATH = oldRoot })

	dir := filepath.Join(ROOT_PATH, "config", "ssl")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "account_private.pem")
	if err := os.WriteFile(path, []byte("not a pem"), 0o600); err != nil {
		t.Fatal(err)
	}

	key, generated := ReadPrivateKey()
	if key == nil || !generated {
		t.Fatal("ReadPrivateKey did not replace a corrupt account key")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("replacement key was not written: %v", err)
	}
	if _, generatedAgain := ReadPrivateKey(); generatedAgain {
		t.Fatal("the newly written account key was not accepted")
	}
}
