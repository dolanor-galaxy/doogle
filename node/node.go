package node

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/mathetake/doogle/crawler"

	"github.com/mathetake/doogle/grpc"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ed25519"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// constant network parameters
const (
	alpha      = 3
	bucketSize = 20
)

type item struct {
	dAddrStr doogleAddressStr

	url         string
	title       string
	description string

	// outgoing hyperlinks
	edges []doogleAddress

	// localRank represents computed locally PageRank
	localRank float64
}

type Node struct {
	DAddr doogleAddress

	// table for routing
	// keys correspond to `distance bits`
	// type: map{int -> *routingBucket}
	routingTable map[int]*routingBucket

	// distributed hash table points to addresses of items
	// type: map{doogleAddressStr -> *dhtValue}
	dht sync.Map

	// map of address to item's pointer
	// type: map{doogleAddressStr -> *item}
	items sync.Map

	// for certificate creation
	publicKey  []byte
	secretKey  []byte
	nonce      []byte
	difficulty int

	// certificate
	certificate *doogle.NodeCertificate

	// logger
	logger *logrus.Logger

	// crawler
	crawler crawler.Crawler
}

// nodeInfo contains the information for connecting nodes
type nodeInfo struct {
	dAddr      doogleAddress
	nAddr      string
	accessedAt int64
}

type routingBucket struct {
	bucket []*nodeInfo
	mux    sync.Mutex
}

// pop item on `idx` and then append `ni`
func (rb *routingBucket) popAndAppend(idx int, ni *nodeInfo) {
	prev := rb.bucket
	l := len(prev)
	rb.bucket = make([]*nodeInfo, l)
	for i := 0; i < l; i++ {
		if i == l-1 {
			rb.bucket[i] = ni
		} else if i < idx {
			rb.bucket[i] = prev[i]
		} else {
			rb.bucket[i] = prev[i+1]
		}
	}
}

type dhtValue struct {
	itemAddresses []doogleAddressStr
	mux           sync.Mutex
}

func (n *Node) isValidSender(ctx context.Context, rawAddr, pk, nonce []byte, difficulty int) bool {

	// refuse the one with the given difficulty less than its difficulty
	if len(rawAddr) < addressLength || difficulty < n.difficulty {
		return false
	}

	var da doogleAddress
	copy(da[:], rawAddr[:])

	if pr, ok := peer.FromContext(ctx); ok {
		nAdd := pr.Addr.String()
		// if NodeCertificate is valid, update routing table with nodeInfo
		if verifyAddress(da, nAdd, pk, nonce, difficulty) {
			ni := nodeInfo{
				dAddr:      da,
				nAddr:      nAdd,
				accessedAt: time.Now().UTC().Unix(),
			}

			// update the routing table
			n.updateRoutingTable(&ni)
			return true
		}
	}
	return false
}

// update routingTable using a given nodeInfo
func (n *Node) updateRoutingTable(info *nodeInfo) {
	idx := getMostSignificantBit(n.DAddr.xor(info.dAddr))

	rb, ok := n.routingTable[idx]
	if !ok || rb == nil {
		panic(fmt.Sprintf("the routing table on %d not exist", idx))
	}

	// lock the bucket
	rb.mux.Lock()
	defer rb.mux.Unlock() // unlock the bucket
	for i, n := range rb.bucket {
		if n.dAddr == info.dAddr {
			// Update accessedAt on target node.
			n.accessedAt = time.Now().UTC().Unix()

			// move the target to tail of the bucket
			rb.popAndAppend(i, n)
			return
		}
	}

	ni := &nodeInfo{
		nAddr:      info.nAddr,
		dAddr:      info.dAddr,
		accessedAt: time.Now().UTC().Unix(),
	}

	if len(rb.bucket) < bucketSize {
		rb.bucket = append(rb.bucket, ni)
	} else {
		rb.popAndAppend(0, ni)
	}
}

func (n *Node) StoreItem(ctx context.Context, in *doogle.StoreItemRequest) (*doogle.Empty, error) {
	if !n.isValidSender(
		ctx, in.Certificate.DoogleAddress,
		in.Certificate.PublicKey,
		in.Certificate.Nonce,
		int(in.Certificate.Difficulty)) {
		return nil, status.Error(codes.InvalidArgument, "invalid certificate")
	}

	es := make([]doogleAddress, len(in.Edges))
	for i, e := range in.Edges {
		es[i] = sha1.Sum(e)
	}

	it := &item{
		url:         in.Url,
		title:       in.Title,
		edges:       es,
		description: in.Description,
	}

	// store item on index
	idx := doogleAddressStr(in.Index)
	actual, _ := n.items.LoadOrStore(idx, &dhtValue{
		itemAddresses: []doogleAddressStr{},
		mux:           sync.Mutex{},
	})

	dhtV, ok := actual.(*dhtValue)
	if !ok {
		return nil, status.Error(codes.Internal, "failed to convert to *dhtValue")
	}

	dhtV.mux.Lock()
	dhtV.itemAddresses = append(dhtV.itemAddresses, it.dAddrStr)
	n.items.Store(it.dAddrStr, it)
	defer dhtV.mux.Unlock()
	return nil, nil
}

