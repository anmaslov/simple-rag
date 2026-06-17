package confluence

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"confluence-rag/backend/internal/config"
)

type Cursor struct {
	Start int
	Limit int
}

type Space struct {
	Key  string
	Name string
}

type Ancestor struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type Page struct {
	ID        string
	SpaceKey  string
	Title     string
	URL       string
	Version   int
	Status    string
	RawHTML   string
	Ancestors []Ancestor
	UpdatedAt time.Time
}

type PageBatch struct {
	Pages   []Page
	Next    Cursor
	HasNext bool
}

type Client interface {
	ListSpaces(ctx context.Context) ([]Space, error)
	ListPagesBySpace(ctx context.Context, spaceKey string, cursor Cursor) (PageBatch, error)
	ListChildPages(ctx context.Context, pageID string, cursor Cursor) (PageBatch, error)
	SearchPagesByCQL(ctx context.Context, cql string, cursor Cursor) (PageBatch, error)
	GetPage(ctx context.Context, pageID string) (Page, error)
}

type RESTClient struct {
	cfg  config.ConfluenceConfig
	http *http.Client
	log  *slog.Logger
}

func New(cfg config.ConfluenceConfig, log *slog.Logger) *RESTClient {
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.SkipTLSVerify}} //nolint:gosec // Explicit corporate self-signed opt-in.
	return &RESTClient{cfg: cfg, http: &http.Client{Timeout: 60 * time.Second, Transport: tr}, log: log}
}

func (c *RESTClient) ListSpaces(ctx context.Context) ([]Space, error) {
	u := c.cfg.BaseURL + "/rest/api/space?limit=500"
	var out struct {
		Results []struct{ Key, Name string } `json:"results"`
	}
	if err := c.getJSON(ctx, u, &out); err != nil {
		return nil, err
	}
	spaces := make([]Space, 0, len(out.Results))
	for _, s := range out.Results {
		spaces = append(spaces, Space{Key: s.Key, Name: s.Name})
	}
	return spaces, nil
}

func (c *RESTClient) ListPagesBySpace(ctx context.Context, spaceKey string, cur Cursor) (PageBatch, error) {
	if cur.Limit <= 0 {
		cur.Limit = c.cfg.PageLimit
	}
	q := url.Values{}
	q.Set("spaceKey", spaceKey)
	q.Set("type", "page")
	q.Set("status", "current")
	q.Set("limit", strconv.Itoa(cur.Limit))
	q.Set("start", strconv.Itoa(cur.Start))
	q.Set("expand", "body.storage,version,space,ancestors,_links")
	u := c.cfg.BaseURL + "/rest/api/content?" + q.Encode()
	return c.fetchPageBatch(ctx, u, cur)
}

func (c *RESTClient) ListChildPages(ctx context.Context, pageID string, cur Cursor) (PageBatch, error) {
	if cur.Limit <= 0 {
		cur.Limit = c.cfg.PageLimit
	}
	q := url.Values{}
	q.Set("limit", strconv.Itoa(cur.Limit))
	q.Set("start", strconv.Itoa(cur.Start))
	q.Set("expand", "body.storage,version,space,ancestors,_links")
	u := c.cfg.BaseURL + "/rest/api/content/" + url.PathEscape(pageID) + "/child/page?" + q.Encode()
	return c.fetchPageBatch(ctx, u, cur)
}

func (c *RESTClient) SearchPagesByCQL(ctx context.Context, cql string, cur Cursor) (PageBatch, error) {
	if cur.Limit <= 0 {
		cur.Limit = c.cfg.PageLimit
	}
	q := url.Values{}
	q.Set("cql", cql)
	q.Set("limit", strconv.Itoa(cur.Limit))
	q.Set("start", strconv.Itoa(cur.Start))
	q.Set("expand", "body.storage,version,space,ancestors,_links")
	u := c.cfg.BaseURL + "/rest/api/content/search?" + q.Encode()
	return c.fetchPageBatch(ctx, u, cur)
}

func (c *RESTClient) GetPage(ctx context.Context, pageID string) (Page, error) {
	q := url.Values{}
	q.Set("expand", "body.storage,version,space,ancestors,_links")
	u := c.cfg.BaseURL + "/rest/api/content/" + url.PathEscape(pageID) + "?" + q.Encode()
	var raw rawPage
	if err := c.getJSON(ctx, u, &raw); err != nil {
		return Page{}, err
	}
	return c.mapPage(raw), nil
}

func (c *RESTClient) fetchPageBatch(ctx context.Context, u string, cur Cursor) (PageBatch, error) {
	var out struct {
		Results []rawPage `json:"results"`
		Size    int       `json:"size"`
		Limit   int       `json:"limit"`
		Links   struct {
			Next string `json:"next"`
		} `json:"_links"`
	}
	if err := c.getJSON(ctx, u, &out); err != nil {
		return PageBatch{}, err
	}
	pages := make([]Page, 0, len(out.Results))
	for _, p := range out.Results {
		pages = append(pages, c.mapPage(p))
	}
	limit := out.Limit
	if limit == 0 {
		limit = cur.Limit
	}
	return PageBatch{Pages: pages, Next: Cursor{Start: cur.Start + limit, Limit: limit}, HasNext: out.Links.Next != "" || len(pages) == limit}, nil
}

func (c *RESTClient) getJSON(ctx context.Context, u string, dst any) error {
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		c.authorize(req)
		resp, err := c.http.Do(req)
		if err == nil && resp.StatusCode < 300 {
			defer resp.Body.Close()
			return json.NewDecoder(resp.Body).Decode(dst)
		}
		if resp != nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			last = fmt.Errorf("confluence status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
				return last
			}
		} else {
			last = err
		}
		c.log.Warn("confluence request failed", "url", u, "attempt", attempt+1, "error", last)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	return last
}

func (c *RESTClient) authorize(req *http.Request) {
	switch strings.ToLower(c.cfg.AuthType) {
	case "basic":
		req.SetBasicAuth(c.cfg.Username, c.cfg.Token)
	default:
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	}
	req.Header.Set("Accept", "application/json")
}

type rawPage struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Space  struct {
		Key string `json:"key"`
	} `json:"space"`
	Version struct {
		Number int    `json:"number"`
		When   string `json:"when"`
	} `json:"version"`
	Body struct {
		Storage struct {
			Value string `json:"value"`
		} `json:"storage"`
	} `json:"body"`
	Ancestors []Ancestor `json:"ancestors"`
	Links     struct {
		WebUI string `json:"webui"`
		Base  string `json:"base"`
	} `json:"_links"`
}

func (c *RESTClient) mapPage(p rawPage) Page {
	updated, _ := time.Parse(time.RFC3339, p.Version.When)
	web := p.Links.WebUI
	if web != "" && strings.HasPrefix(web, "/") {
		web = c.cfg.BaseURL + web
	}
	if web == "" {
		web = c.cfg.BaseURL + "/pages/viewpage.action?pageId=" + p.ID
	}
	return Page{ID: p.ID, SpaceKey: p.Space.Key, Title: p.Title, URL: web, Version: p.Version.Number, Status: p.Status, RawHTML: p.Body.Storage.Value, Ancestors: p.Ancestors, UpdatedAt: updated}
}
