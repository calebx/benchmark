package vrpc

import (
	"context"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	pb "nothing.com/benchmark/vrpc/gen"
)

type client struct {
	sync.Mutex
	stream pb.Vrpc_DoClient
	batch  *batch
}

type batch struct {
	once  *sync.Once       // one batch will fire batch request once
	reqs  [][]byte         // aggregated requests
	resp  *pb.BatchResp    // batch response pb
	done  chan struct{}    // done channel for batch processing
	ready chan struct{}    // ready channel to signal that the batch is ready to be sent
	after <-chan time.Time // after channel to trigger batch processing
}

func newCli(ctx context.Context, cid uint32, port uint32, addr string, isVsock bool) (*client, error) {
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
		return nil, fmt.Errorf("do vrpc client stream: %w", err)
	}
	c := &client{stream: stream}
	c.reset()
	return c, nil
}

func (c *client) reset() {
	one := &batch{
		once:  &sync.Once{},
		reqs:  [][]byte{},
		resp:  &pb.BatchResp{},
		done:  make(chan struct{}),
		ready: make(chan struct{}, 1),
		after: time.After(5 * time.Millisecond), // <- TODO: move to config
	}

	go func() {
		_ = <-one.after
		one.ready <- struct{}{}
	}()

	c.batch = one
}

func (c *client) invoke(ctx context.Context, cmd string, payload []byte) (batchResp *pb.BatchResp, idx int, doneCh chan struct{}, err error) {
	c.Lock()
	defer c.Unlock()

	one := c.batch

	select {
	case <-ctx.Done():
		return nil, -1, nil, ctx.Err()
	default:
		one.reqs = append(one.reqs, payload)
		idx = len(one.reqs) - 1

		if idx > 20 { // TODO: <- move to config
			select {
			case one.ready <- struct{}{}:
				// ready to fire
				// will be blocked if others triggered already but that is fine to continue
			default:
				// ok to continue
			}
		}

		go func() {
			one.once.Do(func() {
				<-one.ready

				c.Lock()
				defer c.Unlock()

				defer func() {
					close(one.done)
					c.reset()
				}()

				batchReq := &pb.BatchReq{}
				for _, req := range one.reqs {
					batchReq.Requests = append(batchReq.Requests, &pb.Request{
						Command: cmd,
						Payload: req,
					})
				}
				if err := c.stream.Send(batchReq); err != nil {
					log.WithError(err).Error("stream send req failed")
					return
				}
				respPb, err := c.stream.Recv()
				if err != nil {
					log.WithError(err).Error("stream recv resp failed")
					return
				}

				one.resp.Responses = respPb.GetResponses()
			})
		}()

		return one.resp, idx, one.done, nil
	}
}
