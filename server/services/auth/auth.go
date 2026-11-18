package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/utils/context"
	log "github.com/sirupsen/logrus"
)

// HasAuth checks whether the current user has auth for an email
func HasAuth(ctx *context.Context, email *models.Email) bool {
	if ctx.IsAdmin {
		return true
	}
	var ue []models.UserEmail
	err := db.Instance.Table(&models.UserEmail{}).Where("email_id = ? and user_id = ?", email.Id, ctx.UserID).Find(&ue)
	if err != nil {
		log.Errorf("Error while checking user: %v", err)
		return false
	}

	return len(ue) != 0
}

func DkimGen() string {
	privatePath := config.Instance.DkimPrivateKeyPath
	if strings.TrimSpace(privatePath) == "" {
		privatePath = filepath.Join(config.ROOT_PATH, "config", "dkim", "dkim.priv")
	}
	publicPath := filepath.Join(filepath.Dir(privatePath), "dkim.public")
	privKeyStr, _ := os.ReadFile(privatePath)
	publicKeyStr, _ := os.ReadFile(publicPath)
	if len(privKeyStr) > 0 && len(publicKeyStr) > 0 {
		return string(publicKeyStr)
	}

	var (
		privKey crypto.Signer
		err     error
	)

	privKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatalf("Failed to generate key: %v", err)
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		log.Fatalf("Failed to marshal private key: %v", err)
	}

	_ = os.MkdirAll(filepath.Dir(privatePath), 0o700)
	f, err := os.OpenFile(privatePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		log.Fatalf("Failed to create key file: %v", err)
	}
	defer f.Close()

	privBlock := pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privBytes,
	}
	if err := pem.Encode(f, &privBlock); err != nil {
		log.Fatalf("Failed to write key PEM block: %v", err)
	}
	if err := f.Close(); err != nil {
		log.Fatalf("Failed to close key file: %v", err)
	}

	var pubBytes []byte

	switch pubKey := privKey.Public().(type) {
	case *rsa.PublicKey:
		// RFC 6376 is inconsistent about whether RSA public keys should
		// be formatted as RSAPublicKey or SubjectPublicKeyInfo.
		// Erratum 3017 (https://www.rfc-editor.org/errata/eid3017)
		// proposes allowing both.  We use SubjectPublicKeyInfo for
		// consistency with other implementations including opendkim,
		// Gmail, and Fastmail.
		pubBytes, err = x509.MarshalPKIXPublicKey(pubKey)
		if err != nil {
			log.Fatalf("Failed to marshal public key: %v", err)
		}
	default:
		panic("unreachable")
	}

	params := []string{
		"v=DKIM1",
		"k=rsa",
		"p=" + base64.StdEncoding.EncodeToString(pubBytes),
	}

	publicKey := strings.Join(params, "; ")

	os.WriteFile(publicPath, []byte(publicKey), 0o644)

	return publicKey
}
