package node

import (
	"context"
	"crypto/sha1"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mathetake/doogle/crawler"
	"github.com/mathetake/doogle/grpc"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"gotest.tools/assert"
)

const (
	localhost = "127.0.0.1"
	numServer = 10
)

var (
	zeroAddress = doogleAddress{
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	}
	zeroInfo    = &nodeInfo{zeroAddress, "", 0}
	testServers []*struct {
		port string
		node *Node
	}
)

type mockCrawler struct {
	title, description string
	tokens, edgeURLs   []string
}

func (c *mockCrawler) AnalyzeURL(url string) (title, description string, tokens, edgeURLs []string, err error) {
	return c.title, c.description, c.tokens, c.edgeURLs, nil
}

var _ crawler.Crawler = &mockCrawler{}

func TestMain(m *testing.M) {
	logger := logrus.New()
	logger.SetLevel(1)

	for i := 0; i < numServer; i++ {
		srv := &struct {
			port string
			node *Node
		}{}
		srv.node, srv.port = runServer(1, logger)
		testServers = append(testServers, srv)
	}
	os.Exit(m.Run())
}

// set up doogle server on specified port
func runServer(difficulty int, logger *logrus.Logger) (*Node, string) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	port := ":" + strings.Split(lis.Addr().String(), ":")[1]
	node, err := NewNode(difficulty, localhost+port, logger, nil)
	if err != nil {
		log.Fatalf("failed to craete new node: %v", err)
	}

	s := grpc.NewServer()
	doogle.RegisterDoogleServer(s, node)
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	time.Sleep(100 * time.Millisecond)
	return node, port
}

func resetRoutingTable() {
	for i := range testServers {
		rt := map[int]*routingBucket{}
		for i := 0; i < addressBits; i++ {
			b := make([]*nodeInfo, 0, bucketSize)
			rt[i] = &routingBucket{bucket: b, mux: sync.Mutex{}}
		}
		testServers[i].node.routingTable = rt
	}
}

