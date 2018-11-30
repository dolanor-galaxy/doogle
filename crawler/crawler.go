package crawler

type Crawler interface {
	AnalyzeURL(url string) (title, description string, tokens, edgeURLs []string, err error)
}

type doogleCrawler struct{}

func NewCrawler() (Crawler, error) { return nil, nil }

func (c *doogleCrawler) AnalyzeURL(url string) (string, string, []string, []string, error) {
	return "", "", nil, nil, nil
}

var _ Crawler = &doogleCrawler{}
