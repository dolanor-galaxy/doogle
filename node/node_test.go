package node

import (
	"context"
	"log"
	"net"
	"os"
	"testing"
	"time"

	"gotest.tools/assert"

	"google.golang.org/grpc/reflection"

	pb "github.com/mathetake/doogle/grpc"
	"google.golang.org/grpc"
)

const (
	localhost = "localhost"
	port1     = ":3843"
	port2     = ":3844"
	port3     = ":3839"
)

var testServers = []struct {
	port string
	node *Node
}{
	{port1, &Node{}},
	{port2, &Node{}},
	{port3, &Node{}},
}

func TestMain(m *testing.M) {
	for _, ts := range testServers {
		runServer(ts.port, ts.node)
	}
	os.Exit(m.Run())
}

// set up doogle server on specified port
func runServer(port string, node *Node) {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterDoogleServer(s, node)
	go func() {
		reflection.Register(s)
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	time.Sleep(100 * time.Millisecond)
}

func TestPing(t *testing.T) {
	conn, err := grpc.Dial(localhost+port1, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewDoogleClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, err := c.Ping(ctx, &pb.NodeCertificate{})
	assert.Equal(t, nil, err)
	assert.Equal(t, "Pong", r.Message)
}
