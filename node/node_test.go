package node

import (
	"context"
	"crypto/sha1"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/mathetake/doogle/grpc"
	"google.golang.org/grpc"
	"gotest.tools/assert"
)

var zeroAddress = doogleAddress{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}

var zeroInfo = &nodeInfo{zeroAddress, "", 0}

const (
	localhost = "[::1]"
	port1     = ":3841"
	port2     = ":3842"
	port3     = ":3833"
	port4     = ":3834"
	port5     = ":3835"
	port6     = ":3836"
)

var testServers = []*struct {
	port       string
	difficulty int
	node       *Node
}{
	{port: port1, difficulty: 1},
	{port: port2, difficulty: 1},
	{port: port3, difficulty: 1},
	{port: port4, difficulty: 1},
	{port: port5, difficulty: 1},
	{port: port6, difficulty: 1},
}

func TestMain(m *testing.M) {
	for _, ts := range testServers {
		ts.node = runServer(ts.port, ts.difficulty)
	}
	os.Exit(m.Run())
}

// set up doogle server on specified port
func runServer(port string, difficulty int) *Node {
	node, err := NewNode(difficulty, localhost+port, nil, nil)
	if err != nil {
		log.Fatalf("failed to craete new node: %v", err)
	}

	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	doogle.RegisterDoogleServer(s, node)
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	time.Sleep(100 * time.Millisecond)
	return node
}

func resetRoutingTable() {
	for i := range testServers {
		// reset routing table on testServers[0]
		rt := map[int]*routingBucket{}
		for i := 0; i < addressBits; i++ {
			b := make([]*nodeInfo, 0, bucketSize)
			rt[i] = &routingBucket{bucket: b, mux: sync.Mutex{}}
		}
		testServers[i].node.routingTable = rt
	}
}

func TestVerifyAddressOnServers(t *testing.T) {
	for i, cc := range testServers {
		c := cc
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			assert.Equal(t, true, verifyAddress(c.node.DAddr, localhost+c.port, c.node.publicKey, c.node.nonce, c.node.difficulty))
		})
	}
}

func TestPopAndAppend(t *testing.T) {
	targetInfo := &nodeInfo{dAddr: testServers[0].node.DAddr}

	for i, cc := range []struct {
		idx    int
		before []*nodeInfo
		after  []*nodeInfo
	}{
		{
			idx:    0,
			before: []*nodeInfo{zeroInfo},
			after:  []*nodeInfo{targetInfo},
		},
		{
			idx:    0,
			before: []*nodeInfo{zeroInfo, zeroInfo, zeroInfo},
			after:  []*nodeInfo{zeroInfo, zeroInfo, targetInfo},
		},
		{
			idx:    0,
			before: []*nodeInfo{targetInfo, zeroInfo, zeroInfo},
			after:  []*nodeInfo{zeroInfo, zeroInfo, targetInfo},
		},
		{
			idx:    1,
			before: []*nodeInfo{targetInfo, zeroInfo, zeroInfo},
			after:  []*nodeInfo{targetInfo, zeroInfo, targetInfo},
		},
		{
			idx:    2,
			before: []*nodeInfo{targetInfo, zeroInfo},
			after:  []*nodeInfo{targetInfo, targetInfo},
		},
		{
			idx:    2,
			before: []*nodeInfo{targetInfo, zeroInfo, zeroInfo},
			after:  []*nodeInfo{targetInfo, zeroInfo, targetInfo},
		},
		{
			idx:    2,
			before: []*nodeInfo{targetInfo, zeroInfo, zeroInfo, targetInfo, zeroInfo},
			after:  []*nodeInfo{targetInfo, zeroInfo, targetInfo, zeroInfo, targetInfo},
		},
	} {
		c := cc
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			rb := routingBucket{mux: sync.Mutex{}, bucket: c.before}
			rb.popAndAppend(c.idx, targetInfo)
			assert.Equal(t, len(c.after), len(rb.bucket))
			for i, exp := range c.after {
				assert.Equal(t, *rb.bucket[i], *exp)
			}
		})
	}
}

func TestPingWithCertificate(t *testing.T) {
	for i, cc := range testServers {
		var tIDx = i + 1
		if tIDx == len(testServers) {
			tIDx = 0
		}
		c := cc
		tc := testServers[tIDx]
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			conn, err := grpc.Dial(localhost+c.port, grpc.WithInsecure())
			if err != nil {
				log.Fatalf("did not connect: %v", err)
			}
			defer conn.Close()
			client := doogle.NewDoogleClient(conn)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			r, err := client.PingWithCertificate(ctx, tc.node.certificate)
			assert.Equal(t, nil, err)
			assert.Equal(t, "pong", r.Message)
		})
	}
}

