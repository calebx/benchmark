package vrpc

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"net"
	"sync"
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

func NewClient(ctx context.Context, cid uint32, port uint32, addr string, isVsock bool, size int) (Client, error) {
	pool := &clientPool{
		CID:      cid,
		Port:     port,
		Addr:     addr,
		IsVsock:  isVsock,
		closed:   make(chan bool),
		streamCh: make(chan *client, size),
	}
	for range size {
		cli, err := newClient(ctx, cid, port, addr, isVsock)
		if err != nil {
			return nil, err
		}
		pool.streamCh <- cli
		pool.streamSl = append(pool.streamSl, cli)
	}
	return pool, nil
}

type clientPool struct {
	CID      uint32
	Port     uint32
	Addr     string
	IsVsock  bool
	closed   chan bool
	streamCh chan *client
	streamSl []*client
}

func (cp *clientPool) Invoke(ctx context.Context, cmd string, payload []byte) (resp *Response, err error) {
	cli := cp.get()
	defer func() {
		cp.put(cli)
	}()

	batch, i, done, err := cli.Invoke(ctx, cmd, payload)
	if batch == nil || done == nil {
		return nil, errors.New("invoke err")
	}
	_ = <-done

	responses := batch.GetResponse()
	if len(responses) <= i {
		return nil, errors.New("response too short")
	}

	respPb := responses[i]
	resp = &Response{
		Code:    respPb.GetCode(),
		Message: respPb.GetMessage(),
		Payload: respPb.GetPayload(),
	}
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
				cli, err = newClient(newCtx, cp.CID, cp.Port, cp.Addr, cp.IsVsock)
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
	i := rand.Intn(len(cp.streamSl))
	return cp.streamSl[i]
	// return <-cp.streamCh
}

func (cp *clientPool) put(client *client) {
	// cp.streamCh <- client
}

func newClient(ctx context.Context, cid uint32, port uint32, addr string, isVsock bool) (*client, error) {
	dialer := netDialer(addr)
	if isVsock {
		dialer = vsockDialer(cid, port)
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
	c := &client{
		stream: stream,
	}
	c.reset()
	return c, nil
}

type client struct {
	sync.Mutex
	stream pb.Vrpc_DoClient

	todos [][]byte
	after <-chan time.Time
	ready chan bool
	once  *sync.Once
	respx *RespX
}

type RespX struct {
	pb     *pb.BatchResp
	doneCh chan bool
}

func (c *client) reset() {
	c.todos = nil
	c.after = time.After(5 * time.Millisecond)
	c.ready = make(chan bool, 1)

	after := c.after
	ready := c.ready
	go func() {
		<-after
		ready <- true
	}()

	c.once = new(sync.Once)
	c.respx = &RespX{
		pb:     &pb.BatchResp{},
		doneCh: make(chan bool),
	}
}

func (c *client) Invoke(ctx context.Context, cmd string, payload []byte) (batch *pb.BatchResp, i int, isDone chan bool, err error) {
	c.Lock()
	defer c.Unlock()

	select {
	case <-ctx.Done():
		return nil, -1, nil, ctx.Err()
	default:
		ready := c.ready

		c.todos = append(c.todos, payload)
		if len(c.todos) > 10 {
			ready <- true
		}

		i := len(c.todos) - 1
		respPb := c.respx.pb
		doneCh := c.respx.doneCh
		once := c.once
		go func() {
			once.Do(func() {
				defer func() {
					close(doneCh)
					c.reset()
				}()

				<-ready
				batchReq := &pb.BatchReq{
					Requests: []*pb.Request{},
				}
				for _, todo := range c.todos {
					batchReq.Requests = append(batchReq.Requests, &pb.Request{
						Command: cmd,
						Payload: todo,
					})
				}
				if err := c.stream.Send(batchReq); err != nil {
					log.Printf("send error: %v", err)
					return
				}
				batchPb, err := c.stream.Recv()
				if err != nil {
					log.Printf("recv error: %v", err)
					return
				}
				respPb.Response = batchPb.GetResponse()
			})
		}()

		return respPb, i, doneCh, nil
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
