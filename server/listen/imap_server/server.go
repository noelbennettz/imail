package imap_server

import (
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/id"
	log "github.com/sirupsen/logrus"

	"github.com/emersion/go-imap/v2/imapserver"
)

// Server is a server instance.
//
// A server contains a list of users.
type Server struct{}

// NewServer creates a new server.
func NewServer() *Server {
	return &Server{}
}

type Status int8

const (
	UNAUTHORIZED Status = 1
	AUTHORIZED   Status = 2
	SELECTED     Status = 3
	LOGOUT       Status = 4
)

type serverSession struct {
	server         *Server // immutable
	ctx            *context.Context
	status         Status
	currentMailbox string
	connectTime    time.Time
	deleteUidList  []int
}

// NewSession creates a new IMAP session.
func (s *Server) NewSession() imapserver.Session {
	tc := &context.Context{}
	tc.SetValue(context.LogID, id.GenLogID())

	return &serverSession{
		server:      s,
		ctx:         tc,
		connectTime: time.Now(),
	}
}

func (s *serverSession) Close() error {
	return nil
}

func (s *serverSession) Subscribe(mailbox string) error {
	return nil
}

func (s *serverSession) Unsubscribe(mailbox string) error {
	return nil
}

func (s *serverSession) Append(mailbox string, r imap.LiteralReader, options *imap.AppendOptions) (*imap.AppendData, error) {
	log.WithContext(s.ctx).Errorf("Append Not Implemented")
	return nil, nil
}

func (s *serverSession) Unselect() error {
	s.currentMailbox = ""
	return nil
}