func TestPingWithoutCertificate(t *testing.T) {
	for i, cc := range testServers {
		c := cc
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			conn, err := grpc.Dial(localhost+c.port, grpc.WithInsecure())
			if err != nil {
				log.Fatalf("did not connect: %v", err)
			}
			defer conn.Close()
			client := doogle.NewDoogleClient(conn)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			r, err := client.Ping(ctx, &doogle.StringMessage{Message: ""})
			assert.Equal(t, nil, err)
			assert.Equal(t, "pong", r.Message)
		})
	}

}

func TestPingTo(t *testing.T) {
	for i, cc := range []struct {
		fromPort   string
		toPort     string
		isErrorNil bool
	}{
		{port1, port2, true},
		{port1, port3, true},
		{port3, port5, true},
		{port3, ":1231", false},
	} {
		c := cc
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			conn, err := grpc.Dial(localhost+c.fromPort, grpc.WithInsecure())
			if err != nil {
				log.Fatalf("did not connect: %v", err)
			}
			defer conn.Close()
			client := doogle.NewDoogleClient(conn)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			_, err = client.PingTo(ctx, &doogle.NodeInfo{NetworkAddress: localhost + c.toPort})
			assert.Equal(t, c.isErrorNil, err == nil)
		})
	}
}

func TestIsValidSender(t *testing.T) {
	for i, cc := range []struct {
		networkAddr string
		rawAddr     []byte
		pk          []byte
		nonce       []byte
		difficulty  int32
		exp         bool
	}{
		{"", nil, nil, nil, 10, false},
		{"localhost1234", []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, nil, nil, 10, false},
		{
			"ab80",
			[]byte{137, 247, 252, 74, 101, 232, 49, 193, 122, 237, 123, 84, 199, 94, 78, 176, 92, 104, 69, 253},
			[]byte("pk"), []byte{172, 171, 254, 98, 171, 6, 169, 186, 105, 145},
			2,
			true,
		},
		{
			"ab80",
			[]byte{137, 247, 252, 74, 101, 232, 49, 193, 122, 237, 123, 84, 199, 94, 78, 176, 92, 104, 69, 253},
			[]byte("pk"), []byte{172, 171, 254, 98, 171, 6, 169, 186, 105, 145},
			10,
			false,
		},
		{
			"ab80",
			[]byte{137, 247, 252, 74, 101, 232, 49, 193, 122, 237, 123, 84, 199, 94, 78, 176, 92, 104, 69, 253},
			[]byte("pk"), []byte{172, 171, 254, 98, 171, 6, 169, 186, 105, 145},
			1,
			false,
		},
	} {
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			c := cc
			node, err := NewNode(2, "bar", nil, nil)
			if err != nil {
				t.Fatalf("failed to create new node: %v", err)
			}
			actual := node.isValidSender(&doogle.NodeCertificate{
				DoogleAddress:  c.rawAddr,
				NetworkAddress: c.networkAddr,
				PublicKey:      c.pk,
				Nonce:          c.nonce,
				Difficulty:     c.difficulty,
			})
			assert.Equal(t, c.exp, actual)
		})
	}
}

