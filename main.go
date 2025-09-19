package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type pageResult struct {
	URL   string
	Title string
	Err   error
}

func newHTTPClient() *http.Client {
	tr := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 60 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:        128,
		MaxIdleConnsPerHost: 32,
		MaxConnsPerHost:     32,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	return &http.Client{Transport: tr, Timeout: 15 * time.Second}
}

func fetch(ctx context.Context, client *http.Client, url string) (*goquery.Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "LemmeScrapeIt/0.1")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	return doc, err
}

func extractTitle(doc *goquery.Document) string {
	return strings.TrimSpace(doc.Find("title").First().Text())
}

func extractLinks(doc *goquery.Document, baseURL string) []string {
	var links []string

	base, err := url.Parse(baseURL)
	if err != nil {
		return links
	}
	seen := make(map[string]struct{})

	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		link, exists := s.Attr("href")
		if !exists {
			return
		}
		ref, err := url.Parse(link)
		if err != nil {
			return
		}
		normalLink := base.ResolveReference(ref).String()
		if _, dup := seen[normalLink]; dup {
			return
		}
		seen[normalLink] = struct{}{}
		links = append(links, normalLink)
	})
	return links
}

func main() {
	start := time.Now()
	runCtx, runCancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer runCancel()
	client := newHTTPClient()
	targetURL := "https://quotes.toscrape.com/"

	doc, err := fetch(runCtx, client, targetURL)
	if err != nil {
		fmt.Println("Error fetching:", err)
		return
	}

	title := extractTitle(doc)

	fmt.Println("Title:", title)

	links := extractLinks(doc, targetURL)
	fmt.Println("List of all the links:", links)

	baseURL, _ := url.Parse(targetURL)
	baseHost := strings.ToLower(baseURL.Hostname())

	filtered := make([]string, 0, len(links))
	for _, u := range links {
		uu, err := url.Parse(u)
		if err != nil {
			continue
		}
		if uu.Scheme != "http" && uu.Scheme != "https" {
			continue
		}

		h := strings.ToLower(uu.Hostname())
		if h != baseHost && !strings.HasSuffix(h, "."+baseHost) {
			continue
		}

		filtered = append(filtered, uu.String())
	}

	const workers = 8
	jobs := make(chan string, len(filtered))
	results := make(chan pageResult, len(filtered))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for u := range jobs {
				reqCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				doc, err := fetch(reqCtx, client, u)
				cancel()

				if err != nil {
					results <- pageResult{URL: u, Err: err}
					continue
				}
				results <- pageResult{URL: u, Title: extractTitle(doc)}
			}
		}(i)
	}

	for _, u := range filtered {
		jobs <- u
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	ok, fail := 0, 0
	for r := range results {
		if r.Err != nil {
			fail++
			continue
		}
		ok++
		fmt.Printf("%s - %s\n", r.URL, r.Title)
	}
	fmt.Printf("Fetched %d OK, %d errors\n", ok, fail)
	elapsed := time.Since(start)
	fmt.Printf("Running time: %s\n", elapsed)
}
