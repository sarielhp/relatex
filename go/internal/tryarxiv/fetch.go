// Package tryarxiv downloads arXiv e-print sources politely and runs relatex on each: the
// engine of the try_arxiv command, which lets a user try relatex on real papers.
package tryarxiv

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// UserAgent identifies the tool to arXiv; it carries no personal contact address.
const UserAgent = "relatex-try_arxiv (+https://github.com/sarielhp/relatex)"

// MinDelay is the shortest pause between two requests to arXiv (its API terms ask for three seconds).
const MinDelay = 3 * time.Second

// maxDownload caps one e-print download.
const maxDownload = 200 << 20

// Client is a polite arXiv client: one request at a time, MinDelay or more between requests.
type Client struct {
	HTTP    *http.Client
	APIBase string // https://export.arxiv.org by default
	Delay   time.Duration
	Sleep   func(time.Duration)
	Now     func() time.Time
	last    time.Time
}

// NewClient returns a client for arXiv's export host.
func NewClient(delay time.Duration) *Client {
	if delay < MinDelay {
		delay = MinDelay
	}
	return &Client{
		HTTP:    &http.Client{Timeout: 5 * time.Minute},
		APIBase: "https://export.arxiv.org",
		Delay:   delay,
		Sleep:   time.Sleep,
		Now:     time.Now,
	}
}

// get performs one paced GET and returns the body (at most maxDownload bytes).
func (c *Client) get(ctx context.Context, address string) ([]byte, error) {
	if !c.last.IsZero() {
		if wait := c.Delay - c.Now().Sub(c.last); wait > 0 {
			c.Sleep(wait)
		}
	}
	c.last = c.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", UserAgent)
	response, err := c.HTTP.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("arXiv answered %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxDownload+1))
	if err == nil && len(body) > maxDownload {
		err = fmt.Errorf("download larger than %d MB", maxDownload>>20)
	}
	return body, err
}

// EPrint downloads the source of one paper: a gzip-compressed tar archive, a gzip-compressed
// single file, or (for a paper without source) a PDF.
func (c *Client) EPrint(ctx context.Context, id string) ([]byte, error) {
	return c.get(ctx, c.APIBase+"/e-print/"+id)
}

type feed struct {
	Entries []struct {
		ID string `xml:"id"`
	} `xml:"entry"`
}

// Recent lists the identifiers of the newest papers of an arXiv category group such as "cs" (one API request).
func (c *Client) Recent(ctx context.Context, category string, count int) ([]string, error) {
	query := url.Values{}
	query.Set("search_query", "cat:"+category+".*")
	query.Set("sortBy", "submittedDate")
	query.Set("sortOrder", "descending")
	query.Set("max_results", fmt.Sprint(count))
	body, err := c.get(ctx, c.APIBase+"/api/query?"+query.Encode())
	if err != nil {
		return nil, err
	}
	return parseFeed(body)
}

func parseFeed(body []byte) ([]string, error) {
	var parsed feed
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("arXiv listing: %w", err)
	}
	ids := []string{}
	for _, entry := range parsed.Entries {
		if id := identifier(entry.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// identifier is the arXiv identifier (with its version removed) of an abs URL such as
// http://arxiv.org/abs/2401.00001v2 or http://arxiv.org/abs/hep-th/9901001v1.
func identifier(address string) string {
	_, id, found := strings.Cut(address, "/abs/")
	if !found {
		return ""
	}
	if at := strings.LastIndex(id, "v"); at > 0 && allDigits(id[at+1:]) {
		id = id[:at]
	}
	return id
}

func allDigits(text string) bool {
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return text != ""
}

// Pick returns n distinct elements of ids in random order (all of them when there are fewer).
func Pick(ids []string, n int, random *rand.Rand) []string {
	shuffled := append([]string(nil), ids...)
	random.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	if n < len(shuffled) {
		shuffled = shuffled[:n]
	}
	return shuffled
}
