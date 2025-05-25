package main

import (
	"context"
	"io"
	"log"
	"math/rand"
	"net/http"
	"sync"

	jsoniter "github.com/json-iterator/go"
	"github.com/labstack/echo/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

		client := pool.Pick()
		respStr, err := client.Proxy(req.XID)
		if err != nil {
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
	streams []*Client
}

func (p *ClientPool) Pick() *Client {
	idx := rand.Intn(len(p.streams))
	return p.streams[idx]
}

func NewClientPool(addr string, size int) (*ClientPool, error) {
	pool := &ClientPool{}
	for i := 0; i < size; i++ {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}

		client := pb.NewEchoServiceClient(conn)
		stream, err := client.EchoStream(context.Background())
		if err != nil {
			return nil, err
		}

		pool.streams = append(pool.streams, &Client{
			stream: stream,
		})
	}
	return pool, nil
}
