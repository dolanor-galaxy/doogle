package node

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"google.golang.org/grpc"

	pb "github.com/mathetake/doogle/grpc"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ed25519"
	"google.golang.org/grpc/peer"
)

type item struct {
	url   string
	dAddr doogleAddress

	// outgoing hyperlinks
	edges []doogleAddress

	// localRank represents computed locally PageRank
	localRank float64
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
	port  string
}

func (n *Node) isValidSender(ctx context.Context, rawAddr []byte, pk, nonce []byte, difficulty int) bool {
	if len(rawAddr) < addressLength {
		return false
	}

	var da doogleAddress
	copy(da[:], rawAddr[:addressLength])

	if pr, ok := peer.FromContext(ctx); ok {
		addr := strings.Split(pr.Addr.String(), ":")

		// if NodeCertificate is valid, update routing table with nodeInfo
		if verifyAddress(da, addr[0], addr[1], pk, nonce, difficulty) {
			ni := nodeInfo{
				dAddr: da,
				host:  addr[0],
				port:  addr[1],
			}
			// if it is verified, update routing table
			n.updateRoutingTable(&ni)
			return true
		}
	}
	return false
}

func (n *Node) updateRoutingTable(info *nodeInfo) {
	// update routingTable using given nodeInfo
}

func (n *Node) StoreItem(ctx context.Context, in *pb.StoreItemRequest) (*pb.Empty, error) {
	return nil, nil
}
func (n *Node) FindIndex(ctx context.Context, in *pb.FindIndexRequest) (*pb.FindIndexReply, error) {
	return nil, nil
}

func (n *Node) FindNode(ctx context.Context, in *pb.FindNodeRequest) (*pb.FindeNodeReply, error) {
	return nil, nil
}

func (n *Node) GetIndex(ctx context.Context, in *pb.StringMessage) (*pb.GetIndexReply, error) {
	return nil, nil
}

func (n *Node) PostUrl(ctx context.Context, in *pb.StringMessage) (*pb.StringMessage, error) {
	return nil, nil
}

func (n *Node) Ping(ctx context.Context, in *pb.NodeCertificate) (*pb.StringMessage, error) {
	// TODO: logging the result of validation
	n.isValidSender(ctx, in.DoogleAddress, in.PublicKey, in.Nonce, int(in.Difficulty))
	return &pb.StringMessage{Message: "Pong"}, nil
}

func (n *Node) PingTo(ctx context.Context, in *pb.NodeInfo) (*pb.StringMessage, error) {
	conn, err := grpc.Dial(in.Host+":"+in.Port, grpc.WithInsecure())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "did not connect: %v", err)
	}
	defer conn.Close()

	c := pb.NewDoogleClient(conn)
	r, err := c.Ping(ctx, &pb.NodeCertificate{
		DoogleAddress: n.dAddr[:addressLength],
		PublicKey:     n.publicKey,
		Nonce:         n.nonce,
		Difficulty:    int32(n.difficulty),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "c.Ping failed: %v", err)
	}

	return &pb.StringMessage{Message: r.Message}, nil
}

func NewNode(difficulty int, host, port string) (*Node, error) {
	pk, sk, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate encryption keys")
	}

	// set node parameters
	node := Node{
		publicKey:  pk,
		secretKey:  sk,
		difficulty: difficulty,
	}

	// solve network puzzle
	node.dAddr, node.nonce, err = newNodeAddress(host, port, node.publicKey, node.difficulty)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate address")
	}

	// TODO: start scheduled crawler
	// TODO: start PageRank computing scheduler
	return &node, nil
}

var _ pb.DoogleServer = &Node{}
