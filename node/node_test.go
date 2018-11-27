package node

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"testing"
	"time"

	"gotest.tools/assert"

	pb "github.com/mathetake/doogle/grpc"
	"google.golang.org/grpc"
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

var testServers = []struct {
	port string
	node *Node
}{
	{port1, &Node{}},
	{port2, &Node{}},
	{port3, &Node{}},
	{port4, &Node{}},
	{port5, &Node{}},
	{port6, &Node{}},
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
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	time.Sleep(100 * time.Millisecond)
}

func TestPing(t *testing.T) {
	for i, cc := range testServers {
		c := cc
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			conn, err := grpc.Dial(localhost+c.port, grpc.WithInsecure())
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
		})
	}

}
