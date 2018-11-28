package crawler

type Crawler interface{}

type doogleCrawler struct{}

func NewCrawler() (Crawler, error) { return nil, nil }

var _ Crawler = doogleCrawler{}
