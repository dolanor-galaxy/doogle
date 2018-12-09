package crawler

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"time"

	"github.com/mathetake/doogle/grpc"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

type mockDoogleClient struct{}

var mClient doogle.DoogleClient = mockDoogleClient{}

func (mockDoogleClient) StoreItem(ctx context.Context, in *doogle.StoreItemRequest, opts ...grpc.CallOption) (*doogle.Empty, error) {
	return nil, nil
}

func (mockDoogleClient) FindIndex(ctx context.Context, in *doogle.FindIndexRequest, opts ...grpc.CallOption) (*doogle.FindIndexReply, error) {
	return nil, nil
}

func (mockDoogleClient) FindNode(ctx context.Context, in *doogle.FindNodeRequest, opts ...grpc.CallOption) (*doogle.NodeInfos, error) {
	return nil, nil
}

func (mockDoogleClient) PingWithCertificate(ctx context.Context, in *doogle.NodeCertificate, opts ...grpc.CallOption) (*doogle.NodeCertificate, error) {
	return nil, nil
}

func (mockDoogleClient) Ping(ctx context.Context, in *doogle.StringMessage, opts ...grpc.CallOption) (*doogle.StringMessage, error) {
	return nil, nil
}

func (mockDoogleClient) PingTo(ctx context.Context, in *doogle.NodeInfo, opts ...grpc.CallOption) (*doogle.StringMessage, error) {
	return nil, nil
}

func (mockDoogleClient) GetIndex(ctx context.Context, in *doogle.StringMessage, opts ...grpc.CallOption) (*doogle.GetIndexReply, error) {
	return nil, nil
}

func (mockDoogleClient) PostUrl(ctx context.Context, in *doogle.StringMessage, opts ...grpc.CallOption) (*doogle.StringMessage, error) {
	return nil, nil
}

func TestDoogleCrawler_worker(t *testing.T) {
	logger := logrus.New()
	crawler, _ := NewCrawler(1, 4, logger)
	cr := crawler.(*doogleCrawler)
	cr.SetDoogleClient(mClient)

	for _, url := range []string{
		"https://www.google.com",
		"https://en.wikipedia.org/wiki/Kademlia",
		"https://en.wikipedia.org/wiki/Distributed_hash_table",
		"https://en.wikipedia.org/wiki/japan",
		"https://en.wikipedia.org/wiki/golang",
	} {
		cr.queue <- url
	}

	time.Sleep(1 * time.Second)
}

func TestDoogleCrawler_Crawl(t *testing.T) {
	logger := logrus.New()
	crawler, _ := NewCrawler(4, 4, logger)
	cr := crawler.(*doogleCrawler)
	cr.SetDoogleClient(mClient)

	cr.Crawl([]string{
		"https://www.google.com",
		"https://en.wikipedia.org/wiki/Kademlia",
		"https://en.wikipedia.org/wiki/Distributed_hash_table",
		"https://en.wikipedia.org/wiki/japan",
	})

	time.Sleep(1 * time.Second)
}

func TestDoogleCrawler_analyze(t *testing.T) {
	crawler, _ := NewCrawler(0, 0, nil)
	cr := crawler.(*doogleCrawler)

	for i, cc := range []struct {
		target    string
		expTitle  string
		expEdges  []string
		expTokens []string
	}{
		{
			target: `
<!DOCTYPE html><html>
	<header>
		<title>title1</title>
	</header>
	<body>
		<a href="https://www.google.com">
	</body>
</html>`,
			expTitle:  "title1",
			expEdges:  []string{"https://www.google.com"},
			expTokens: []string{"title1"},
		},
		{
			target: `
<!DOCTYPE html><html>
	<header>
		<title>This is a pen</title>
	</header>
	<body>
		<a href="https://www.google.com"> 123456 </a>
		<a href="https://www.doogle.com"> 123456 </a>
	</body>
</html>`,
			expTitle:  "This is a pen",
			expEdges:  []string{"https://www.google.com", "https://www.doogle.com"},
			expTokens: []string{"this", "is", "a", "pen", "123456", "123456"},
		},
		{
			target: `
<!DOCTYPE html><html>
	<header>
		<title>This is a pen 100yen</title>
	</header>
	<body>
		<a href="https://www.google.com"> 123456 </a>
		<a href="https://www.doogle.com"> 123456 </a>
		<p> this is first text field</p>
	</body>
</html>`,
			expTitle:  "This is a pen 100yen",
			expEdges:  []string{"https://www.google.com", "https://www.doogle.com"},
			expTokens: []string{"this", "is", "a", "pen", "100yen", "123456", "123456", "this", "is", "first", "text", "field"},
		},
		{
			target: `
<!DOCTYPE html><html>
	<header>
		<title>This is a pen 100yen</title>
	</header>
	<body>
		<a href="https://www.google.com"> 123456 </a>
		<a href="htt://www.doogle.com"> 123456 </a>
		<a href="/img/cat.jpg"></a>
		<p> this is first text field</p>
	</body>
</html>`,
			expTitle:  "This is a pen 100yen",
			expEdges:  []string{"https://www.google.com"},
			expTokens: []string{"this", "is", "a", "pen", "100yen", "123456", "123456", "this", "is", "first", "text", "field"},
		},
	} {
		c := cc
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			body := strings.NewReader(c.target)
			aTitle, aTokens, aEdgeURLs, err := cr.analyze(body, "")
			if err != nil {
				panic(err)
			}
			assert.Equal(t, c.expTitle, aTitle)
			assert.Equal(t, len(c.expEdges), len(aEdgeURLs))

			for i := range c.expEdges {
				assert.Equal(t, c.expEdges[i], aEdgeURLs[i])
			}

			assert.Equal(t, len(c.expTokens), len(aTokens))

			for i := range c.expTokens {
				assert.Equal(t, c.expTokens[i], aTokens[i])
			}
		})
	}
}
