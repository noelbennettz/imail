package password

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"strings"
)

const (
	pbkdf2Prefix     = "pbkdf2-sha256"
	pbkdf2Iterations = 210000
	pbkdf2SaltLength = 16
	pbkdf2KeyLength  = 32
)

// Encode returns a salted PBKDF2-HMAC-SHA256 hash suitable for storage.
func Encode(password string) string {
	salt := make([]byte, pbkdf2SaltLength)
	if _, err := rand.Read(salt); err != nil {
		panic(err)
	}
	derived := pbkdf2Key([]byte(password), salt, pbkdf2Iterations, pbkdf2KeyLength)
	return pbkdf2Prefix + "$" + strconv.Itoa(pbkdf2Iterations) + "$" +
		base64.RawStdEncoding.EncodeToString(salt) + "$" +
		base64.RawStdEncoding.EncodeToString(derived)
}

// Verify checks a password against a stored PBKDF2 hash or a legacy MD5 hash.
// The second return value is true when the stored hash should be replaced with
// a PBKDF2 hash after a successful login.
func Verify(password, stored string) (bool, bool) {
	if strings.HasPrefix(stored, pbkdf2Prefix+"$") {
		parts := strings.Split(stored, "$")
		if len(parts) != 4 {
			return false, false
		}
		iterations, err := strconv.Atoi(parts[1])
		if err != nil || iterations <= 0 {
			return false, false
		}
		salt, err := base64.RawStdEncoding.DecodeString(parts[2])
		if err != nil || len(salt) == 0 {
			return false, false
		}
		want, err := base64.RawStdEncoding.DecodeString(parts[3])
		if err != nil || len(want) == 0 {
			return false, false
		}
		got := pbkdf2Key([]byte(password), salt, iterations, len(want))
		return subtle.ConstantTimeCompare(got, want) == 1, false
	}

	// Legacy databases used a fixed-salt double MD5. Verify it once so the
	// account can be upgraded on the next successful login.
	if len(stored) == 32 && subtle.ConstantTimeCompare([]byte(EncodeLegacy(password)), []byte(stored)) == 1 {
		return true, true
	}
	return false, false
}

// EncodeLegacy is kept only for verifying and migrating pre-PBKDF2 hashes.
func EncodeLegacy(password string) string {
	return Md5Encode(Md5Encode(password+"imail") + "imail2023")
}

func Md5Encode(str string) string {
	h := md5.New()
	h.Write([]byte(str))
	return hex.EncodeToString(h.Sum(nil))
}

func pbkdf2Key(password, salt []byte, iterations, keyLength int) []byte {
	hashLength := sha256.Size
	blockCount := (keyLength + hashLength - 1) / hashLength
	output := make([]byte, 0, blockCount*hashLength)

	for block := 1; block <= blockCount; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		var index [4]byte
		binary.BigEndian.PutUint32(index[:], uint32(block))
		mac.Write(index[:])
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)

		for iteration := 1; iteration < iterations; iteration++ {
			mac.Reset()
			mac.Write(u)
			u = mac.Sum(nil)
			for i := range t {
				t[i] ^= u[i]
			}
		}
		output = append(output, t...)
	}
	return output[:keyLength]
}
