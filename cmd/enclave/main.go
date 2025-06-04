package main

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"

	"nothing.com/benchmark/vrpc"
)

const (
	port      = 50001
	errorCode = 10005
	timeoutMs = 1000
	isVsock   = false
)

type Req struct {
	XID string `json:"xid"`
}

type Resp struct {
	DIX string `json:"dix"`
}

func main() {
	dp := vrpc.NewDispatcher(uint32(errorCode), timeoutMs*time.Millisecond)
	dp.Register("/echo", func(ctx context.Context, req *Req) (*Resp, error) {
		xid := req.XID
		if len(xid) > 64 {
			xid = xid[:64]
		}

		return &Resp{
			DIX: reverse(xid),
		}, nil
	})

	log.Println("start starts", "port:", port, "isVsock:", isVsock)
	server := vrpc.NewServer(uint32(port), dp, isVsock)
	server.Start()
}

func reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
