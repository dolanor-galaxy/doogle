// define item on distributed hash table.
// In our settings, we
package node

type item struct {
	url   string
	dAddr doogleAddress

	// outgoing hyperlinks
	edges []doogleAddress

	// localRank represents computed locally PageRank
	localRank float64
}
