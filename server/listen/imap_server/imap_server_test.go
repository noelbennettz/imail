package imap_server

import (
	"bufio"
	"crypto/tls"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/utils/array"
	pcontext "github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/password"
)

var (
	clientUnLogin *imapclient.Client
	clientLogin   *imapclient.Client
	imapTestAddr  string
)

func TestMain(m *testing.M) {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	config.ROOT_PATH = filepath.Clean(filepath.Join(wd, "../..")) + string(os.PathSeparator)
	config.Init()
	config.Instance.Domain = "domain.com"
	config.Instance.Domains = []string{"domain.com"}
	config.Instance.WebDomain = "mail.domain.com"
	config.Instance.DbType = config.DBTypeSQLite
	config.Instance.DbDSN = filepath.Join(config.ROOT_PATH, "config", "imail_temp.db")
	// The IMAP tests create and expunge fixed UIDs, so each run must start from
	// an empty database instead of whatever the previous run left behind.
	_ = os.Remove(config.Instance.DbDSN)
	config.Instance.SSLPublicKeyPath = filepath.Join(config.ROOT_PATH, "config", "ssl", "public.crt")
	config.Instance.SSLPrivateKeyPath = filepath.Join(config.ROOT_PATH, "config", "ssl", "private.key")
	if err := db.Init(""); err != nil {
		panic(err)
	}
	seedIMAPTestData()
	crt, err := tls.LoadX509KeyPair(config.Instance.SSLPublicKeyPath, config.Instance.SSLPrivateKeyPath)
	if err != nil {
		panic(err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{crt}})
	if err != nil {
		panic(err)
	}
	imapTestAddr = ln.Addr().String()
	instanceTLS = newIMAPServer()
	go func() {
		if err := instanceTLS.Serve(ln); err != nil {
			panic(err)
		}
	}()

	options := &imapclient.Options{
		WordDecoder: &mime.WordDecoder{CharsetReader: charset.Reader},
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	clientUnLogin, err = imapclient.DialTLS(imapTestAddr, options)
	if err != nil {
		panic(err)
	}

	clientLogin, err = imapclient.DialTLS(imapTestAddr, options)
	if err != nil {
		panic(err)
	}

	err = clientLogin.Login("testCase", "testCase").Wait()
	if err != nil {
		panic(err)
	}

	code := m.Run()
	clientUnLogin.Close()
	clientLogin.Close()
	Stop()
	os.Exit(code)
}

func seedIMAPTestData() {
	var user models.User
	_, err := db.Instance.Table(&models.User{}).Where("account=?", "testCase").Get(&user)
	if err != nil {
		panic(err)
	}
	if user.ID == 0 {
		user = models.User{Account: "testCase", Name: "testCase", Password: password.Encode("testCase")}
		if _, err = db.Instance.Insert(&user); err != nil {
			panic(err)
		}
	}

	var num int
	_, err = db.Instance.Table(&models.UserEmail{}).Where("user_id=?", user.ID).Select("count(1)").Get(&num)
	if err != nil {
		panic(err)
	}
	if num > 0 {
		return
	}

	statuses := []int8{3}
	for i := 0; i < 10; i++ {
		statuses = append(statuses, 0)
	}
	for _, status := range statuses {
		email := models.Email{Subject: "test", Status: status}
		if _, err = db.Instance.Insert(&email); err != nil {
			panic(err)
		}
		ue := models.UserEmail{UserID: user.ID, EmailID: email.Id, Status: status}
		if _, err = db.Instance.Insert(&ue); err != nil {
			panic(err)
		}
	}
}

func TestCapability(t *testing.T) {
	res, err := clientUnLogin.Capability().Wait()
	if err != nil {
		t.Error(err)
	}
	if _, ok := res["IMAP4rev1"]; !ok {
		t.Error("Capability Error")
	}

	res, err = clientLogin.Capability().Wait()
	if err != nil {
		t.Error(err)
	}
	if _, ok := res["IDLE"]; !ok {
		t.Error("IDLE Capability Error")
	}
}

func TestLogin(t *testing.T) {
	err := clientUnLogin.Login("testCase", "testCaseasdfsadf").Wait()
	sErr := err.(*imap.Error)
	if sErr.Code != "AUTHENTICATIONFAILED" {
		t.Error("Login Error")
	}
}

func TestCreate(t *testing.T) {
	err := clientLogin.Create("一级菜单", nil).Wait()
	if err != nil {
		t.Error(err)
	}

	err = clientLogin.Create("一级菜单/二级菜单", nil).Wait()
	if err != nil {
		t.Error(err)
	}

	res, err := clientLogin.List("", "*", nil).Collect()
	if err != nil {
		t.Error(err)
	}
	var mailbox []string
	for _, v := range res {
		mailbox = append(mailbox, v.Mailbox)
	}

	if !array.InArray("一级菜单", mailbox) || !array.InArray("一级菜单/二级菜单", mailbox) {
		t.Error(mailbox)
	}
}

func TestRename(t *testing.T) {
	err := clientLogin.Rename("一级菜单", "主菜单").Wait()
	if err != nil {
		t.Error(err)
	}

	res, err := clientLogin.List("", "*", nil).Collect()
	if err != nil {
		t.Error(err)
	}
	var mailbox []string
	for _, v := range res {
		mailbox = append(mailbox, v.Mailbox)
	}

	if !array.InArray("主菜单", mailbox) {
		t.Error(mailbox)
	}
}

func TestList(t *testing.T) {
	res, err := clientUnLogin.List("", "", &imap.ListOptions{}).Collect()

	if err == nil {
		t.Logf("%+v", res)
		t.Error("List Unlogin error")
	}

	res, err = clientLogin.List("", "", &imap.ListOptions{}).Collect()
	if err != nil {
		t.Error(err)
	}
	if len(res) != 0 {
		t.Errorf("LIST with an empty pattern should not match any mailbox: %+v", res)
	}

	res, err = clientLogin.List("", "*", &imap.ListOptions{}).Collect()
	if err != nil {
		t.Error(err)
	}
	if len(res) == 0 {
		t.Error("List Error")
	}

	res, err = clientLogin.List("", "主菜单/%", &imap.ListOptions{}).Collect()
	if err != nil {
		t.Error(err)
	}
	if len(res) == 0 {
		t.Error("List Error")
	}

	if len(res) != 1 {
		t.Error("List Error")
	}

	res, err = clientLogin.List("", "主菜单/*", &imap.ListOptions{}).Collect()
	if err != nil {
		t.Error(err)
	}
	if len(res) != 1 {
		t.Error("List Error")
	}
	if len(res) == 0 {
		t.Error("List Error")
	}

	res, err = clientLogin.List("", "%", &imap.ListOptions{}).Collect()
	if err != nil {
		t.Error(err)
	}
	if !containsMailbox(res, "INBOX") {
		t.Errorf("LIST %%%% did not return INBOX: %+v", res)
	}
}

func containsMailbox(list []*imap.ListData, name string) bool {
	for _, item := range list {
		if item.Mailbox == name {
			return true
		}
	}
	return false
}

func TestDelete(t *testing.T) {
	clientLogin.Create("一级菜单/二级菜单", nil).Wait()

	err := clientLogin.Delete("一级菜单/二级菜单").Wait()
	if err != nil {
		t.Error(err)
	}
	res, err := clientLogin.List("", "*", nil).Collect()
	if err != nil {
		t.Error(err)
	}
	var mailbox []string
	for _, v := range res {
		mailbox = append(mailbox, v.Mailbox)
	}

	if array.InArray("二级菜单", mailbox) {
		t.Error(mailbox)
	}
}

func TestSelect(t *testing.T) {
	res, err := clientUnLogin.Select("INBOX", &imap.SelectOptions{}).Wait()
	if err == nil {
		t.Logf("%+v", res)
		t.Error("Select Unlogin error")
	}

	res, err = clientLogin.Select("INBOX", &imap.SelectOptions{}).Wait()
	if err != nil {
		t.Logf("%+v", res)
		t.Error("Select error")
	}

	if res == nil || res.NumMessages == 0 {
		t.Error("Select Error")
	}

	res, err = clientLogin.Select("Deleted Messages", &imap.SelectOptions{}).Wait()
	if err != nil {
		t.Logf("%+v", res)
		t.Error("Select error")
	}

	if res == nil || res.NumMessages == 0 {
		t.Error("Select Error")
	}
}

func TestStatus(t *testing.T) {
	res, err := clientUnLogin.Status("INBOX", &imap.StatusOptions{}).Wait()
	if err == nil {
		t.Logf("%+v", res)
		t.Error("Select Unlogin error")
	}

	res, err = clientLogin.Status("INBOX", &imap.StatusOptions{}).Wait()
	if err != nil {
		t.Logf("%+v", res)
		t.Error("Select error")
	}
}

func TestFetch(t *testing.T) {
	res2, err := clientLogin.Select("INBOX", &imap.SelectOptions{}).Wait()
	if err != nil {
		t.Logf("%+v", res2)
		t.Error("Fetch error")
	}

	res, err := clientLogin.Fetch(imap.SeqSetNum(1), &imap.FetchOptions{
		Envelope:     true,
		Flags:        true,
		InternalDate: true,
		RFC822Size:   true,
		UID:          true,
		BodySection: []*imap.FetchItemBodySection{
			{
				Specifier: imap.PartSpecifierText,
				Peek:      true,
			},
		},
	}).Collect()
	if err != nil {
		t.Logf("%+v", res)
		t.Error("Fetch error")
	}

	res, err = clientLogin.Fetch(imap.SeqSetNum(1, 2, 3, 4, 5, 6, 7, 8, 9), &imap.FetchOptions{
		Flags: true,
		UID:   true,
	}).Collect()
	if err != nil {
		t.Logf("%+v", res)
		t.Error("Fetch error")
	}

	res, err = clientLogin.Fetch(imap.SeqSetNum(1), &imap.FetchOptions{
		Envelope:     true,
		Flags:        true,
		InternalDate: true,
		RFC822Size:   true,
		UID:          true,
		BodySection: []*imap.FetchItemBodySection{
			{
				Specifier:    imap.PartSpecifierHeader,
				HeaderFields: []string{"subject"},
				Peek:         true,
			},
		},
	}).Collect()
	if err != nil {
		t.Logf("%+v", res)
		t.Error("Fetch error")
	}

	res, err = clientLogin.Fetch(imap.SeqSetNum(1), &imap.FetchOptions{
		UID: true,
		BodySection: []*imap.FetchItemBodySection{
			{
				Specifier: imap.PartSpecifierHeader,
				Peek:      true,
			},
		},
	}).Collect()
	if err != nil {
		t.Logf("%+v", res)
		t.Error("Fetch error")
	}
}

func TestStore(t *testing.T) {
	res, err := clientLogin.Store(
		imap.UIDSetNum(1),
		&imap.StoreFlags{
			Op:    imap.StoreFlagsAdd,
			Flags: []imap.Flag{"\\Seen"},
		},
		&imap.StoreOptions{}).Collect()
	if err != nil {
		t.Errorf("%+v", err)
	}
	t.Logf("%+v", res)
}

func TestClose(t *testing.T) {
}

func TestExpunge(t *testing.T) {
	clientLogin.Select("INBOX", &imap.SelectOptions{}).Wait()

	res, err := clientLogin.UIDExpunge(imap.UIDSetNum(1, 2)).Collect()
	if err != nil {
		t.Errorf("%+v", err)
	}
	t.Logf("%+v", res)

	// UID 1 belongs to Deleted Messages, not INBOX: UID EXPUNGE must not touch
	// it while INBOX is selected.
	var inInbox []models.UserEmail
	db.Instance.Table("user_email").Where("id=2").Find(&inInbox)
	if len(inInbox) != 0 {
		t.Error("UID EXPUNGE did not delete the INBOX message")
	}
	var stillDeleted []models.UserEmail
	db.Instance.Table("user_email").Where("id=1").Find(&stillDeleted)
	if len(stillDeleted) != 1 {
		t.Fatal("UID EXPUNGE deleted a message outside the selected mailbox")
	}

	clientLogin.Select("Deleted Messages", &imap.SelectOptions{}).Wait()
	if _, err := clientLogin.UIDExpunge(imap.UIDSetNum(1)).Collect(); err != nil {
		t.Errorf("%+v", err)
	}
	var ues []models.UserEmail
	db.Instance.Table("user_email").Where("id=1").Find(&ues)
	if len(ues) > 0 {
		t.Error("TestExpunge Error")
	}
}

func TestExamine(t *testing.T) {
}

func TestSubscribe(t *testing.T) {
}

func TestUnSubscribe(t *testing.T) {
}

func TestLSub(t *testing.T) {
}

func TestCheck(t *testing.T) {
}

func TestSearch(t *testing.T) {
	clientLogin.Select("INBOX", &imap.SelectOptions{}).Wait()

	res, err := clientLogin.Search(&imap.SearchCriteria{
		UID: []imap.UIDSet{
			[]imap.UIDRange{
				{Start: 1},
			},
			[]imap.UIDRange{
				{Start: 2},
			},
			[]imap.UIDRange{
				{Start: 2, Stop: 5},
			},
		},
	}, &imap.SearchOptions{}).Wait()
	if err != nil {
		t.Errorf("%+v", err)
	}
	t.Logf("%+v", res)
}

func TestMove(t *testing.T) {
	clientLogin.Select("INBOX", &imap.SelectOptions{}).Wait()

	res, err := clientLogin.Fetch(imap.SeqSetNum(1), &imap.FetchOptions{
		Envelope:     true,
		Flags:        true,
		InternalDate: true,
		RFC822Size:   true,
		UID:          true,
		BodySection: []*imap.FetchItemBodySection{
			{
				Specifier: imap.PartSpecifierText,
				Peek:      true,
			},
		},
	}).Collect()
	if err != nil {
		t.Logf("%+v", res)
		t.Error("Fetch error")
	}

	if len(res) > 0 {
		uid := res[0].UID

		_, err = clientLogin.Move(imap.UIDSetNum(uid), "Junk").Wait()
		if err != nil {
			t.Errorf("%+v", err)
		}

	}
}

func TestCopy(t *testing.T) {
	clientLogin.Select("INBOX", &imap.SelectOptions{}).Wait()

	res, err := clientLogin.Fetch(imap.SeqSetNum(1), &imap.FetchOptions{
		Envelope:     true,
		Flags:        true,
		InternalDate: true,
		RFC822Size:   true,
		UID:          true,
		BodySection: []*imap.FetchItemBodySection{
			{
				Specifier: imap.PartSpecifierText,
				Peek:      true,
			},
		},
	}).Collect()
	if err != nil {
		t.Logf("%+v", res)
		t.Error("Fetch error")
	}

	if len(res) > 0 {
		_, err = clientLogin.Copy(imap.UIDSetNum(res[0].UID), "Junk").Wait()
		if err != nil {
			t.Errorf("%+v", err)
		}
	} else {
		t.Error("No Fetch Result")
	}
}

func TestCopyMultipleRanges(t *testing.T) {
	if _, err := clientLogin.Select("INBOX", &imap.SelectOptions{}).Wait(); err != nil {
		t.Fatal(err)
	}
	if err := clientLogin.Create("MultiCopyDest", nil).Wait(); err != nil {
		t.Fatal(err)
	}
	var user models.User
	if _, err := db.Instance.Table("user").Where("account=?", "testCase").Get(&user); err != nil {
		t.Fatal(err)
	}
	var dest models.Group
	if _, err := db.Instance.Table("group").Where("user_id=? and name=?", user.ID, "MultiCopyDest").Get(&dest); err != nil {
		t.Fatal(err)
	}
	db.Instance.Exec("delete from user_email where user_id=? and group_id=?", user.ID, dest.ID)

	set := imap.SeqSet{{Start: 1, Stop: 2}, {Start: 3, Stop: 4}}
	if _, err := clientLogin.Copy(set, "MultiCopyDest").Wait(); err != nil {
		t.Fatalf("copy: %v", err)
	}
	count, err := db.Instance.Table("user_email").Where("user_id=? and group_id=?", user.ID, dest.ID).Count()
	if err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("multi-range COPY copied %d messages, want 4", count)
	}
}

