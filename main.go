package main

import (
	"encoding/hex"
	"flag"
	"net"

	"github.com/mathetake/doogle/crawler"

	"github.com/mathetake/doogle/grpc"
	"github.com/mathetake/doogle/node"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	// parameters
	port       string
	difficulty int
)

func main() {
	// initialize logger
	logger := logrus.New()

	// parse params
	flag.StringVar(&port, "p", "", "port for node")
	flag.IntVar(&difficulty, "d", 0, "difficulty for cryptographic puzzle")
	flag.Parse()

	// listen port
	lis, err := net.Listen("tcp", port)
	if err != nil {
		logger.Fatalf("failed to listen: %v", err)
	}

	// create crawler
	cr, err := crawler.NewCrawler()
	if err != nil {
		logger.Fatalf("failed to initialize crawler: %v", err)
	}

	// create new node
	srv, err := node.NewNode(difficulty, lis.Addr().String(), logger, cr)
	if err != nil {
		logger.Fatalf("failed to create node: %v", err)
	}

	logger.Infof("node created: doogleAddress=%v\n", hex.EncodeToString(srv.DAddr[:]))

	err = cr.StartCrawl()
	if err != nil {
		logger.Fatal("failed to start crawler")
	}

	// register node
	s := grpc.NewServer(grpc.UnaryInterceptor(doogle.UnaryServerInterceptor(logger)))
	doogle.RegisterDoogleServer(s, srv)
	reflection.Register(s)
	logger.Infof("node listen on port: %s \n", port)
	if err := s.Serve(lis); err != nil {
		logger.Fatalf("failed to serve: %v", err)
	}
}
