package node

import (
	"context"
	"crypto/sha1"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/mathetake/doogle/crawler"
	"github.com/mathetake/doogle/grpc"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ed25519"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	alpha                  = 3
	bucketSize             = 20
	maxIterationOnFindNode = 10e4
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

var _ doogle.DoogleServer = &Node{}

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

func (n *Node) isValidSender(ct *doogle.NodeCertificate) bool {

	// refuse the one with the given difficulty less than its difficulty
	if len(ct.DoogleAddress) < addressLength || int(ct.Difficulty) < n.difficulty {
		return false
	}

	var da doogleAddress
	copy(da[:], ct.DoogleAddress[:])

	// if NodeCertificate is valid, update routing table with nodeInfo
	if verifyAddress(da, ct.NetworkAddress, ct.PublicKey, ct.Nonce, int(ct.Difficulty)) {
		ni := nodeInfo{
			dAddr:      da,
			nAddr:      ct.NetworkAddress,
			accessedAt: time.Now().UTC().Unix(),
		}

		// update the routing table
		n.updateRoutingTable(&ni)
		return true
	}
	return false
}

// update routingTable using a given nodeInfo
func (n *Node) updateRoutingTable(info *nodeInfo) {
	idx := getMostSignificantBit(n.DAddr.xor(info.dAddr))
	if idx < 0 {
		errors.Errorf("collision occurred")
		return
	}

	rb, ok := n.routingTable[idx]
	if !ok || rb == nil {
		panic(fmt.Sprintf("the routing table on %d not exist", idx))
	}

	// lock the bucket
	rb.mux.Lock()
	defer rb.mux.Unlock()

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
	if !n.isValidSender(in.Certificate) {
		return nil, status.Error(codes.InvalidArgument, "invalid certificate")
	}

	es := make([]doogleAddress, len(in.EdgeURLs))
	for i, e := range in.EdgeURLs {
		es[i] = sha1.Sum([]byte(e))
	}

	// calc item's address
	h := sha1.Sum([]byte(in.Url))
	itemAddr := doogleAddressStr(h[:])

	// calc index's address
	h = sha1.Sum([]byte(in.Index))
	idxAddr := doogleAddressStr(h[:])

	it := &item{
		url:         in.Url,
		dAddrStr:    itemAddr,
		title:       in.Title,
		edges:       es,
		description: in.Description,
	}

	// store item on index
	actual, _ := n.dht.LoadOrStore(idxAddr, &dhtValue{
		itemAddresses: []doogleAddressStr{},
		mux:           sync.Mutex{},
	})

	dhtV, ok := actual.(*dhtValue)
	if !ok {
		return nil, status.Error(codes.Internal, "failed to convert to *dhtValue")
	}

	dhtV.mux.Lock()
	defer dhtV.mux.Unlock()

	var included = false
	for _, addr := range dhtV.itemAddresses {
		if addr == it.dAddrStr {
			included = true
		}
	}

	if !included {
		dhtV.itemAddresses = append(dhtV.itemAddresses, it.dAddrStr)
	}

	if raw, loaded := n.items.LoadOrStore(it.dAddrStr, it); loaded {
		prev := raw.(*item)
		it.localRank = prev.localRank
		n.items.Store(it.dAddrStr, it)
	}

	return nil, nil
}

func (n *Node) FindNode(ctx context.Context, in *doogle.FindNodeRequest) (*doogle.NodeInfos, error) {
	if !n.isValidSender(in.Certificate) {
		return nil, status.Error(codes.InvalidArgument, "invalid certificate")
	}

	var targetAddr doogleAddress
	copy(targetAddr[:], in.DoogleAddress[:])

	ret, err := n.findNode(targetAddr)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "findNode failed: %v", err)
	}
	return &doogle.NodeInfos{Infos: ret}, nil
}

func (n *Node) findNode(targetAddr doogleAddress) ([]*doogle.NodeInfo, error) {

	ret, err := n.findNearestNode(targetAddr)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "findNearestNode failed: %v", err)
	}

	prevMap := map[string]struct{}{}
	for _, r := range ret {
		prevMap[r.NetworkAddress] = struct{}{}
	}
	var prevNum = len(ret)

	for i := 0; i < maxIterationOnFindNode; i++ {
		for _, r := range ret {
			// ask nearest nodes for nodeInfo nearest to targetAddress
			conn, err := grpc.Dial(r.NetworkAddress, grpc.WithInsecure())
			if err != nil {
				n.logger.Errorf("did not connect: %v", err)
				continue
			}

			c := doogle.NewDoogleClient(conn)
			rep, err := c.FindNode(context.Background(), &doogle.FindNodeRequest{
				Certificate:   n.certificate,
				DoogleAddress: targetAddr[:],
			})

			if err != nil {
				n.logger.Errorf("failed to call FindNode: %v", err)
				continue
			}
			conn.Close()

			// update routing table
			for _, r := range rep.Infos {
				var dAddr doogleAddress
				copy(dAddr[:], r.DoogleAddress)
				n.updateRoutingTable(&nodeInfo{
					dAddr: dAddr,
					nAddr: r.NetworkAddress,
				})
			}
		}

		// get nearest nodes from its routing table
		ret, err = n.findNearestNode(targetAddr)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "findNearestNode failed: %v", err)
		}

		// check duplication
		var cnt int
		for _, r := range ret {
			if _, ok := prevMap[r.NetworkAddress]; ok {
				cnt++
			}
		}

		// if anything hasn't changed, break and return
		if cnt == prevNum {
			break
		}

		// reset prevMap/prevNum for next loop
		prevMap = map[string]struct{}{}
		for _, r := range ret {
			prevMap[r.NetworkAddress] = struct{}{}
		}
		prevNum = len(ret)
	}

	return ret, nil
}

