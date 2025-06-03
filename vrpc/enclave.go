package vrpc

import (
	"fmt"
	"net"
	"time"

	"github.com/mdlayher/vsock"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	pb "nothing.com/benchmark/vrpc/gen"
)

type Server interface {
	Start()
	Close()
}

func NewServer(port uint32, dispatcher Dispatcher, isVsock bool) Server {
	return &server{
		Port:       port,
		IsVsock:    isVsock,
		Dispatcher: dispatcher,
	}
}

type server struct {
	// internally it is a classical gRPC server
	pb.UnimplementedVrpcServer

	Port       uint32
	IsVsock    bool
	Dispatcher Dispatcher
	grpcServer *grpc.Server
}

func (s *server) Start() {
	s.grpcServer = grpc.NewServer(
		// NOTE: it is well tuned for current use case
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    1 * time.Minute,
			Timeout: 5 * time.Second,
		}),
		// NOTE: it is well tuned for current use case
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             1 * time.Minute,
			PermitWithoutStream: true,
		}),
	)
	pb.RegisterVrpcServer(s.grpcServer, s)

	var lis net.Listener
	var err error
	if s.IsVsock {
		lis, err = vsock.Listen(s.Port, nil)
	} else {
		lis, err = net.Listen("tcp", fmt.Sprintf(":%d", s.Port))
	}
	if err != nil {
		log.Fatalf("listen to network err: %v", err)
	}
	if err = s.grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func (s *server) Close() {
	s.grpcServer.Stop()
}

func (s *server) Do(stream pb.Vrpc_DoServer) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			return err
		}

		resp := &pb.BatchResp{}
		log.WithField("len", len(req.Requests)).Info("do_req")
		for _, r := range req.GetRequests() {
			code, message, payload := s.Dispatcher.Dispatch(r.GetCommand(), r.GetPayload())
			resp.Response = append(resp.Response, &pb.Response{
				Code:    code,
				Message: message,
				Payload: payload,
			})
		}

		if err = stream.Send(resp); err != nil {
			return err
		}
	}
}
