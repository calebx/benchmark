package vrpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockDispatcher struct{}

func (*mockDispatcher) Dispatch(cmd string, payload []byte) (uint32, string, []byte) {
	if cmd == "panic" {
		panic("panic")
	}
	return 0, cmd, payload
}

func (*mockDispatcher) Register(cmd string, handler interface{}) {
	// nothing
}

func TestMain(m *testing.M) {
	s := NewServer(5670, &mockDispatcher{}, false)

	go s.Start()
	defer func() {
		s.Close()
	}()

	time.Sleep(3 * time.Second)
	m.Run()
}

func TestClient(t *testing.T) {
	ctx := context.Background()
	cli, err := NewClient(ctx, 16, 5670, "127.0.0.1:5670", false, 2)
	defer cli.Close()

	assert.NoError(t, err)
	assert.NotNil(t, cli)

	t.Run("happy path", func(t *testing.T) {
		resp, err := cli.Invoke(ctx, "test", []byte("hello world"))
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, uint32(0), resp.Code)
		assert.Equal(t, "test", resp.Message)
		assert.Equal(t, []byte("hello world"), resp.Payload)
	})
}