func (n *Node) findNearestNode(targetAddr doogleAddress) ([]*doogle.NodeInfo, error) {
	msb := getMostSignificantBit(n.DAddr.xor(targetAddr))
	if msb < 0 {
		return nil, status.Error(codes.Internal, "collision occurred")
	}

	rb, ok := n.routingTable[msb]
	if !ok || rb == nil {
		panic(fmt.Sprintf("the routing table on %d not exist", msb))
	}

	rb.mux.Lock()
	defer rb.mux.Unlock()

	if len(rb.bucket) < alpha {
		ret := make([]*doogle.NodeInfo, len(rb.bucket))
		for i := range ret {
			ret[i] = &doogle.NodeInfo{
				DoogleAddress:  rb.bucket[i].dAddr[:],
				NetworkAddress: rb.bucket[i].nAddr,
			}
		}

		// TODO: handle the case where len(rb.bucket) == 0
		return ret, nil
	}

	ns := make([]*nodeInfo, len(rb.bucket))
	copy(ns, rb.bucket)

	sort.Slice(ns, func(i, j int) bool {
		return ns[i].dAddr.xor(targetAddr).lessThanEqual(ns[j].dAddr.xor(targetAddr))
	})

	ret := make([]*doogle.NodeInfo, alpha)
	for i := range ret {
		ret[i] = &doogle.NodeInfo{
			NetworkAddress: ns[i].nAddr,
			DoogleAddress:  ns[i].dAddr[:],
		}
	}
	return ret, nil
}

func (n *Node) FindIndex(ctx context.Context, in *doogle.FindIndexRequest) (*doogle.FindIndexReply, error) {
	if !n.isValidSender(in.Certificate) {
		return nil, status.Error(codes.InvalidArgument, "invalid certificate")
	}

	var rep = &doogle.FindIndexReply{}

	raw, ok := n.dht.Load(in.DoogleAddress)
	if !ok {
		var res *doogle.FindIndexReply_NodeInfos
		var err error

		res.NodeInfos, err = n.FindNode(ctx, &doogle.FindNodeRequest{
			Certificate:   in.Certificate,
			DoogleAddress: []byte(in.DoogleAddress),
		})

		if err != nil {
			return nil, status.Errorf(codes.Internal, "FindNode failed: %v", err)
		}
		rep.Result = res
		return rep, nil
	}

	dhtV, ok := raw.(*dhtValue)
	if !ok {
		return nil, status.Error(codes.Internal, "failed to convert to *dhtValue")
	}

	as := dhtV.itemAddresses // copy slice
	var res *doogle.FindIndexReply_Items
	items := make([]*doogle.Item, 0)
	for _, addr := range as {
		if raw, ok := n.items.Load(addr); ok {
			if it, ok := raw.(*item); ok {
				items = append(items, &doogle.Item{
					Url:       it.url,
					LocalRank: it.localRank,
				})
			}
		}
	}
	res.Items.Items = items
	rep.Result = res
	return rep, nil
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

	di := &doogle.StoreItemRequest{
		Url:         in.Message,
		Title:       title,
		Description: desc,
		EdgeURLs:    eURLs,
		Certificate: n.certificate,
	}

	// make StoreItem requests to store the url into DHT
	for _, token := range tokens {
		addr := sha1.Sum([]byte(token))
		di.Index = token

		rep, err := n.findNearestNode(addr)
		if err != nil {
			n.logger.Errorf("failed to find node for %s : %v", token, err)
			continue
		}

		// if the reply is empty, Store item into its own table
		if len(rep) == 0 {
			_, err = n.StoreItem(context.Background(), di)
			if err != nil {
				n.logger.Errorf("failed to call StoreItem: %v", err)
			}
		} else {
			// call StoreItem request on closest nodes
			var wg = sync.WaitGroup{}
			for _, ni := range rep {
				wg.Add(1)
				go func() {
					defer wg.Done()
					conn, err := grpc.Dial(ni.NetworkAddress, grpc.WithInsecure())
					defer conn.Close()
					if err != nil {
						n.logger.Errorf("did not connect: %v", err)
						return
					}
					c := doogle.NewDoogleClient(conn)
					_, err = c.StoreItem(context.Background(), di)
					if err != nil {
						n.logger.Errorf("failed to call StoreItem: %v", err)
						return
					}
				}()
			}
			wg.Wait()
		}
	}
	return nil, nil
}

func (n *Node) PingWithCertificate(ctx context.Context, in *doogle.NodeCertificate) (*doogle.StringMessage, error) {
	if n.isValidSender(in) {
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
	r, err := c.PingWithCertificate(context.Background(), n.certificate)
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
		NetworkAddress: nAddr,
		DoogleAddress:  node.DAddr[:],
		PublicKey:      node.publicKey,
		Nonce:          node.nonce,
		Difficulty:     int32(node.difficulty),
	}

	// TODO: start PageRank computing scheduler
	return &node, nil
}
