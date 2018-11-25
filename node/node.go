package node

import (
	"context"
	"net"
	"strings"

	pb "github.com/mathetake/doogle/grpc"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ed25519"
)

type PeerParameters struct {
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
	dht map[doogleAddressStr][]doogleAddressStr

	// map of address to item's pointer
	items map[doogleAddressStr]*item

	// for certification
	publicKey  []byte
	secretKey  []byte
	nonce      []byte
	difficulty int
}

// nodeInfo contains the information for connecting nodes
type nodeInfo struct {
	dAddr doogleAddress
	host  string
	port  int
}

func (n *Node) Ping(ctx context.Context, in *pb.NodeCertificate) (*pb.StringMessage, error) {
	// if NodeCertificate is not empty, validate and store it in dht

	return &pb.StringMessage{Message: "Pong"}, nil
}

func NewNode(params *PeerParameters) (*Node, error) {
	pk, sk, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate encryption keys")
	}

	nAdds := strings.Split(params.networkAddress.String(), ":")
	if len(nAdds) != 2 {
		return nil, errors.Errorf("invalid network address")
	}

	// set node parameters
	node := Node{
		publicKey:  pk,
		secretKey:  sk,
		difficulty: params.difficulty,
	}

	// solve network puzzle
	node.dAddr, node.nonce, err = newAddress(nAdds[0], nAdds[1], node.publicKey, node.difficulty)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate address")
	}

	// TODO: start scheduled crawler
	return &node, nil
}

// var _ pb.DooglleServer = &Node{}
