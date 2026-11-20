package gopop

import (
	"context"
	"net"
	"time"
)

type Status int8

const (
	UNAUTHORIZED Status = 1
	TRANSACTION  Status = 2
	UPDATE       Status = 3
)

type Session struct {
	Status    Status
	User      string
	DeleteIds []int64
	Ctx       context.Context
	Conn      net.Conn
	InTls     bool
	AliveTime time.Time
	// Messages maps the POP3 message number shown to the client to the
	// internal email id. Sizes holds the same snapshot's message sizes.
	Messages map[int64]int64
	Sizes    map[int64]int64
}
