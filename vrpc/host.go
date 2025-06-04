package vrpc

import (
	"context"
	"net"

	"github.com/mdlayher/vsock"
)

type Response struct {
	Code    uint32 `json:"code"`
	Message string `json:"message"`
	Payload []byte `json:"payload"`
}

type Client interface {
	Invoke(ctx context.Context, cmd string, payload []byte) (resp *Response, err error)
	Close()
}

func NewClient(ctx context.Context, cid uint32, port uint32, addr string, isVsock bool, size int) (Client, error) {
	pool := &clientPool{
		CID:     cid,
		Port:    port,
		Addr:    addr,
		IsVsock: isVsock,
		closed:  make(chan bool),
		clients: nil,
		currIdx: 0,
	}
	for range size {
		cli, err := newCli(ctx, cid, port, addr, isVsock)
		if err != nil {
			return nil, err
		}
		pool.clients = append(pool.clients, cli)
	}
	return pool, nil
}

func vsockDialer(cid uint32, port uint32) func(context.Context, string) (net.Conn, error) {
	return func(ctx context.Context, addr string) (net.Conn, error) {
		return vsock.Dial(cid, port, nil)
	}
}

func netDialer(addr string) func(context.Context, string) (net.Conn, error) {
	return func(ctx context.Context, _ string) (net.Conn, error) {
		return net.Dial("tcp", addr)
	}
}
