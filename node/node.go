package node

import (
	"context"
	pb "github.com/mathetake/doogle/grpc"
)

type Node struct{}

func (n *Node) Ping(ctx context.Context, in *pb.Empty) (*pb.PingReply, error) {
	return &pb.PingReply{Message: "Pong"}, nil
}
