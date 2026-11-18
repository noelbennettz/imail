package gopop

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Server is a POP3 server instance.
type Server struct {
	Domain           string        // greeting domain
	Port             int           // listen port
	Addr             string        // optional host:port override
	Network          string        // tcp, tcp4 or tcp6
	TlsEnabled       bool          // whether the listener starts in TLS mode
	TlsConfig        *tls.Config   // TLS configuration
	ConnectAliveTime time.Duration // connection lifetime, zero means no limit
	Action           Action
	stop             chan bool
	close            bool
	lck              sync.Mutex
}

// listenAddress returns the configured bind address or the port wildcard.
func (s *Server) listenAddress() string {
	if s.Addr != "" {
		return s.Addr
	}
	return fmt.Sprintf(":%d", s.Port)
}

// listenNetwork returns the socket family used for binding.
func (s *Server) listenNetwork() string {
	if s.Network != "" {
		return s.Network
	}
	return "tcp"
}

// NewPop3Server creates a new server instance
func NewPop3Server(port int, domain string, tlsEnabled bool, tlsConfig *tls.Config, action Action) *Server {
	return &Server{
		Domain:     domain,
		Port:       port,
		TlsEnabled: tlsEnabled,
		TlsConfig:  tlsConfig,
		Action:     action,
		stop:       make(chan bool, 1),
	}
}

// Start starts the server
func (s *Server) Start() error {
	if !s.TlsEnabled {
		return s.startWithoutTLS()
	} else {
		return s.startWithTLS()
	}
}

func (s *Server) startWithTLS() error {
	if s.lck.TryLock() {
		listener, err := tls.Listen(s.listenNetwork(), s.listenAddress(), s.TlsConfig)
		if err != nil {
			return err
		}
		s.close = false
		defer func() {
			listener.Close()
		}()

		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					if s.close {
						break
					} else {
						continue
					}
				}
				go s.handleClient(conn)
			}
		}()
		<-s.stop
	} else {
		return errors.New("Server Is Running")
	}

	return nil
}

func (s *Server) startWithoutTLS() error {
	if s.lck.TryLock() {
		listener, err := net.Listen(s.listenNetwork(), s.listenAddress())
		if err != nil {
			return err
		}
		s.close = false
		defer func() {
			listener.Close()
		}()

		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					if s.close {
						break
					} else {
						continue
					}
				}
				go s.handleClient(conn)
			}
		}()
		<-s.stop
	} else {
		return errors.New("Server Is Running")
	}

	return nil
}

// Stop stops the server
func (s *Server) Stop() {
	s.close = true
	s.stop <- true
}

