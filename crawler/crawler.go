package crawler

import (
	"io"
	"net/http"
	"regexp"
	"strings"

	"context"
	"time"

	"fmt"

	"github.com/mathetake/doogle/grpc"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type Crawler interface {
	AnalyzePage(url string) (title string, tokens, edgeURLs []string, err error)
	Crawl([]string)
	SetDoogleClient(cl doogle.DoogleClient)
}

type doogleCrawler struct {
	tokenRegex *regexp.Regexp
	urlRegex   *regexp.Regexp
	dClient    doogle.DoogleClient
	queue      chan string
	logger     *logrus.Logger
}

var _ Crawler = &doogleCrawler{}

func NewCrawler(queueCap, numWorker int, logger *logrus.Logger) (Crawler, error) {
	tRegex, err := regexp.Compile("([a-z0-9]+)")
	if err != nil {
		return nil, errors.Errorf("failed to compile tokenRegexp: %v", err)
	}

	urlRegex, err := regexp.Compile(`^(http:\/\/www\.|https:\/\/www\.|http:\/\/|https:\/\/)?[a-z0-9]+([\-\.]{1}[a-z0-9]+)*\.[a-z]{2,5}(:[0-9]{1,5})?(\/.*)?$`)
	if err != nil {
		return nil, errors.Errorf("failed to compile tokenRegexp: %v", err)
	}

	crawler := &doogleCrawler{
		tokenRegex: tRegex,
		urlRegex:   urlRegex,
		logger:     logger,
		queue:      make(chan string, queueCap),
	}

	for i := 0; i < numWorker; i++ {
		go crawler.worker(i)
	}

	return crawler, nil
}

func (c *doogleCrawler) SetDoogleClient(cl doogle.DoogleClient) {
	c.dClient = cl
}

func (c *doogleCrawler) Crawl(urls []string) {
	if cap(c.queue) < 1 {
		return
	}

	for _, url := range urls {
		if c.urlRegex.MatchString(url) {
			select {
			case c.queue <- url:
			default:
				c.logger.Info("crawling queue is full")
			}
		}
	}
}

func (c *doogleCrawler) AnalyzePage(url string) (string, []string, []string, error) {
	res, err := http.Get(url)
	if err != nil {
		return "", nil, nil, errors.Errorf("failed to https.Get: %v", err)
	}

	defer res.Body.Close()
	return c.analyze(res.Body)
}

func (c *doogleCrawler) worker(id int) {
	var workerFmt = fmt.Sprintf("[%d-th worker]", id)
	c.logger.Info(workerFmt, " started")

	for {
		time.Sleep(1 * time.Millisecond)

		url, _ := <-c.queue
		c.logger.Infof("%s got url: %s", workerFmt, url)
		_, _, urls, err := c.AnalyzePage(url)
		if err != nil {
			c.logger.Errorf("%s AnalyzePage failed : %v", workerFmt, err)
			continue
		}

		for _, url := range urls {
			_, err := c.dClient.PostUrl(context.Background(), &doogle.StringMessage{
				Message: url,
			})
			if err != nil {
				c.logger.Errorf("%s PostUrl failed : %v", workerFmt, err)
			}
		}
	}
}

func (c *doogleCrawler) analyze(body io.Reader) (string, []string, []string, error) {
	doc := html.NewTokenizer(body)
	var title string
	var tokens []string
	var edgeURLs []string

	selected := map[string]interface{}{}

	for tokenType := doc.Next(); tokenType != html.ErrorToken; {
		token := doc.Token()

		if tokenType == html.TextToken {
			for _, w := range strings.Split(token.Data, " ") {
				_, ok := selected[w]
				if c.tokenRegex.MatchString(w) && !ok {
					tokens = append(tokens, w)
				}
			}
		}

		if tokenType == html.StartTagToken {
			if token.Data == "title" {
				doc.Next()
				title = doc.Token().String()

				for _, w := range strings.Split(title, " ") {
					_, ok := selected[w]
					if c.tokenRegex.MatchString(w) && !ok {
						tokens = append(tokens, w)
					}
				}
			}

			if token.DataAtom != atom.A {
				tokenType = doc.Next()
				continue
			}
			for _, attr := range token.Attr {
				if attr.Key == "href" {
					_, ok := selected[attr.Val]
					if c.urlRegex.MatchString(attr.Val) && !ok {
						edgeURLs = append(edgeURLs, attr.Val)
					}
				}
			}
		}
		tokenType = doc.Next()
	}

	if title == "" || len(tokens) == 0 || len(edgeURLs) == 0 {
		return "", nil, nil, errors.Errorf("failed to get sufficient information")
	}
	return title, tokens, edgeURLs, nil
}
