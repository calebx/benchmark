package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/labstack/echo/v4"
	"github.com/mdlayher/vsock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/keepalive"
	pb "nothing.com/benchmark/proto/echo"
)

const enclaveAddr = "127.0.0.1:5005"

type JsonIterSerializer struct{}

func (j *JsonIterSerializer) Serialize(c echo.Context, i interface{}, indent string) error {
	enc := jsoniter.NewEncoder(c.Response())
	if indent != "" {
		enc.SetIndent("", indent)
	}
	return enc.Encode(i)
}

func (j *JsonIterSerializer) Deserialize(c echo.Context, i interface{}) error {
	return jsoniter.NewDecoder(c.Request().Body).Decode(i)
}

type Req struct {
	XID string `json:"xid"`
}

type Resp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	pool, err := NewClientPool(enclaveAddr, 16)
	if err != nil {
		log.Fatalf("failed to create client pool: %v", err)
	}

	e := echo.New()
	e.JSONSerializer = &JsonIterSerializer{}

	e.POST("/", func(c echo.Context) error {
		req := &Req{}
		if err := c.Bind(req); err != nil {
			return c.JSON(http.StatusBadRequest, &Resp{
				Code:    400,
				Message: "Bad Request",
			})
		}

		client := pool.Pick()
		defer func() {
			pool.Put(client)
		}()

		respStr, err := client.Proxy(req.XID)
		if err != nil {
			defer func() {
				_ = client.stream.CloseSend()
				client, err = NewClient(enclaveAddr)
				if err != nil {
					log.Printf("failed to recreate client: %v", err)
				}
				// here if it failed to recreate the client,
				// it will be put back to the pool again,
				// and trigger a new client creation on the next request by a new try.
			}()

			log.Printf("stream error: %s", err)
			return c.JSON(http.StatusInternalServerError, &Resp{Code: 500, Message: err.Error()})
		}

		return c.JSON(http.StatusOK, &Resp{
			Code:    200,
			Message: respStr,
		})
	})

	e.Logger.Fatal(e.Start(":1323"))
}

type Client struct {
	stream pb.EchoService_EchoStreamClient
}

func (c *Client) Proxy(xid string) (string, error) {
	if err := c.stream.Send(&pb.EchoRequest{
		Payload: []byte(xid),
	}); err != nil {
		return "", err
	}

	resp, err := c.stream.Recv()
	if err != nil {
		return "", err
	}
	return string(resp.Payload), nil
}

type ClientPool struct {
	streamCh chan *Client
}

func (p *ClientPool) Pick() *Client {
	return <-p.streamCh
}

func (p *ClientPool) Put(client *Client) {
	p.streamCh <- client
}

func NewClientPool(addr string, size int) (*ClientPool, error) {
	pool := &ClientPool{
		streamCh: make(chan *Client, size),
	}
	for range size {
		client, err := NewClient(addr)
		if err != nil {
			return nil, err
		}
		pool.streamCh <- client
	}
	return pool, nil
}

func NewClient(addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithContextDialer(vsockDialer(16, 5005)),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                1 * time.Minute,
			Timeout:             3 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return nil, err
	}

	client := pb.NewEchoServiceClient(conn)
	stream, err := client.EchoStream(context.Background(), grpc.UseCompressor("gzip"))
	if err != nil {
		return nil, err
	}
	return &Client{
		stream: stream,
	}, nil
}

func vsockDialer(cid uint32, port uint32) func(context.Context, string) (net.Conn, error) {
	return func(ctx context.Context, addr string) (net.Conn, error) {
		return vsock.Dial(16, 5005, nil)
	}
}
