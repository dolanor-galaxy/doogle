package node

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/mathetake/doogle/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"gotest.tools/assert"
)

var zeroAddress = doogleAddress{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}

var zerInfo = &nodeInfo{zeroAddress, "", "", 0}

const (
	localhost = "localhost"
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
	node, err := NewNode(difficulty, localhost, port, nil, nil)
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

func TestPopAndAppend(t *testing.T) {
	targetInfo := &nodeInfo{dAddr: testServers[0].node.DAddr}

	for i, cc := range []struct {
		idx    int
		before []*nodeInfo
		after  []*nodeInfo
	}{
		{
			idx:    0,
			before: []*nodeInfo{zerInfo},
			after:  []*nodeInfo{targetInfo},
		},
		{
			idx:    0,
			before: []*nodeInfo{zerInfo, zerInfo, zerInfo},
			after:  []*nodeInfo{zerInfo, zerInfo, targetInfo},
		},
		{
			idx:    0,
			before: []*nodeInfo{targetInfo, zerInfo, zerInfo},
			after:  []*nodeInfo{zerInfo, zerInfo, targetInfo},
		},
		{
			idx:    1,
			before: []*nodeInfo{targetInfo, zerInfo, zerInfo},
			after:  []*nodeInfo{targetInfo, zerInfo, targetInfo},
		},
		{
			idx:    2,
			before: []*nodeInfo{targetInfo, zerInfo, zerInfo},
			after:  []*nodeInfo{targetInfo, zerInfo, targetInfo},
		},
		{
			idx:    2,
			before: []*nodeInfo{targetInfo, zerInfo, zerInfo, targetInfo, zerInfo},
			after:  []*nodeInfo{targetInfo, zerInfo, targetInfo, zerInfo, targetInfo},
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
			r, err := client.PingWithCertificate(ctx, &doogle.NodeCertificate{})
			assert.Equal(t, nil, err)
			assert.Equal(t, "pong", r.Message)

			// TODO: check if the table is updated
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

			// TODO: check if the table is updated
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

			_, err = client.PingTo(ctx, &doogle.NodeInfo{Host: localhost, Port: c.toPort[1:]})
			actual := err == nil
			assert.Equal(t, c.isErrorNil, actual)
			if !actual {
				t.Logf("actual error message: %v", err)
			}
		})
	}
}

type testAddr string

func (ta testAddr) Network() string { return "" }
func (ta testAddr) String() string  { return string(ta) }

var _ net.Addr = testAddr("")

func TestIsValidSender(t *testing.T) {
	for i, cc := range []struct {
		addr       string
		rawAddr    []byte
		pk         []byte
		nonce      []byte
		difficulty int
		exp        bool
	}{
		{"", nil, nil, nil, 10, false},
		{"localhost:1234", []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, nil, nil, 10, false},
		{
			"ab:80",
			[]byte{137, 247, 252, 74, 101, 232, 49, 193, 122, 237, 123, 84, 199, 94, 78, 176, 92, 104, 69, 253},
			[]byte("pk"), []byte{172, 171, 254, 98, 171, 6, 169, 186, 105, 145},
			2,
			true,
		},
		{
			"ab:80",
			[]byte{137, 247, 252, 74, 101, 232, 49, 193, 122, 237, 123, 84, 199, 94, 78, 176, 92, 104, 69, 253},
			[]byte("pk"), []byte{172, 171, 254, 98, 171, 6, 169, 186, 105, 145},
			10,
			false,
		},
		{
			"ab:80",
			[]byte{137, 247, 252, 74, 101, 232, 49, 193, 122, 237, 123, 84, 199, 94, 78, 176, 92, 104, 69, 253},
			[]byte("pk"), []byte{172, 171, 254, 98, 171, 6, 169, 186, 105, 145},
			1,
			false,
		},
	} {
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			c := cc
			p := peer.Peer{Addr: testAddr(c.addr), AuthInfo: nil}
			ctx := context.Background()
			ctx = peer.NewContext(ctx, &p)

			node, err := NewNode(2, "bar", "foo", nil, nil)
			if err != nil {
				t.Fatalf("failed to create new node: %v", err)
			}
			actual := node.isValidSender(ctx, c.rawAddr, c.pk, c.nonce, c.difficulty)
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
		host:  localhost,
		port:  testServers[1].port,
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
			before: []*nodeInfo{zerInfo},
			after:  []*nodeInfo{zerInfo, target},
		},
		{
			before: []*nodeInfo{target, zerInfo},
			after:  []*nodeInfo{zerInfo, target},
		},
		{
			before: []*nodeInfo{zerInfo, zerInfo},
			after:  []*nodeInfo{zerInfo, zerInfo, target},
		},
		{
			before: []*nodeInfo{
				zerInfo, zerInfo, zerInfo, zerInfo, zerInfo,
				zerInfo, zerInfo, target, zerInfo, zerInfo,
				zerInfo, zerInfo, zerInfo, zerInfo, zerInfo,
				zerInfo, zerInfo, zerInfo, zerInfo, zerInfo,
			},
			after: []*nodeInfo{
				zerInfo, zerInfo, zerInfo, zerInfo, zerInfo,
				zerInfo, zerInfo, zerInfo, zerInfo, zerInfo,
				zerInfo, zerInfo, zerInfo, zerInfo, zerInfo,
				zerInfo, zerInfo, zerInfo, zerInfo, target,
			},
		},
		{
			before: []*nodeInfo{
				zerInfo, zerInfo, zerInfo, zerInfo, zerInfo,
				zerInfo, zerInfo, zerInfo, zerInfo, zerInfo,
				zerInfo, zerInfo, zerInfo, zerInfo, zerInfo,
				zerInfo, zerInfo, zerInfo, zerInfo, zerInfo,
			},
			after: []*nodeInfo{
				zerInfo, zerInfo, zerInfo, zerInfo, zerInfo,
				zerInfo, zerInfo, zerInfo, zerInfo, zerInfo,
				zerInfo, zerInfo, zerInfo, zerInfo, zerInfo,
				zerInfo, zerInfo, zerInfo, zerInfo, target,
			},
		},
	} {
		c := cc
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {

			testServers[0].node.routingTable[msb].bucket = c.before
			testServers[0].node.updateRoutingTable(target)
			assert.Equal(t, len(c.after), len(testServers[0].node.routingTable[msb].bucket))

			for i := range c.after {
				assert.Equal(t, c.after[i].port, testServers[0].node.routingTable[msb].bucket[i].port)
				assert.Equal(t, c.after[i].host, testServers[0].node.routingTable[msb].bucket[i].host)
				assert.Equal(t, c.after[i].dAddr, testServers[0].node.routingTable[msb].bucket[i].dAddr)
			}
		})
	}
}
