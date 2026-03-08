package ingest

import "testing"

func TestParseRSSFeed(t *testing.T) {
	t.Parallel()

	body := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item>
      <title>BTC ETF flows rise</title>
      <link>https://example.com/a?utm_source=x&amp;ok=1</link>
      <pubDate>Fri, 06 Mar 2026 16:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`)

	items, err := parseFeed(body)
	if err != nil {
		t.Fatalf("parseFeed returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("unexpected item count: %d", len(items))
	}
	if items[0].Title != "BTC ETF flows rise" {
		t.Fatalf("unexpected title: %q", items[0].Title)
	}
	if items[0].URL == "" {
		t.Fatal("expected URL to be present")
	}
}

func TestParseAtomFeed(t *testing.T) {
	t.Parallel()

	body := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <title>SEC update</title>
    <link rel="alternate" href="https://example.com/sec"/>
    <updated>2026-03-06T16:00:00Z</updated>
  </entry>
</feed>`)

	items, err := parseFeed(body)
	if err != nil {
		t.Fatalf("parseFeed returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("unexpected item count: %d", len(items))
	}
	if items[0].URL != "https://example.com/sec" {
		t.Fatalf("unexpected URL: %s", items[0].URL)
	}
}
