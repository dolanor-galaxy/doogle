package node

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"testing"
	"time"

	pb "github.com/mathetake/doogle/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"gotest.tools/assert"
)

const (
	localhost = "localhost"
	port1     = ":3841"
	port2     = ":3842"
	port3     = ":3833"
	port4     = ":3834"
	port5     = ":3835"
	port6     = ":3836"
)

var testServerParams = []struct {
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

var testServers = make([]*Node, 0, 100)

func TestMain(m *testing.M) {
	for _, ts := range testServerParams {
		runServer(ts.port, ts.difficulty)
	}
	os.Exit(m.Run())
}

// set up doogle server on specified port
func runServer(port string, difficulty int) {
	node, err := NewNode(difficulty, localhost, port)
	if err != nil {
		log.Fatalf("failed to craete new node: %v", err)
	}

	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterDoogleServer(s, node)
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	time.Sleep(100 * time.Millisecond)
	testServers = append(testServers, node)
}

func TestPing(t *testing.T) {
	for i, cc := range testServerParams {
		c := cc
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			conn, err := grpc.Dial(localhost+c.port, grpc.WithInsecure())
			if err != nil {
				log.Fatalf("did not connect: %v", err)
			}
			defer conn.Close()
			client := pb.NewDoogleClient(conn)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			r, err := client.Ping(ctx, &pb.NodeCertificate{})
			assert.Equal(t, nil, err)
			assert.Equal(t, "Pong", r.Message)

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
			client := pb.NewDoogleClient(conn)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			_, err = client.PingTo(ctx, &pb.NodeInfo{Host: localhost, Port: c.toPort[1:]})
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
	} {
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			c := cc
			p := peer.Peer{Addr: testAddr(c.addr), AuthInfo: nil}
			ctx := context.Background()
			ctx = peer.NewContext(ctx, &p)

			node, err := NewNode(0, "bar", "foo")
			if err != nil {
				t.Fatalf("failed to create new node: %v", err)
			}
			actual := node.isValidSender(ctx, c.rawAddr, c.pk, c.nonce, c.difficulty)
			assert.Equal(t, c.exp, actual)
		})
	}
}

func TestUpdateRoutingTable(t *testing.T) {}
