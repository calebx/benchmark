package vrpc

import (
	"context"
	"errors"
	"log"
	"net"
	"time"

	"github.com/mdlayher/vsock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	pb "nothing.com/benchmark/vrpc/gen"
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

func NewClient(ctx context.Context, cid uint32, port uint32, addr string, isDev bool, size int) (Client, error) {
	pool := &clientPool{
		CID:      cid,
		Port:     port,
		Addr:     addr,
		IsDev:    isDev,
		closed:   make(chan bool),
		streamCh: make(chan *client, size),
	}
	for range size {
		cli, err := newClient(ctx, cid, port, addr, isDev)
		if err != nil {
			return nil, err
		}
		pool.streamCh <- cli
	}
	return pool, nil
}

type clientPool struct {
	CID      uint32
	Port     uint32
	Addr     string
	IsDev    bool
	closed   chan bool
	streamCh chan *client
}

func (cp *clientPool) Invoke(ctx context.Context, cmd string, payload []byte) (resp *Response, err error) {
	cli := cp.get()
	defer func() {
		cp.put(cli)
	}()

	resp, err = cli.Invoke(ctx, cmd, payload)
	if err != nil {
		defer func() {
			select {
			case <-cp.closed:
				log.Printf("client pool is closed")
			default:
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
					log.Printf("context error: %v", err)
					return
				}

				_ = cli.stream.CloseSend()
				newCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				cli, err = newClient(newCtx, cp.CID, cp.Port, cp.Addr, cp.IsDev)
				if err != nil {
					log.Printf("recreate client failed: %v", err)
				}
			}

			// here if it failed to recreate the client,
			// it will be put back to the pool again,
			// and trigger a new client creation on the next request by a new try.
		}()
		log.Printf("invoke cmd failed: %s - %v", cmd, err)
	}
	return resp, err
}

func (cp *clientPool) Close() {
	close(cp.closed)
	close(cp.streamCh)
	for cli := range cp.streamCh {
		if cli != nil {
			if err := cli.stream.CloseSend(); err != nil {
				log.Printf("close stream failed: %v", err)
			}
			continue
		}
		// if cli is nil, it means the channel is closed
		return
	}
}

func (cp *clientPool) get() *client {
	return <-cp.streamCh
}

func (cp *clientPool) put(client *client) {
	cp.streamCh <- client
}

func newClient(ctx context.Context, cid uint32, port uint32, addr string, isDev bool) (*client, error) {
	dialer := vsockDialer(cid, port)
	if isDev {
		dialer = netDialer(addr)
	}
	conn, err := grpc.NewClient(
		addr,
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                2 * time.Minute, // NOTE: well tuned for current use case
			Timeout:             5 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return nil, err
	}

	pbClient := pb.NewVrpcClient(conn)
	stream, err := pbClient.Do(ctx)
	if err != nil {
		return nil, err
	}
	return &client{
		stream: stream,
	}, nil
}

type client struct {
	stream pb.Vrpc_DoClient
}

func (c *client) Invoke(ctx context.Context, cmd string, payload []byte) (resp *Response, err error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		if err := c.stream.Send(&pb.Request{
			Command: cmd,
			Payload: payload,
		}); err != nil {
			return nil, err
		}

		respPb, err := c.stream.Recv()
		if err != nil {
			return nil, err
		}
		return &Response{
			Code:    respPb.GetCode(),
			Message: respPb.GetMessage(),
			Payload: respPb.GetPayload(),
		}, nil
	}
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