func TestCustomFolderFetchAndStore(t *testing.T) {
	if _, err := clientLogin.Select("INBOX", &imap.SelectOptions{}).Wait(); err != nil {
		t.Fatal(err)
	}
	if err := clientLogin.Create("RegressionBox", nil).Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := clientLogin.Copy(imap.SeqSetNum(1), "RegressionBox").Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := clientLogin.Select("RegressionBox", &imap.SelectOptions{}).Wait(); err != nil {
		t.Fatal(err)
	}

	messages, err := clientLogin.Fetch(imap.SeqSetNum(1), &imap.FetchOptions{UID: true, Flags: true}).Collect()
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("fetch in a custom folder returned %d messages, want 1", len(messages))
	}
	uid := messages[0].UID

	store := func(op imap.StoreFlagsOp, flag imap.Flag) {
		t.Helper()
		if _, err := clientLogin.Store(imap.UIDSetNum(uid), &imap.StoreFlags{Op: op, Flags: []imap.Flag{flag}}, &imap.StoreOptions{}).Collect(); err != nil {
			t.Fatalf("store: %v", err)
		}
	}

	store(imap.StoreFlagsAdd, imap.FlagSeen)
	var ue models.UserEmail
	if _, err := db.Instance.Table("user_email").Where("id=?", int(uid)).Get(&ue); err != nil {
		t.Fatal(err)
	}
	if ue.IsRead != 1 {
		t.Fatal("+FLAGS \\Seen did not mark the message read")
	}
	store(imap.StoreFlagsDel, imap.FlagSeen)
	var check models.UserEmail
	if _, err := db.Instance.Table("user_email").Where("id=?", int(uid)).Get(&check); err != nil {
		t.Fatal(err)
	}
	ue = check
	if ue.IsRead != 0 {
		t.Fatal("-FLAGS \\Seen did not mark the message unread")
	}

	store(imap.StoreFlagsAdd, imap.FlagDeleted)
	exists, err := db.Instance.Table("user_email").Where("id=?", int(uid)).Exist()
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("+FLAGS \\Deleted deleted the message before EXPUNGE")
	}
	store(imap.StoreFlagsDel, imap.FlagDeleted)
	if _, err := clientLogin.Expunge().Collect(); err != nil {
		t.Fatal(err)
	}
	exists, err = db.Instance.Table("user_email").Where("id=?", int(uid)).Exist()
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("-FLAGS \\Deleted did not cancel the pending expunge")
	}
	store(imap.StoreFlagsAdd, imap.FlagDeleted)
	if _, err := clientLogin.Expunge().Collect(); err != nil {
		t.Fatal(err)
	}
	exists, err = db.Instance.Table("user_email").Where("id=?", int(uid)).Exist()
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("EXPUNGE did not delete the message")
	}
}

