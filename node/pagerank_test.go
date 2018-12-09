package node

import (
	"fmt"
	"testing"

	"sync"

	"gotest.tools/assert"
)

func TestNode_computeLocalRank(t *testing.T) {
	node, err := NewNode(1, "doogle", logger, nil, 10)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	cases := []struct {
		key     doogleAddressStr
		dhtV    *dhtValue
		items   map[doogleAddressStr]*item
		highest doogleAddressStr
	}{
		{
			key: doogleAddressStr([]byte{1, 0, 0}),
			dhtV: &dhtValue{
				itemAddresses: []doogleAddressStr{
					"1", "2", "3",
				},
				mux: sync.Mutex{},
			},
			items: map[doogleAddressStr]*item{
				"1": {
					dAddrStr:  "1",
					localRank: 0,
					edges:     []doogleAddressStr{"3"},
				},
				"2": {
					dAddrStr:  "2",
					localRank: 0,
					edges:     []doogleAddressStr{"3"},
				},
				"3": {
					dAddrStr:  "3",
					localRank: 0,
					edges:     []doogleAddressStr{"1"},
				},
			},
		},
		{
			key: doogleAddressStr([]byte{1, 0, 0}),
			dhtV: &dhtValue{
				itemAddresses: []doogleAddressStr{
					"1", "2", "3",
				},
				mux: sync.Mutex{},
			},
			items: map[doogleAddressStr]*item{
				"1": {
					dAddrStr:          "1",
					localRank:         0.4,
					rankComputedCount: 1,
					edges:             []doogleAddressStr{"3"},
				},
				"2": {
					dAddrStr:          "2",
					localRank:         0.1,
					rankComputedCount: 2,
					edges:             []doogleAddressStr{"3"},
				},
				"3": {
					dAddrStr:          "3",
					localRank:         0.001,
					rankComputedCount: 3,
					edges:             []doogleAddressStr{"1"},
				},
			},
		},
	}

	for i, cc := range cases {
		c := cc
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			node.dht.Store(c.key, c.dhtV)

			for _, addr := range c.dhtV.itemAddresses {
				it, ok := c.items[addr]
				if !ok {
					t.Fatal("failed to get *item")
				}
				node.items.Store(addr, it)
			}

			err := node.computeLocalRank(c.key)
			assert.Equal(t, true, err == nil)
		})
	}
}
