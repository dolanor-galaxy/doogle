package main

import (
	"encoding/hex"
	"flag"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mathetake/doogle/crawler"
	"github.com/mathetake/doogle/grpc"
	"github.com/mathetake/doogle/node"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	port       string
	difficulty int
	queueCap   int
	numWorker  int
)

func main() {
	// initialize logger
	logger := logrus.New()

	// parse params
	flag.StringVar(&port, "p", "", "port for node")
	flag.IntVar(&difficulty, "d", 0, "difficulty for cryptographic puzzle")
	flag.IntVar(&queueCap, "c", 0, "crawler's channel capacity")
	flag.IntVar(&numWorker, "w", 0, "number of crawler's worker")
	flag.Parse()

	// listen port
	lis, err := net.Listen("tcp", port)
	if err != nil {
		logger.Fatalf("failed to listen: %v", err)
	}

	// create crawler
	cr, err := crawler.NewCrawler(queueCap, numWorker, logger)
	if err != nil {
		logger.Fatalf("failed to initialize crawler: %v", err)
	}

	// create new node
	srv, err := node.NewNode(difficulty, lis.Addr().String(), logger, cr, queueCap)
	if err != nil {
		logger.Fatalf("failed to create node: %v", err)
	}

	defer srv.CloseConnections()

	logger.Infof("node created: doogleAddress=%v\n", hex.EncodeToString(srv.DAddr[:]))

	// register node
	s := grpc.NewServer()
	// s := grpc.NewServer(grpc.UnaryInterceptor(doogle.UnaryServerInterceptor(logger)))
	doogle.RegisterDoogleServer(s, srv)
	reflection.Register(s)

	go func() {
		logger.Infof("node listen on port: %s, num of crawler's worker: %d \n", port, numWorker)
		logger.Infof("difficulty: %d, crawler's queue capacity: %d \n", difficulty, queueCap)
		if err := s.Serve(lis); err != nil {
			logger.Fatalf("failed to serve: %v", err)
		}
	}()

	srv.StartPageRankComputer(numWorker)

	// make gRPC connection to doogle node for crawler service
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	for err != nil {
		logger.Info("wait until the server starts listening...")
		time.Sleep(5 * time.Second)
		conn, err = grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	}

	defer conn.Close()

	// set doogleClient on crawler
	cr.SetDoogleClient(doogle.NewDoogleClient(conn))

	logger.Println("crawler is ready")

	gracefulStop := make(chan os.Signal, 1)
	signal.Notify(gracefulStop, syscall.SIGTERM, syscall.SIGINT, syscall.SIGUSR2)
	<-gracefulStop

	// graceful shutdown
	s.GracefulStop()
}
