package node

import (
	"sync"

	"fmt"
	"time"

	"github.com/pkg/errors"
	"gonum.org/v1/gonum/graph/network"
	"gonum.org/v1/gonum/graph/simple"
)

const (
	worldNodeID int64 = -1
	damp              = 0.80
	tol               = 1e-3
)

var inProcessAddr sync.Map

func (n *Node) StartPageRankComputer(numWorker int) {
	for i := 0; i < numWorker; i++ {
		go func() {
			var workerFmt = fmt.Sprintf("[%d-th pagerankComputer]", i)
			n.logger.Infof("%s started", workerFmt)

			for {
				time.Sleep(1 * time.Second)
				dAddr, _ := <-n.pageRankComputingQueue
				if err := n.computeLocalRank(dAddr); err != nil {
					n.logger.Errorf("%s failed to compute: %v", workerFmt, err)
				}
			}
		}()
	}
}

func (n *Node) computeLocalRank(key doogleAddressStr) error {
	if _, loaded := inProcessAddr.LoadOrStore(key, struct{}{}); loaded {
		return nil
	}

	raw, ok := n.dht.Load(key)
	if !ok {
		return errors.Errorf("not found")
	}

	dhtV, ok := raw.(*dhtValue)
	if !ok {
		return errors.Errorf("type assertion failed")
	}

	dhtV.mux.Lock()

	if len(dhtV.itemAddresses) < 2 {
		return nil
	}

	g := simple.NewDirectedGraph()
	// add world node
	g.AddNode(simple.Node(worldNodeID))

	addrToID := make(map[doogleAddressStr]int64, len(dhtV.itemAddresses))
	idToAddr := make(map[int64]doogleAddressStr, len(dhtV.itemAddresses))
	for id, addr := range dhtV.itemAddresses {
		id64 := int64(id + 1)
		addrToID[addr] = id64
		idToAddr[id64] = addr
		g.AddNode(simple.Node(id))
	}

	dhtV.mux.Unlock()

	for addr, fromID := range addrToID {
		raw, ok := n.items.Load(addr)
		if !ok {
			n.logger.Errorf("failed to get the address of item %s", addr)
			continue
		}

		it, ok := raw.(*item)
		if !ok {
			n.logger.Errorf("type assertion failed.")
			continue
		}

		for _, edge := range it.edges {
			if toID, ok := addrToID[edge]; ok && g.Has(toID) {
				g.SetEdge(simple.Edge{F: simple.Node(fromID), T: simple.Node(toID)})
			} else {
				// set edge to world node
				g.SetEdge(simple.Edge{F: simple.Node(fromID), T: simple.Node(worldNodeID)})
				g.SetEdge(simple.Edge{F: simple.Node(worldNodeID), T: simple.Node(fromID)})
			}
		}
	}

	ranks := network.PageRank(g, damp, tol)

	for addr, id := range addrToID {
		raw, ok := n.items.Load(addr)
		if !ok {
			n.logger.Errorf("failed to get the address of item %s", addr)
			continue
		}

		it, ok := raw.(*item)
		if !ok {
			n.logger.Errorf("type assertion failed.")
			continue
		}

		rank, _ := ranks[id]

		it.mux.Lock()

		// update PageRank
		it.rankComputedCount++
		it.localRank += (rank - it.localRank) / it.rankComputedCount
		it.mux.Unlock()
	}

	inProcessAddr.Delete(key)
	return nil
}
