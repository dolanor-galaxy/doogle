package node

import (
	"context"
	"net"

	pb "github.com/mathetake/doogle/proto"
)

// address for indices and nodes
type doogleAddress string

type PeerParameters struct {
	publicKeyPath  string
	secretKeyPath  string
	difficulty     int
	networkAddress net.Addr
}

type Node struct {
	// should be 160 bits
	dAddr doogleAddress

	// table for routing
	// keys correspond to `distance bits`
	routingTable map[int][]*nodeInfo

	// distributed hash table points to addresses of items
	dht map[string][]doogleAddress

	// map of address to item's pointer
	items map[doogleAddress]*item

	// for certification
	publicKey  string
	nonce      string
	difficulty string
}

// nodeInfo contains the information for connecting nodes
type nodeInfo struct {
	dAddr doogleAddress
	host  string
	port  int
}

func (n *Node) Ping(ctx context.Context, in *pb.Empty) (*pb.PingReply, error) {
	return &pb.PingReply{Message: "Pong"}, nil
}

func NewNode(params *PeerParameters) (*Node, error) {
	// set node parameters
	n := Node{}

	// solve network puzzle

	// start crawler
	return &n, nil
}

var _ pb.DooglleServer = &Node{}
