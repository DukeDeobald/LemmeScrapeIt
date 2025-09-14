package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/PuerkitoBio/goquery"
)

func fetch(ctx context.Context, url string) (*goquery.Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
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

func extractTitle(doc *goquery.Document) (string, error) {
	title := doc.Find("title").First().Text()
	return title, nil
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	doc, err := fetch(ctx, "https://quotes.toscrape.com/")
	if err != nil {
		fmt.Println("Error fetching:", err)
		return
	}

	title, err := extractTitle(doc)
	if err != nil {
		fmt.Println("Error parsing:", err)
		return
	}

	fmt.Println("Title:", title)

	links := extractLinks(doc, "https://quotes.toscrape.com/")
	fmt.Println("List of all the links:", links)
	elapsed := time.Since(start)
	fmt.Printf("Running time: %s\n", elapsed)
}
