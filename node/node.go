package node

import (
	"context"

	pb "github.com/mathetake/doogle/grpc"
)

type address string

var _ pb.DooglleServer = &Node{}

// Node
type Node struct {
	// should be 160 bits
	id address

	// table for routing
	// keys correspond to `distance bits`
	routingTable map[int][]*nodeInfo

	// distributed hash table points to addresses of items
	dht map[string][]address

	// map of address to item's pointer
	items map[address]*item
}

// nodeInfo contains the information for network connection
type nodeInfo struct {
	id   address
	host string
	port int
}

func (n *Node) Ping(ctx context.Context, in *pb.Empty) (*pb.PingReply, error) {
	return &pb.PingReply{Message: "Pong"}, nil
}
