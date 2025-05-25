package benchmark

import (
	"context"
	"log"
	"math/rand/v2"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "nothing.com/benchmark/proto/echo"
)

const URL = "127.0.0.1:5005"

type Config struct {
	Concurrency int
	PayloadSize int
	Duration    time.Duration
}

type Benchmark struct {
	cfg    *Config
	client pb.EchoServiceClient
}

func NewBenchmark(cfg *Config) (*Benchmark, error) {
	conn, err := grpc.NewClient(URL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	client := pb.NewEchoServiceClient(conn)
	return &Benchmark{cfg: cfg, client: client}, nil
}

func (b *Benchmark) RunUnary() {
	var wg sync.WaitGroup
	var mu sync.Mutex
	qpsCount := 0
	errCount := 0

	payload := make([]byte, b.cfg.PayloadSize)
	ctx, cancel := context.WithTimeout(context.Background(), b.cfg.Duration)
	defer cancel()

	for i := 0; i < b.cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					_, err := b.client.Echo(context.Background(), &pb.EchoRequest{Payload: fillRandom(payload)})
					if err != nil {
						mu.Lock()
						errCount++
						mu.Unlock()
					}
					qpsCount++
				}
			}
		}()
	}

	wg.Wait()
	b.printQPS(qpsCount, errCount)
}

func (b *Benchmark) RunBidStream() {
	var wg sync.WaitGroup
	var mu sync.Mutex
	qpsCount := 0
	payload := make([]byte, b.cfg.PayloadSize)
	ctx, cancel := context.WithTimeout(context.Background(), b.cfg.Duration)
	defer cancel()

	for i := 0; i < b.cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stream, err := b.client.EchoStream(context.Background())
			if err != nil {
				log.Printf("stream error: %v", err)
				return
			}

			for {
				select {
				case <-ctx.Done():
					return
				default:
					if err := stream.Send(&pb.EchoRequest{Payload: fillRandom(payload)}); err != nil {
						return
					}
					if _, err := stream.Recv(); err != nil {
						return
					}
					mu.Lock()
					qpsCount++
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	b.printQPS(qpsCount, 0)
}

func (b *Benchmark) printQPS(qpsCount int, errCount int) {
	elapsed := b.cfg.Duration.Seconds()
	log.Printf("Total QPS: %.2f, %.2f", float64(qpsCount)/elapsed, float32(errCount*100)/float32(qpsCount))
}

func fillRandom(buf []byte) []byte {
	for i := range buf {
		buf[i] = byte(rand.IntN(256))
	}
	return buf
}
