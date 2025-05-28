package vsockcli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"path"

	"github.com/mdlayher/vsock"
)

type enclaveClient struct {
	Config       *Config
	clients      []*http.Client
	clientsIdxCh chan int
}

func newEnclaveClient(cfg *Config) (VsockCli, error) {
	cfg = cfg.setupDefaults()

	clients := make([]*http.Client, 0, cfg.CliCount)
	for range cfg.CliCount {
		clients = append(clients, newHttpClient(cfg))
	}

	clientsIdxCh := make(chan int, cfg.CliCount*cfg.MaxConnsPerCli)
	for i := range cfg.CliCount * cfg.MaxConnsPerCli {
		clientsIdxCh <- i % cfg.CliCount
	}

	return &enclaveClient{
		Config:       cfg,
		clients:      clients,
		clientsIdxCh: clientsIdxCh,
	}, nil
}

func newHttpClient(cfg *Config) *http.Client {
	cfg = cfg.setupDefaults()

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				// is_mocked for local development
				if cfg.IsMocked {
					return net.DialTimeout(network, addr, cfg.Timeout)
				}
				return vsock.Dial(cfg.CID, cfg.Port, &vsock.Config{})
			}
		},
		MaxConnsPerHost:     cfg.MaxConnsPerCli,
		MaxIdleConns:        cfg.MaxConnsPerCli,
		MaxIdleConnsPerHost: cfg.MaxConnsPerCli,
		IdleConnTimeout:     cfg.Timeout,
		ForceAttemptHTTP2:   true,
	}

	log.Println("new http_client initialized")
	return &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
	}
}

func (e *enclaveClient) Get(ctx context.Context, fullPath string) (buf []byte, err error) {
	i := <-e.clientsIdxCh
	defer func() {
		if err != nil {
			e.clients[i] = newHttpClient(e.Config)
		}
		e.clientsIdxCh <- i
	}()
	client := e.clients[i]

	uri := path.Join(e.Config.Origin, fullPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, fmt.Errorf("enclave_client do Get from [%s] create request failed [%w]", uri, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("enclave_client do Get from [%s] failed [%w]", uri, err)
	}
	defer func() { _ = resp.Body.Close() }()

	buf, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("enclave_client do Get from [%s] read response body failed [%w]", uri, err)
	}
	return buf, nil
}

func (e *enclaveClient) Post(ctx context.Context, fullPath string, payload interface{}) (buf []byte, err error) {
	i := <-e.clientsIdxCh
	defer func() {
		if err != nil {
			e.clients[i] = newHttpClient(e.Config)
		}
		e.clientsIdxCh <- i
	}()
	client := e.clients[i]

	buf, err = json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal req [%+v] failed [%w]", payload, err)
	}

	uri, _ := url.JoinPath(e.Config.Origin, fullPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri, bytes.NewBuffer(buf))
	if err != nil {
		return nil, fmt.Errorf("enclave_client do Get from [%s] create request failed [%w]", uri, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("enclave_client do POST request to url [%s] failed [%w]", uri, err)
	}
	defer func() { _ = resp.Body.Close() }()

	buf, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("enclave_client do POST from [%s] read response body failed [%w]", uri, err)
	}
	return buf, nil
}
