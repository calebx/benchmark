package main

import (
	"context"
	"log"
	"time"

	"github.com/mdlayher/vsock"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/keepalive"

	pb "nothing.com/benchmark/proto/echo"
)

type echoServer struct {
	pb.UnimplementedEchoServiceServer
}

func (s *echoServer) Echo(ctx context.Context, req *pb.EchoRequest) (*pb.EchoResponse, error) {
	return &pb.EchoResponse{Payload: req.Payload[:32]}, nil
}

func (s *echoServer) EchoStream(stream pb.EchoService_EchoStreamServer) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			return err
		}
		time.Sleep(time.Millisecond)
		payload := req.Payload
		if len(payload) > 32 {
			payload = payload[:32]
		}
		if err := stream.Send(&pb.EchoResponse{Payload: payload}); err != nil {
			return err
		}
	}
}

func main() {
	grpcServer := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    10 * time.Second,
			Timeout: 3 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             time.Minute,
			PermitWithoutStream: true,
		}),
	)
	pb.RegisterEchoServiceServer(grpcServer, &echoServer{})
	log.Println("Echo gRPC registered")

	// lis, err := net.Listen("tcp", ":5005")
	lis, err := vsock.Listen(5005, nil)
	if err != nil {
		log.Fatalf("failed to listen vsock: %v", err)
	}
	log.Println("Create VSOCK listener on port :5005")

	log.Println("Echo gRPC server start listening on :5005")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
