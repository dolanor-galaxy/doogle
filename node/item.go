// define item on distributed hash table.
// In our settings, we
package node

type item struct {
	url string
	id  address

	// outgoing hyperlinks
	edges []address

	// localRank represents computed locally PageRank
	localRank float64
}
