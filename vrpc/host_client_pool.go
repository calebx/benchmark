package vrpc

import (
	"context"
	"errors"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
)

type clientPool struct {
	CID     uint32
	Port    uint32
	Addr    string
	IsVsock bool

	closed  chan bool
	clients []*client
	currIdx int
}

func (cp *clientPool) Invoke(ctx context.Context, cmd string, payload []byte) (resp *Response, err error) {
	cliIdx, cli := cp.get()

	batchResp, idx, done, err := cli.invoke(ctx, cmd, payload)
	if batchResp == nil || done == nil {
		return nil, fmt.Errorf("invoke failed: %v", err)
	}

	defer func(err error) {
		if err == nil {
			return
		}

		log.WithError(err).WithField("cmd", cmd).Error("invoke cmd failed")
		select {
		case <-cp.closed:
			log.Printf("client pool is closed")
		default:
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return
			}

			log.Printf("recreating client due to error: %v", err)
			_ = cli.stream.CloseSend()
			newCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			newCli, err := newCli(newCtx, cp.CID, cp.Port, cp.Addr, cp.IsVsock)
			if err != nil {
				log.Printf("recreate client failed: %v", err)
			} else {
				cp.clients[cliIdx] = newCli
			}
		}
	}(err)

	_ = <-done
	respSl := batchResp.GetResponses()
	if len(respSl) <= idx {
		return nil, fmt.Errorf("resp not found in batch result, size=%d, idx=%d", len(respSl), idx)
	}
	respPb := respSl[idx]
	resp = &Response{
		Code:    respPb.GetCode(),
		Message: respPb.GetMessage(),
		Payload: respPb.GetPayload(),
	}
	return resp, err
}

func (cp *clientPool) Close() {
	close(cp.closed)
	for _, cli := range cp.clients {
		if cli != nil {
			if err := cli.stream.CloseSend(); err != nil {
				log.Printf("close stream failed: %v", err)
			}
		}
	}
}

func (cp *clientPool) get() (int, *client) {
	idx := cp.currIdx
	cli := cp.clients[idx]
	cp.currIdx = (cp.currIdx + 1) % len(cp.clients)
	return idx, cli
}
