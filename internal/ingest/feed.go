package ingest

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"sift/internal/source"
)

type FeedItem struct {
	URL       string
	Title     string
	Published time.Time
}

func FetchFeedItems(ctx context.Context, client *http.Client, src source.Source) ([]FeedItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "sift/0.1 (+https://sift.local)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request source feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("read source feed body: %w", err)
	}

	items, err := parseFeed(body)
	if err != nil {
		return nil, err
	}

	filtered := make([]FeedItem, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.URL) == "" || strings.TrimSpace(item.Title) == "" {
			continue
		}
		filtered = append(filtered, item)
	}

	return filtered, nil
}

func parseFeed(body []byte) ([]FeedItem, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil, fmt.Errorf("empty feed document")
		}
		if err != nil {
			return nil, fmt.Errorf("read feed XML token: %w", err)
		}

		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}

		switch strings.ToLower(start.Name.Local) {
		case "rss":
			return parseRSS(body)
		case "feed":
			return parseAtom(body)
		case "rdf":
			return parseRDF(body)
		default:
			return nil, fmt.Errorf("unsupported feed root: %s", start.Name.Local)
		}
	}
}

type rssDocument struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	GUID    string `xml:"guid"`
	PubDate string `xml:"pubDate"`
	Date    string `xml:"date"`
}

func parseRSS(body []byte) ([]FeedItem, error) {
	var doc rssDocument
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse RSS feed: %w", err)
	}

	items := make([]FeedItem, 0, len(doc.Channel.Items))
	for _, item := range doc.Channel.Items {
		link := strings.TrimSpace(item.Link)
		if link == "" && looksLikeURL(item.GUID) {
			link = strings.TrimSpace(item.GUID)
		}

		published, _ := parseFeedTime(item.PubDate)
		if published.IsZero() {
			published, _ = parseFeedTime(item.Date)
		}

		items = append(items, FeedItem{
			URL:       link,
			Title:     strings.TrimSpace(item.Title),
			Published: published,
		})
	}

	return items, nil
}

type atomDocument struct {
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title     string     `xml:"title"`
	ID        string     `xml:"id"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
	Links     []atomLink `xml:"link"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

func parseAtom(body []byte) ([]FeedItem, error) {
	var doc atomDocument
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse Atom feed: %w", err)
	}

	items := make([]FeedItem, 0, len(doc.Entries))
	for _, entry := range doc.Entries {
		link := atomLinkURL(entry.Links)
		if link == "" && looksLikeURL(entry.ID) {
			link = strings.TrimSpace(entry.ID)
		}

		published, _ := parseFeedTime(entry.Published)
		if published.IsZero() {
			published, _ = parseFeedTime(entry.Updated)
		}

		items = append(items, FeedItem{
			URL:       link,
			Title:     strings.TrimSpace(entry.Title),
			Published: published,
		})
	}

	return items, nil
}

type rdfDocument struct {
	Items []rssItem `xml:"item"`
}

func parseRDF(body []byte) ([]FeedItem, error) {
	var doc rdfDocument
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse RDF feed: %w", err)
	}

	items := make([]FeedItem, 0, len(doc.Items))
	for _, item := range doc.Items {
		link := strings.TrimSpace(item.Link)
		if link == "" && looksLikeURL(item.GUID) {
			link = strings.TrimSpace(item.GUID)
		}
		published, _ := parseFeedTime(item.PubDate)
		if published.IsZero() {
			published, _ = parseFeedTime(item.Date)
		}
		items = append(items, FeedItem{
			URL:       link,
			Title:     strings.TrimSpace(item.Title),
			Published: published,
		})
	}

	return items, nil
}

func atomLinkURL(links []atomLink) string {
	for _, link := range links {
		if strings.TrimSpace(link.Href) == "" {
			continue
		}
		if strings.TrimSpace(link.Rel) == "" || strings.EqualFold(strings.TrimSpace(link.Rel), "alternate") {
			return strings.TrimSpace(link.Href)
		}
	}
	for _, link := range links {
		if strings.TrimSpace(link.Href) != "" {
			return strings.TrimSpace(link.Href)
		}
	}
	return ""
}

func parseFeedTime(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}

	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		time.RFC850,
		time.ANSIC,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 MST",
	}

	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported date format: %s", value)
}

func looksLikeURL(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://")
}