func resetDHT() {
	for i := range testServers {
		// reset routing table on testServers[0]
		testServers[i].node.dht = sync.Map{}
		testServers[i].node.items = sync.Map{}
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

func TestNode_PingWithCertificate(t *testing.T) {
	resetRoutingTable()
	defer resetRoutingTable()

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

func TestNode_Ping(t *testing.T) {
	defer resetRoutingTable()

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

func TestNode_PingTo(t *testing.T) {
	defer resetRoutingTable()

	for i, cc := range []struct {
		fromPort   string
		toPort     string
		isErrorNil bool
	}{
		{testServers[0].port, testServers[1].port, true},
		{testServers[1].port, testServers[2].port, true},
		{testServers[2].port, testServers[4].port, true},
		{testServers[3].port, ":80", false},
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

func TestNode_IsValidSender(t *testing.T) {
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

func TestNode_UpdateRoutingTable(t *testing.T) {
	// reset routing table
	resetRoutingTable()
	defer resetRoutingTable()

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

func TestNode_StoreItem(t *testing.T) {
	resetDHT()
	defer resetDHT()

	target := testServers[1]
	from := testServers[0]

	for i, cc := range []*doogle.StoreItemRequest{
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
		c := cc
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			_, err := target.node.StoreItem(context.Background(), c)
			assert.Equal(t, nil, err)

			// calc item's address
			h := sha1.Sum([]byte(c.Url))
			itemAddr := doogleAddressStr(h[:])

			// calc index's address
			h = sha1.Sum([]byte(c.Index))
			idxAddr := doogleAddressStr(h[:])

			// get dhtValue
			raw, ok := target.node.dht.Load(idxAddr)
			assert.Equal(t, true, ok)
			_, ok = raw.(*dhtValue)
			assert.Equal(t, true, ok)

			// check itemsMap
			raw, ok = target.node.items.Load(itemAddr)
			assert.Equal(t, true, ok)
			it, ok := raw.(*item)
			assert.Equal(t, true, ok)
			assert.Equal(t, c.Title, it.title)
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

			raw, ok := target.node.dht.Load(idxAddr)
			assert.Equal(t, true, ok)

			dhtV, ok := raw.(*dhtValue)
			assert.Equal(t, true, ok)
			assert.Equal(t, c.expLen, len(dhtV.itemAddresses))
		})
	}
}

func TestNode_findNearestNode(t *testing.T) {
	var mux sync.Mutex
	srv := testServers[1].node
	for i, cc := range []struct {
		targetAddr []byte
		bitOffset  int
		before     []*nodeInfo
		expected   []string
	}{
		{
			targetAddr: []byte{1},
			before: []*nodeInfo{
				{
					nAddr: string([]byte{2}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
				},
				{
					nAddr: string([]byte{3}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3},
				},
				{
					nAddr: string([]byte{4}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4},
				},
			},
			expected: []string{string([]byte{2}), string([]byte{3}), string([]byte{4})},
		},
		{
			targetAddr: []byte{1},
			before: []*nodeInfo{
				{
					nAddr: string([]byte{2}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4},
				},
				{
					nAddr: string([]byte{3}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
				},
				{
					nAddr: string([]byte{4}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3},
				},
			},
			expected: []string{string([]byte{3}), string([]byte{4}), string([]byte{2})},
		},
		{
			targetAddr: []byte{1},
			before: []*nodeInfo{
				{
					nAddr: string([]byte{2}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4},
				},
				{
					nAddr: string([]byte{3}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
				},
				{
					nAddr: string([]byte{4}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3},
				},
			},
			expected:  []string{string([]byte{3}), string([]byte{4}), string([]byte{2})},
			bitOffset: -2,
		},
		{
			targetAddr: []byte{1},
			before: []*nodeInfo{
				{
					nAddr: string([]byte{2}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
				},
				{
					nAddr: string([]byte{3}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3},
				},
			},
			expected: []string{string([]byte{2}), string([]byte{3})},
		},
		{
			targetAddr: []byte{1},
			before:     []*nodeInfo{},
			expected:   []string{},
		},
		{
			targetAddr: []byte{1},
			before: []*nodeInfo{
				{
					nAddr: string([]byte{2}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0},
				},
				{
					nAddr: string([]byte{3}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4},
				},
				{
					nAddr: string([]byte{4}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0},
				},
				{
					nAddr: string([]byte{5}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
				},
				{
					nAddr: string([]byte{6}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3},
				},
			},
			expected:  []string{string([]byte{5}), string([]byte{6}), string([]byte{3})},
			bitOffset: -10,
		},
	} {
		c := cc
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			mux.Lock()
			defer mux.Unlock()
			var dAddr doogleAddress
			copy(dAddr[:], c.targetAddr)
			msb := getMostSignificantBit(srv.DAddr.xor(dAddr)) + c.bitOffset

			srv.routingTable[msb].bucket = c.before
			ret, err := srv.findNearestNode(dAddr, msb, 0)

			assert.Equal(t, nil, err)
			assert.Equal(t, len(c.expected), len(ret))

			for i, actual := range ret {
				assert.Equal(t, c.expected[i], actual.NetworkAddress)
			}
			resetRoutingTable()
		})
	}
}

func TestNode_findNode(t *testing.T) {
	resetRoutingTable()
	defer resetRoutingTable()

	srv := testServers[1].node
	for i, cc := range []struct {
		targetAddr []byte
		before     []*nodeInfo
		expected   []string
	}{
		{
			targetAddr: []byte{1},
			before: []*nodeInfo{
				{
					nAddr: string([]byte{2}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
				},
				{
					nAddr: string([]byte{3}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3},
				},
				{
					nAddr: string([]byte{4}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4},
				},
			},
			expected: []string{string([]byte{2}), string([]byte{3}), string([]byte{4})},
		},
		{
			targetAddr: []byte{1},
			before: []*nodeInfo{
				{
					nAddr: string([]byte{2}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4},
				},
				{
					nAddr: string([]byte{3}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
				},
				{
					nAddr: string([]byte{4}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3},
				},
			},
			expected: []string{string([]byte{3}), string([]byte{4}), string([]byte{2})},
		},
		{
			targetAddr: []byte{1},
			before: []*nodeInfo{
				{
					nAddr: string([]byte{2}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
				},
				{
					nAddr: string([]byte{3}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3},
				},
			},
			expected: []string{string([]byte{2}), string([]byte{3})},
		},
		{
			targetAddr: []byte{1},
			before:     []*nodeInfo{},
			expected:   []string{},
		},
		{
			targetAddr: []byte{1},
			before: []*nodeInfo{
				{
					nAddr: string([]byte{2}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0},
				},
				{
					nAddr: string([]byte{3}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4},
				},
				{
					nAddr: string([]byte{4}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0},
				},
				{
					nAddr: string([]byte{5}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
				},
				{
					nAddr: string([]byte{6}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3},
				},
			},
			expected: []string{string([]byte{5}), string([]byte{6}), string([]byte{3})},
		},
	} {
		c := cc
		_ = t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			var dAddr doogleAddress
			copy(dAddr[:], c.targetAddr)
			msb := getMostSignificantBit(srv.DAddr.xor(dAddr))

			srv.routingTable[msb].bucket = c.before
			ret, err := srv.findNode(dAddr)

			assert.Equal(t, nil, err)
			assert.Equal(t, len(c.expected), len(ret))

			for i, actual := range ret {
				assert.Equal(t, c.expected[i], actual.NetworkAddress)
			}
		})
	}
}

func TestNode_PostUrl(t *testing.T) {
	resetRoutingTable()
	defer resetRoutingTable()
	resetDHT()
	defer resetDHT()

	srv := testServers[0].node
	for i, cc := range []struct {
		url string
		cr  *mockCrawler
	}{
		{
			url: "url1",
			cr: &mockCrawler{
				title:       "title1",
				description: "description",
				edgeURLs:    []string{},
				tokens:      []string{string([]byte{1}), string([]byte{2})},
			},
		},
		{
			url: "url1",
			cr: &mockCrawler{
				title:       "title1",
				description: "description",
				edgeURLs:    []string{"foo.com", "bar.com"},
				tokens:      []string{"token1", "token2"},
			},
		},
	} {
		c := cc
		_ = t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			srv.crawler = c.cr
			_, err := srv.PostUrl(
				context.Background(),
				&doogle.StringMessage{Message: c.url},
			)
			assert.Equal(t, nil, err)

			for _, token := range c.cr.tokens {
				h := sha1.Sum([]byte(c.url))
				itemAddrStr := doogleAddressStr(h[:])

				h = sha1.Sum([]byte(token))
				tokenAddrStr := doogleAddressStr(h[:])
				raw, ok := srv.dht.Load(tokenAddrStr)
				assert.Equal(t, true, ok)

				dhtV, ok := raw.(*dhtValue)
				assert.Equal(t, true, ok)

				dhtV.mux.Lock()
				var isIncluded = false
				var ia doogleAddressStr
				for _, addr := range dhtV.itemAddresses {
					if addr == itemAddrStr {
						isIncluded = true
						ia = addr
					}
				}
				assert.Equal(t, true, isIncluded)

				raw, ok = srv.items.Load(ia)
				assert.Equal(t, true, ok)

				it, ok := raw.(*item)
				assert.Equal(t, true, ok)
				assert.Equal(t, c.cr.title, it.title)
				assert.Equal(t, c.cr.description, it.description)
				assert.Equal(t, len(c.cr.edgeURLs), len(it.edges))

				for i, eu := range c.cr.edgeURLs {
					h = sha1.Sum([]byte(eu))
					expAddr := doogleAddress(h)
					assert.Equal(t, expAddr, it.edges[i])
				}

				dhtV.mux.Unlock()
			}
		})
	}
}

func TestNode_findIndex_notFound(t *testing.T) {
	resetRoutingTable()
	defer resetRoutingTable()

	srv := testServers[1].node
	for i, cc := range []struct {
		targetAddr []byte
		before     []*nodeInfo
		expected   []string
	}{
		{
			targetAddr: []byte{1},
			before: []*nodeInfo{
				{
					nAddr: string([]byte{2}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
				},
				{
					nAddr: string([]byte{3}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3},
				},
				{
					nAddr: string([]byte{4}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4},
				},
			},
			expected: []string{string([]byte{2}), string([]byte{3}), string([]byte{4})},
		},
		{
			targetAddr: []byte{1},
			before: []*nodeInfo{
				{
					nAddr: string([]byte{2}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4},
				},
				{
					nAddr: string([]byte{3}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
				},
				{
					nAddr: string([]byte{4}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3},
				},
			},
			expected: []string{string([]byte{3}), string([]byte{4}), string([]byte{2})},
		},
		{
			targetAddr: []byte{1},
			before: []*nodeInfo{
				{
					nAddr: string([]byte{2}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
				},
				{
					nAddr: string([]byte{3}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3},
				},
			},
			expected: []string{string([]byte{2}), string([]byte{3})},
		},
		{
			targetAddr: []byte{1},
			before:     []*nodeInfo{},
			expected:   []string{},
		},
		{
			targetAddr: []byte{1},
			before: []*nodeInfo{
				{
					nAddr: string([]byte{2}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0},
				},
				{
					nAddr: string([]byte{3}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4},
				},
				{
					nAddr: string([]byte{4}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0},
				},
				{
					nAddr: string([]byte{5}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
				},
				{
					nAddr: string([]byte{6}),
					dAddr: doogleAddress{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3},
				},
			},
			expected: []string{string([]byte{5}), string([]byte{6}), string([]byte{3})},
		},
	} {
		c := cc
		_ = t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			var dAddr doogleAddress
			copy(dAddr[:], c.targetAddr)
			msb := getMostSignificantBit(srv.DAddr.xor(dAddr))

			srv.routingTable[msb].bucket = c.before
			raw, err := srv.findIndex(context.Background(), doogleAddressStr(c.targetAddr))
			assert.Equal(t, nil, err)

			ret, ok := raw.Result.(*doogle.FindIndexReply_NodeInfos)
			assert.Equal(t, true, ok)

			assert.Equal(t, len(c.expected), len(ret.NodeInfos.Infos))

			for i, actual := range ret.NodeInfos.Infos {
				assert.Equal(t, c.expected[i], actual.NetworkAddress)
			}
		})
	}
}

func TestNode_findIndex_Found(t *testing.T) {
	resetRoutingTable()
	defer resetRoutingTable()
	resetDHT()
	defer resetDHT()

	srv := testServers[0].node

	for i, cc := range []struct {
		dAddrStr doogleAddressStr
		expItems []*item
	}{
		{
			dAddrStr: doogleAddressStr(string([]byte{0, 0, 1})),
			expItems: []*item{
				{url: "url1", dAddrStr: "address1", localRank: 0.1},
				{url: "url2", dAddrStr: "address2", localRank: 0.2},
				{url: "url3", dAddrStr: "address3", localRank: 0.3},
			},
		},
		{
			dAddrStr: doogleAddressStr(string([]byte{0, 1, 0})),
			expItems: []*item{
				{url: "url1", dAddrStr: "address1", localRank: 0.1},
				{url: "url2", dAddrStr: "address2", localRank: 0.2},
				{url: "url3", dAddrStr: "address3", localRank: 0.3},
			},
		},
		{
			dAddrStr: doogleAddressStr(string([]byte{1, 1, 0})),
			expItems: []*item{},
		},
	} {
		c := cc
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			dhtV := &dhtValue{
				mux:           sync.Mutex{},
				itemAddresses: []doogleAddressStr{},
			}

			for _, it := range c.expItems {
				dhtV.itemAddresses = append(dhtV.itemAddresses, it.dAddrStr)
				srv.items.Store(it.dAddrStr, it)
			}

			srv.dht.Store(c.dAddrStr, dhtV)

			raw, err := srv.findIndex(context.Background(), c.dAddrStr)
			assert.Equal(t, nil, err)

			ret, ok := raw.Result.(*doogle.FindIndexReply_Items)
			assert.Equal(t, true, ok)
			assert.Equal(t, len(c.expItems), len(ret.Items.Items))

			for i, ai := range ret.Items.Items {
				assert.Equal(t, c.expItems[i].url, ai.Url)
				assert.Equal(t, c.expItems[i].localRank, ai.LocalRank)
			}
		})
	}
}

func TestNode_GetIndex(t *testing.T) {
	resetRoutingTable()
	defer resetRoutingTable()
	resetDHT()
	defer resetDHT()

	srv := testServers[0].node

	for i, cc := range []struct {
		dAddrStr doogleAddressStr
		items    []*item
		expected []string
	}{
		{
			dAddrStr: doogleAddressStr(string([]byte{0, 0, 1})),
			items: []*item{
				{url: "url1", dAddrStr: "address1", localRank: 0.3},
				{url: "url2", dAddrStr: "address2", localRank: 0.2},
				{url: "url3", dAddrStr: "address3", localRank: 0.1},
			},
			expected: []string{"url1", "url2", "url3"},
		},
		{
			dAddrStr: doogleAddressStr(string([]byte{0, 1, 0})),
			items: []*item{
				{url: "url1", dAddrStr: "address1", localRank: 0.1},
				{url: "url2", dAddrStr: "address2", localRank: 0.2},
				{url: "url3", dAddrStr: "address3", localRank: 0.5},
				{url: "url4", dAddrStr: "address4", localRank: 0.01},
			},
			expected: []string{"url3", "url2", "url1", "url4"},
		},
		{
			dAddrStr: doogleAddressStr(string([]byte{1, 1, 0})),
			items:    []*item{},
			expected: []string{},
		},
	} {
		c := cc
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			dhtV := &dhtValue{
				mux:           sync.Mutex{},
				itemAddresses: []doogleAddressStr{},
			}

			for _, it := range c.items {
				dhtV.itemAddresses = append(dhtV.itemAddresses, it.dAddrStr)
				srv.items.Store(it.dAddrStr, it)
			}

			srv.dht.Store(c.dAddrStr, dhtV)

			res, err := srv.GetIndex(
				context.Background(),
				&doogle.StringMessage{
					Message: string(c.dAddrStr),
				})

			assert.Equal(t, nil, err)
			assert.Equal(t, len(c.expected), len(res.Items))

			for i, ai := range res.Items {
				assert.Equal(t, c.expected[i], ai.Url)
			}
		})
	}
}