func TestAppend(t *testing.T) {
	raw := "From: testCase@domain.com\r\nTo: testCase@domain.com\r\nSubject: appended\r\n\r\nbody\r\n"
	command := clientLogin.Append("Drafts", int64(len(raw)), &imap.AppendOptions{Flags: []imap.Flag{imap.FlagSeen}})
	if _, err := command.Write([]byte(raw)); err != nil {
		t.Fatal(err)
	}
	if err := command.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := command.Wait()
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if data == nil || data.UID == 0 {
		t.Fatalf("append returned no UID: %+v", data)
	}
	var ue models.UserEmail
	if _, err := db.Instance.Table("user_email").Where("id=?", int(data.UID)).Get(&ue); err != nil {
		t.Fatal(err)
	}
	if ue.ID == 0 || ue.Status != 4 || ue.IsRead != 1 {
		t.Fatalf("appended draft not stored correctly: %+v", ue)
	}
}

func TestNoop(t *testing.T) {
	err := clientLogin.Noop().Wait()
	if err != nil {
		t.Error(err)
	}
}

func TestIDLE(t *testing.T) {
	conn, err := tls.Dial("tcp", imapTestAddr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, "OK") {
		t.Fatalf("unexpected greeting: %s", line)
	}

	if _, err = conn.Write([]byte("a001 LOGIN testCase testCase\r\n")); err != nil {
		t.Fatal(err)
	}
	if line, err = readTaggedLine(reader, "a001"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, "OK") {
		t.Fatalf("unexpected LOGIN response: %s", line)
	}

	if _, err = conn.Write([]byte("a002 SELECT INBOX\r\n")); err != nil {
		t.Fatal(err)
	}
	if line, err = readTaggedLine(reader, "a002"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, "OK") {
		t.Fatalf("unexpected SELECT response: %s", line)
	}

	if _, err = conn.Write([]byte("a003 IDLE\r\n")); err != nil {
		t.Fatal(err)
	}
	line, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(line, "+") || !strings.Contains(strings.ToLower(line), "idling") {
		t.Fatalf("unexpected IDLE continuation: %s", line)
	}

	if _, err = conn.Write([]byte("DONE\r\n")); err != nil {
		t.Fatal(err)
	}
	if line, err = readTaggedLine(reader, "a003"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, "OK") {
		t.Fatalf("unexpected DONE response: %s", line)
	}
}