func TestUpdateRoutingTable(t *testing.T) {
	// reset routing table
	resetRoutingTable()

	// update target nodeInfo
	target := &nodeInfo{
		dAddr: testServers[1].node.DAddr,
		nAddr: localhost + testServers[1].port,
	}
	msb := getMostSignificantBit(target.dAddr.xor(testServers[0].node.DAddr))

	for i, cc := range []struct {
		before, after []*nodeInfo
	}{
		{
			before: []*nodeInfo{},
			after:  []*nodeInfo{target},
		},
		{
			before: []*nodeInfo{zeroInfo},
			after:  []*nodeInfo{zeroInfo, target},
		},
		{
			before: []*nodeInfo{target, zeroInfo},
			after:  []*nodeInfo{zeroInfo, target},
		},
		{
			before: []*nodeInfo{zeroInfo, zeroInfo},
			after:  []*nodeInfo{zeroInfo, zeroInfo, target},
		},
		{
			before: []*nodeInfo{
				zeroInfo, zeroInfo, zeroInfo, zeroInfo, zeroInfo,
				zeroInfo, zeroInfo, target, zeroInfo, zeroInfo,
				zeroInfo, zeroInfo, zeroInfo, zeroInfo, zeroInfo,
				zeroInfo, zeroInfo, zeroInfo, zeroInfo, zeroInfo,
			},
			after: []*nodeInfo{
				zeroInfo, zeroInfo, zeroInfo, zeroInfo, zeroInfo,
				zeroInfo, zeroInfo, zeroInfo, zeroInfo, zeroInfo,
				zeroInfo, zeroInfo, zeroInfo, zeroInfo, zeroInfo,
				zeroInfo, zeroInfo, zeroInfo, zeroInfo, target,
			},
		},
		{
			before: []*nodeInfo{
				zeroInfo, zeroInfo, zeroInfo, zeroInfo, zeroInfo,
				zeroInfo, zeroInfo, zeroInfo, zeroInfo, zeroInfo,
				zeroInfo, zeroInfo, zeroInfo, zeroInfo, zeroInfo,
				zeroInfo, zeroInfo, zeroInfo, zeroInfo, zeroInfo,
			},
			after: []*nodeInfo{
				zeroInfo, zeroInfo, zeroInfo, zeroInfo, zeroInfo,
				zeroInfo, zeroInfo, zeroInfo, zeroInfo, zeroInfo,
				zeroInfo, zeroInfo, zeroInfo, zeroInfo, zeroInfo,
				zeroInfo, zeroInfo, zeroInfo, zeroInfo, target,
			},
		},
	} {
		c := cc
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {

			testServers[0].node.routingTable[msb].bucket = c.before
			testServers[0].node.updateRoutingTable(target)
			assert.Equal(t, len(c.after), len(testServers[0].node.routingTable[msb].bucket))

			for i := range c.after {
				assert.Equal(t, c.after[i].nAddr, testServers[0].node.routingTable[msb].bucket[i].nAddr)
				assert.Equal(t, c.after[i].dAddr, testServers[0].node.routingTable[msb].bucket[i].dAddr)
			}
		})
	}
}

func TestNodeStoreIndex(t *testing.T) {
	target := testServers[1]
	from := testServers[0]

	for i, _req := range []*doogle.StoreItemRequest{
		{
			Certificate: from.node.certificate,
			Index:       string([]byte{1}),
			Url:         string([]byte{10}),
			Title:       "title10",
			Description: "description10",
		},
		{
			Certificate: from.node.certificate,
			Index:       string([]byte{1}),
			Url:         string([]byte{11}),
			Title:       "title11",
			Description: "description11",
		},
		{
			Certificate: from.node.certificate,
			Index:       string([]byte{1}),
			Url:         string([]byte{12}),
			Title:       "title12",
			Description: "description12",
		},
		{
			Certificate: from.node.certificate,
			Index:       string([]byte{2}),
			Url:         string([]byte{20}),
			Title:       "title20",
			Description: "description1",
		},
		{
			Certificate: from.node.certificate,
			Index:       string([]byte{2}),
			Url:         string([]byte{20}),
			Title:       "title20",
			Description: "description1",
		},
		{
			Certificate: from.node.certificate,
			Index:       string([]byte{3}),
			Url:         string([]byte{30}),
			Title:       "title30",
			Description: "description1",
		},
	} {
		req := _req
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			_, err := target.node.StoreItem(context.Background(), req)
			assert.Equal(t, nil, err)

			// calc item's address
			h := sha1.Sum([]byte(req.Url))
			itemAddr := doogleAddressStr(h[:])

			// calc index's address
			h = sha1.Sum([]byte(req.Index))
			idxAddr := doogleAddressStr(h[:])

			// get dhtValue
			_dht, ok := target.node.dht.Load(idxAddr)
			assert.Equal(t, true, ok)
			_, ok = _dht.(*dhtValue)
			assert.Equal(t, true, ok)

			// check itemsMap
			_it, ok := target.node.items.Load(itemAddr)
			assert.Equal(t, true, ok)
			it, ok := _it.(*item)
			assert.Equal(t, true, ok)
			assert.Equal(t, req.Title, it.title)
		})
	}

	for i, cc := range []struct {
		idx    string
		expLen int
	}{
		{string([]byte{1}), 3},
		{string([]byte{2}), 1},
		{string([]byte{3}), 1},
	} {
		c := cc
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			// calc index's address
			h := sha1.Sum([]byte(c.idx))
			idxAddr := doogleAddressStr(h[:])

			_dhtV, ok := target.node.dht.Load(idxAddr)
			assert.Equal(t, true, ok)

			dhtV, ok := _dhtV.(*dhtValue)
			assert.Equal(t, true, ok)
			assert.Equal(t, c.expLen, len(dhtV.itemAddresses))
		})
	}
}