func (s *Server) handleClient(conn net.Conn) {
	slog.Debug("pop3 conn")

	defer conn.Close()

	session := &Session{
		Conn:      conn,
		AliveTime: time.Now(),
	}
	if s.TlsEnabled && s.TlsConfig != nil {
		session.InTls = true
	}

	// Check whether the connection has timed out
	if s.ConnectAliveTime != 0 {
		go func() {
			for {
				if time.Now().Sub(session.AliveTime) >= s.ConnectAliveTime {
					session.Conn.Close()
					return
				}
				time.Sleep(3 * time.Second)
			}
		}()
	}

	eol := "\r\n"
	reader := bufio.NewReader(conn)

	fmt.Fprintf(conn, "+OK %s POP3 Server powered by gopop"+eol, s.Domain)

	for {
		rawLine, err := reader.ReadString('\n')
		if err != nil {
			conn.Close()
			return
		}
		session.AliveTime = time.Now()

		cmd, args := getCommand(rawLine)
		slog.Debug(fmt.Sprintf("cmd:%s args:%v", cmd, args))
		var cmdError error

		/*
			user<SP>username<CRLF>	      the user command is usually the first command a POP3 client sends after connecting to the POP3 server; the username argument is the recipient's account name.
			pass<SP>password<CRLF>	      the pass command is sent by the POP3 client after the user command succeeds; it carries the account password, where password is the account password.
			apop<SP>name,digest<CRLF>	      the apop command replaces user and pass and submits the account password to the POP3 server as an MD5 digest.
			stat<CRLF>	     the stat command queries mailbox statistics, such as the number of messages in the mailbox and the number of bytes they occupy.
			uidl<SP>msg#<CRLF>	     the uidl command queries the unique identifier of a message; msg# is the message sequence number, a number starting at 1.
			list<SP>[MSG#]<CRLF>	     the list command lists message information in the mailbox; msg# is an optional message sequence number. Without it, the POP3 server lists all messages; with it, the POP3 server returns only the message with that sequence number.
			retr<SP>msg#<CRLF>	    the retr command fetches the content of a message; msg# is the message sequence number.
			dele<SP>msg#<CRLF>	    the dele command marks a message for deletion; msg# is the message sequence number. The POP3 server only sets the deletion mark, it does not really delete the message; only after the POP3 client sends quit does the POP3 server really delete all messages marked for deletion.
			rest<CRLF>	    the rest command clears the deletion marks of all messages.
			top<SP>msg#<SP>n<CRLF>	    the top command fetches the message headers plus the first n lines of the body; msg# is the message sequence number and n is the number of body lines to return. This command improves the efficiency of listing messages in a Web Mail system (sending and receiving mail through a web site), because the whole message is not needed there, only the headers.
			noop<CRLF>	    the noop command checks the connection between the POP3 client and the POP3 server.
			quit<CRLF>	    the quit command ends the mail retrieval session; after receiving it the POP3 server deletes all messages marked for deletion and closes the network connection to the POP3 client.
			capa<CRLF>  the capa command returns the list of commands the server supports
		*/

		switch cmd {
		case "CAPA":
			commands, err := s.Action.Capa(session)
			if err != nil {
				fmt.Fprintf(conn, "-ERR %s%s", err.Error(), eol)
			} else {
				ret := fmt.Sprintf("+OK Capability list follows%s", eol)

				for _, command := range commands {
					ret += fmt.Sprintf("%s%s", command, eol)
				}
				ret += fmt.Sprintf(".%s", eol)
				fmt.Fprint(conn, ret)
			}
		case "USER":
			userName := getSafeArg(args, 0)
			cmdError = s.Action.User(session, userName)
			if cmdError != nil {
				fmt.Fprintf(conn, "-ERR The user %s doesn't belong here!"+eol, userName)
			} else {
				fmt.Fprint(conn, "+OK"+eol)
			}
		case "PASS":
			password := getSafeArg(args, 0)
			cmdError = s.Action.Pass(session, password)
			if cmdError != nil {
				fmt.Fprint(conn, "-ERR Password incorrect!"+eol)
			} else {
				fmt.Fprint(conn, "+OK User signed in"+eol)
			}
		case "STAT":
			if session.Status == TRANSACTION {
				num, size, err := s.Action.Stat(session)
				if err != nil {
					fmt.Fprintf(conn, "-ERR %s%s", err.Error(), eol)
				} else {
					fmt.Fprintf(conn, "+OK %d %d %s", num, size, eol)
				}
			}
		case "LIST":
			if session.Status == TRANSACTION {
				infos, err := s.Action.List(session, getSafeArg(args, 0))
				if err != nil {
					fmt.Fprintf(conn, "-ERR %s %s", err.Error(), eol)
				} else {
					ret := "+OK" + eol
					for _, info := range infos {
						ret += fmt.Sprintf("%d %d%s", info.Id, info.Size, eol)
					}
					ret += "." + eol
					fmt.Fprint(conn, ret)
				}
			}
		case "UIDL":
			if session.Status == TRANSACTION {
				infos, err := s.Action.Uidl(session, getSafeArg(args, 0))
				if err != nil {
					fmt.Fprintf(conn, "-ERR %s %s", err.Error(), eol)
				} else {
					ret := fmt.Sprintf("+OK%s", eol)
					for _, info := range infos {
						ret += fmt.Sprintf("%d %s%s", info.Id, info.UnionId, eol)
					}
					ret += "." + eol
					fmt.Fprint(conn, ret)
				}
			}
		case "TOP":
			if session.Status == TRANSACTION {
				id, err1 := strconv.ParseInt(getSafeArg(args, 0), 10, 64)
				line, err2 := strconv.Atoi(getSafeArg(args, 1))
				if err1 != nil || err2 != nil {
					fmt.Fprintf(conn, "-ERR %s %s", "params error", eol)
				} else {
					res, err := s.Action.Top(session, id, line)
					if err != nil {
						fmt.Fprintf(conn, "-ERR %s %s", err.Error(), eol)
					} else {
						ret := fmt.Sprintf("+OK%s", eol)
						ret += fmt.Sprintf("%s%s", res, eol)
						ret += "." + eol
						fmt.Fprint(conn, ret)
					}
				}
			}
		case "RETR":
			if session.Status == TRANSACTION {
				id, err := strconv.ParseInt(getSafeArg(args, 0), 10, 64)
				if err != nil {
					fmt.Fprintf(conn, "-ERR %s %s", "params error", eol)
				} else {
					res, size, err := s.Action.Retr(session, id)
					if err != nil {
						fmt.Fprintf(conn, "-ERR %s %s", err.Error(), eol)
					} else {
						ret := fmt.Sprintf("+OK %d%s", size, eol)
						ret += fmt.Sprintf("%s%s", res, eol)
						ret += "." + eol
						fmt.Fprint(conn, ret)
					}
				}
			}
		case "DELE":
			if session.Status == TRANSACTION {
				id, err := strconv.ParseInt(getSafeArg(args, 0), 10, 64)
				if err != nil {
					fmt.Fprintf(conn, "-ERR %s %s", "params error", eol)
				} else {
					err := s.Action.Delete(session, id)
					if err != nil {
						fmt.Fprintf(conn, "-ERR %s %s", err.Error(), eol)
					} else {
						fmt.Fprintf(conn, "+OK%s", eol)
					}
				}
			}
		case "REST":
			if session.Status == TRANSACTION {
				err := s.Action.Rest(session)
				if err != nil {
					fmt.Fprintf(conn, "-ERR %s %s", err.Error(), eol)
				} else {
					fmt.Fprintf(conn, "+OK%s", eol)
				}

			}
		case "QUIT":
			if session.Status == TRANSACTION {
				s.Action.Quit(session)
				fmt.Fprintf(conn, "+OK Bye %s", eol)
				conn.Close()
			}
		case "NOOP":
			fmt.Fprintf(conn, "+OK%s", eol)
		case "STLS":
			if s.TlsConfig == nil {
				fmt.Fprintf(conn, "-ERR %s %s", "unsupport", eol)
			} else if session.InTls {
				fmt.Fprintf(conn, "-ERR already in TLS%s", eol)
			} else {
				// RFC 2595: acknowledge STLS before starting the handshake.
				// Waiting until after Handshake() deadlocks the client.
				fmt.Fprintf(conn, "+OK Begin TLS negotiation%s", eol)
				tc := tls.Server(conn, s.TlsConfig)
				if err := tc.Handshake(); err != nil {
					slog.Error("STLS Error", "err", err)
					return
				}
				session.Status = UNAUTHORIZED
				session.InTls = true
				session.Conn = tc
				conn = tc
				reader = bufio.NewReader(tc)
			}
		default:
			rets, err := s.Action.Custom(session, cmd, args)
			if err != nil {
				fmt.Fprintf(conn, "-ERR %s %s", err.Error(), eol)
			} else {
				if len(rets) == 0 {
					fmt.Fprintf(conn, "+OK %s", eol)
				} else if len(rets) == 1 {
					fmt.Fprintf(conn, "+OK %s%s", rets[0], eol)
				} else {
					ret := fmt.Sprintf("+OK %s", eol)
					for _, re := range rets {
						ret += fmt.Sprintf("%s%s", re, eol)
					}
					ret += "." + eol
					fmt.Fprint(conn, ret)
				}
			}
		}

	}
}

// cuts the line into command and arguments
func getCommand(line string) (string, []string) {
	line = strings.Trim(line, "\r \n")
	cmd := strings.Split(line, " ")

	return strings.ToTitle(cmd[0]), cmd[1:]
}

func getSafeArg(args []string, nr int) string {
	if nr < len(args) {
		return args[nr]
	}
	return ""
}