func TestIdleNotice(t *testing.T) {
	updates := make(chan uint32, 1)
	options := &imapclient.Options{
		WordDecoder: &mime.WordDecoder{CharsetReader: charset.Reader},
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Mailbox: func(data *imapclient.UnilateralDataMailbox) {
				if data.NumMessages != nil {
					updates <- *data.NumMessages
				}
			},
		},
	}

	client, err := imapclient.DialTLS(imapTestAddr, options)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err = client.Login("testCase", "testCase").Wait(); err != nil {
		t.Fatal(err)
	}

	selected, err := client.Select("INBOX", &imap.SelectOptions{}).Wait()
	if err != nil {
		t.Fatal(err)
	}

	idleCmd, err := client.Idle()
	if err != nil {
		t.Fatal(err)
	}

	var user models.User
	_, err = db.Instance.Table(&models.User{}).Where("account=?", "testCase").Get(&user)
	if err != nil {
		t.Fatal(err)
	}
	if user.ID == 0 {
		t.Fatal("test user not found")
	}

	ctx := &pcontext.Context{UserID: user.ID}
	if err = IdleNotice(ctx, user.ID, &models.Email{Id: 1}); err != nil {
		t.Fatal(err)
	}

	select {
	case numMessages := <-updates:
		if numMessages != selected.NumMessages {
			t.Fatalf("unexpected IDLE EXISTS update: got %d, want %d", numMessages, selected.NumMessages)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for IDLE update")
	}

	if err = idleCmd.Close(); err != nil {
		t.Fatal(err)
	}
	if err = idleCmd.Wait(); err != nil {
		t.Fatal(err)
	}
	waitIdleConnectionsDeleted(t, user.ID)
}

func readTaggedLine(reader *bufio.Reader, tag string) (string, error) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return line, err
		}
		if strings.HasPrefix(line, tag+" ") {
			return line, nil
		}
	}
}

func waitIdleConnectionsDeleted(t *testing.T, userId int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := userConnects.Load(userId); !ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("idle connections were not cleaned up")
}

func TestUnselect(t *testing.T) {
}

func TestLogout(t *testing.T) {
	err := clientLogin.Logout().Wait()
	if err != nil {
		t.Error(err)
	}
}
