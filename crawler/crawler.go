package crawler

import (
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type Crawler interface {
	AnalyzePage(url string) (title string, tokens, edgeURLs []string, err error)
}

type doogleCrawler struct {
	tokenRegex, urlRegex *regexp.Regexp
}

var _ Crawler = &doogleCrawler{}

func NewCrawler() (Crawler, error) {
	tRegex, err := regexp.Compile("([a-z0-9]+)")
	if err != nil {
		return nil, errors.Errorf("failed to compile tokenRegexp: %v", err)
	}

	urlRegex, err := regexp.Compile(`https?:\/\/(www\.)?[-a-zA-Z0-9@:%._\+~#=]{2,256}\.[a-z]{2,6}\b([-a-zA-Z0-9@:%_\+.~#?&//=]*)`)
	if err != nil {
		return nil, errors.Errorf("failed to compile tokenRegexp: %v", err)
	}

	return &doogleCrawler{tokenRegex: tRegex, urlRegex: urlRegex}, nil
}

func (c *doogleCrawler) AnalyzePage(url string) (string, []string, []string, error) {
	res, err := http.Get(url)
	if err != nil {
		return "", nil, nil, errors.Errorf("failed to https.Get: %v", err)
	}

	defer res.Body.Close()
	return c.analyze(res.Body)
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
					if c.urlRegex.MatchString(attr.Val) && ok {
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
