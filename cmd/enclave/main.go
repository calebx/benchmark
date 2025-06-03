package main

import (
	"context"

	"nothing.com/benchmark/vrpc"
)

const port = 50001

type Req struct {
	XID string `json:"xid"`
}

type Resp struct {
	DIX string `json:"dix"`
}

func main() {
	dp := vrpc.NewDispatcher(uint32(10001))

	dp.Register("/echo", func(ctx context.Context, req *Req) (*Resp, error) {
		xid := req.XID
		if len(xid) > 64 {
			xid = xid[:64]
		}

		return &Resp{
			DIX: reverse(xid),
		}, nil
	})

	server := vrpc.NewServer(uint32(port), dp, true)
	server.Start()
}

func reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
