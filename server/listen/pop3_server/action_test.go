package pop3_server

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/emersion/go-message/mail"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/third_party/gopop"
	"github.com/noelbennettz/imail/utils/context"
)

func Test_action_Retr(t *testing.T) {
	wd, _ := os.Getwd()
	config.ROOT_PATH = filepath.Clean(filepath.Join(wd, "../..")) + string(os.PathSeparator)
	config.Init()
	config.Instance.Domain = "domain.com"
	config.Instance.Domains = []string{"domain.com"}
	config.Instance.DbType = config.DBTypeSQLite
	config.Instance.DbDSN = filepath.Join(config.ROOT_PATH, "config", "imail_temp.db")
	db.Init("")

	a := action{}
	session := &gopop.Session{
		Ctx: &context.Context{
			UserID: 1,
		},
	}
	got, got1, err := a.Retr(session, 301)

	_, _, _ = got, got1, err
}

func Test_email(t *testing.T) {
	var b bytes.Buffer

	// Create our mail header
	var h mail.Header

	// Create a new mail writer
	mw, _ := mail.CreateWriter(&b, h)

	// Create a text part
	tw, _ := mw.CreateInline()

	var html mail.InlineHeader

	html.Header.Set("Content-Transfer-Encoding", "base64")
	w, _ := tw.CreatePart(html)

	io.WriteString(w, "=")

	w.Close()

	tw.Close()

	fmt.Printf("%s", b.String())
}