func (n *Node) FindIndex(ctx context.Context, in *doogle.FindIndexRequest) (*doogle.FindIndexReply, error) {
	return nil, nil
}

func (n *Node) FindNode(ctx context.Context, in *doogle.FindNodeRequest) (*doogle.FindNodeReply, error) {
	return nil, nil
}

func (n *Node) GetIndex(ctx context.Context, in *doogle.StringMessage) (*doogle.GetIndexReply, error) {
	return nil, nil
}

func (n *Node) PostUrl(ctx context.Context, in *doogle.StringMessage) (*doogle.Empty, error) {
	// analyze the given url
	title, desc, tokens, eURLs, err := n.crawler.AnalyzeURL(in.Message)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to analyze to url: %v", err)
	}

	// prepare StoreItemRequest
	edges := make([][]byte, len(eURLs))
	for i, u := range eURLs {
		edges[i] = sha1.Sum([]byte(u))[:]
	}

	di := &doogle.StoreItemRequest{
		Url:         in.Message,
		Title:       title,
		Description: desc,
		Edges:       edges,
		Certificate: n.certificate,
		Index:       nil,
	}

	// make StoreItem requests to store the url into DHT
	for _, token := range tokens {
		addr := sha1.Sum([]byte(token))[:]
		di.Index = string(addr)

		// find closes nodes to token's address
		rep, err := n.FindNode(ctx, &doogle.FindNodeRequest{
			Certificate:   n.certificate,
			DoogleAddress: addr,
		})

		if err != nil {
			n.logger.Errorf("failed to find node for %s : %v", hex.EncodeToString(addr), err)
		}

		// call StoreItem request on closest nodes
		for _, ni := range rep.NodeInfos.Infos {
			conn, err := grpc.Dial(ni.NetworkAddress, grpc.WithInsecure())
			if err != nil {
				return nil, status.Errorf(codes.Internal, "did not connect: %v", err)
			}

			c := doogle.NewDoogleClient(conn)
			_, err = c.StoreItem(ctx, di)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to call StoreItem: %v", err)
			}
			conn.Close()
		}

	}
	return nil, nil
}

func (n *Node) PingWithCertificate(ctx context.Context, in *doogle.NodeCertificate) (*doogle.StringMessage, error) {
	if n.isValidSender(ctx, in.DoogleAddress, in.PublicKey, in.Nonce, int(in.Difficulty)) {
		return &doogle.StringMessage{Message: "pong"}, nil
	}
	return nil, status.Error(codes.InvalidArgument, "invalid certificate")
}

func (n *Node) Ping(ctx context.Context, in *doogle.StringMessage) (*doogle.StringMessage, error) {
	return &doogle.StringMessage{Message: "pong"}, nil
}

func (n *Node) PingTo(ctx context.Context, in *doogle.NodeInfo) (*doogle.StringMessage, error) {
	conn, err := grpc.Dial(in.NetworkAddress, grpc.WithInsecure())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "did not connect: %v", err)
	}
	defer conn.Close()

	c := doogle.NewDoogleClient(conn)
	r, err := c.PingWithCertificate(ctx, n.certificate)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "c.Ping failed: %v", err)
	}
	return &doogle.StringMessage{Message: r.Message}, nil
}

func NewNode(difficulty int, nAddr string, logger *logrus.Logger, cr crawler.Crawler) (*Node, error) {
	pk, sk, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate encryption keys")
	}

	// initialize routing table
	rt := map[int]*routingBucket{}
	for i := 0; i < addressBits; i++ {
		b := make([]*nodeInfo, 0, bucketSize)
		rt[i] = &routingBucket{bucket: b, mux: sync.Mutex{}}
	}

	// set node parameters
	node := Node{
		publicKey:    pk,
		secretKey:    sk,
		difficulty:   difficulty,
		routingTable: rt,
		logger:       logger,
		crawler:      cr,
	}

	// solve network puzzle
	node.DAddr, node.nonce, err = newNodeAddress(nAddr, node.publicKey, node.difficulty)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate address")
	}

	node.certificate = &doogle.NodeCertificate{
		DoogleAddress: node.DAddr[:],
		PublicKey:     node.publicKey,
		Nonce:         node.nonce,
		Difficulty:    int32(node.difficulty),
	}

	// TODO: start PageRank computing scheduler
	return &node, nil
}

var _ doogle.DoogleServer = &Node{}
