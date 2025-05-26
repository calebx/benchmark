package main

import (
	"context"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/labstack/echo/v4"
	"github.com/mdlayher/vsock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	pb "nothing.com/benchmark/proto/echo"
)

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
	e := echo.New()
	e.JSONSerializer = &JsonIterSerializer{}

	pool, err := NewClientPool("127.0.0.1:5005", 16)
	if err != nil {
		log.Fatalf("failed to create client pool: %v", err)
	}

	e.POST("/", func(c echo.Context) error {
		req := &Req{}
		if err := c.Bind(req); err != nil {
			return c.JSON(http.StatusBadRequest, &Resp{
				Code:    400,
				Message: "Bad Request",
			})
		}

		client, idx := pool.Pick()
		respStr, err := client.Proxy(req.XID)
		if err != nil {
			defer func() {
				_ = client.stream.CloseSend()
				pool.streams[idx], _ = NewClient("127.0.0.1:5005")
			}()

			log.Printf("stream error: %v", err)
			if err == io.EOF {
				return c.JSON(http.StatusBadGateway, &Resp{Code: 502, Message: "Stream closed"})
			}
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
	mu     sync.Mutex
	stream pb.EchoService_EchoStreamClient
}

func (c *Client) Proxy(xid string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	err := c.stream.Send(&pb.EchoRequest{Payload: []byte(xid)})
	if err != nil {
		return "", err
	}
	resp, err := c.stream.Recv()
	if err != nil {
		return "", err
	}
	return string(resp.Payload), nil
}

type ClientPool struct {
	mu      sync.Mutex
	streams []*Client
}

func (p *ClientPool) Pick() (*Client, int) {
	idx := rand.Intn(len(p.streams))
	return p.streams[idx], idx
}

func VsockDialer(cid uint32, port uint32) func(context.Context, string) (net.Conn, error) {
	return func(ctx context.Context, addr string) (net.Conn, error) {
		return vsock.Dial(cid, port, nil)
	}
}

func NewClient(addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithContextDialer(VsockDialer(16, 5005)),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second,
			Timeout:             time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return nil, err
	}

	client := pb.NewEchoServiceClient(conn)
	stream, err := client.EchoStream(context.Background())
	if err != nil {
		return nil, err
	}
	return &Client{
		stream: stream,
	}, nil
}

func NewClientPool(addr string, size int) (*ClientPool, error) {
	pool := &ClientPool{}
	for i := 0; i < size; i++ {
		pool.streams = append(pool.streams)
	}
	return pool, nil
}
