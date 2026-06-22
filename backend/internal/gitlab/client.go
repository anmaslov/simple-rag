package gitlab

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 4 << 20

type Config struct {
	BaseURL       string
	Token         string
	SkipTLSVerify bool
	MaxPages      int
}

type Project struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	WebURL            string `json:"web_url"`
	DefaultBranch     string `json:"default_branch"`
}

type Ref struct {
	Name string `json:"name"`
}

type TreeItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Path string `json:"path"`
	Mode string `json:"mode"`
}

type File struct {
	Path       string
	Content    []byte
	LastCommit string
	WebURL     string
}

type Client interface {
	Test(context.Context) error
	GetProject(context.Context, string) (Project, error)
	SearchProjects(context.Context, string) ([]Project, error)
	ListBranches(context.Context, int64) ([]Ref, error)
	ListTags(context.Context, int64) ([]Ref, error)
	ListTree(context.Context, int64, string) ([]TreeItem, error)
	GetRawFile(context.Context, int64, string, string) (File, error)
}

type RESTClient struct {
	cfg  Config
	http *http.Client
	log  *slog.Logger
}

func New(cfg Config, log *slog.Logger) *RESTClient {
	return &RESTClient{cfg: cfg, http: NewHTTPClient(cfg.SkipTLSVerify), log: log}
}

func NewHTTPClient(skipTLSVerify bool) *http.Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: skipTLSVerify}, //nolint:gosec // Per-connection explicit self-signed certificate opt-in.
	}
	return &http.Client{Transport: tr, Timeout: 60 * time.Second}
}

func (c *RESTClient) Test(ctx context.Context) error {
	var user struct {
		ID int64 `json:"id"`
	}
	if err := c.getJSON(ctx, "/api/v4/user", nil, &user); err != nil {
		return err
	}
	if user.ID == 0 {
		return errors.New("GitLab returned an invalid user")
	}
	return nil
}

func (c *RESTClient) GetProject(ctx context.Context, project string) (Project, error) {
	var out Project
	err := c.getJSON(ctx, "/api/v4/projects/"+url.PathEscape(project), nil, &out)
	return out, err
}

func (c *RESTClient) SearchProjects(ctx context.Context, query string) ([]Project, error) {
	q := url.Values{"search": {query}, "simple": {"true"}, "per_page": {"100"}}
	var out []Project
	err := c.getJSON(ctx, "/api/v4/projects", q, &out)
	return out, err
}

func (c *RESTClient) ListBranches(ctx context.Context, projectID int64) ([]Ref, error) {
	return c.listRefs(ctx, projectID, "branches")
}

func (c *RESTClient) ListTags(ctx context.Context, projectID int64) ([]Ref, error) {
	return c.listRefs(ctx, projectID, "tags")
}

func (c *RESTClient) listRefs(ctx context.Context, projectID int64, kind string) ([]Ref, error) {
	q := url.Values{"per_page": {"100"}}
	var out []Ref
	err := c.getJSON(ctx, fmt.Sprintf("/api/v4/projects/%d/repository/%s", projectID, kind), q, &out)
	return out, err
}

func (c *RESTClient) ListTree(ctx context.Context, projectID int64, ref string) ([]TreeItem, error) {
	maxPages := c.cfg.MaxPages
	if maxPages <= 0 {
		maxPages = 1000
	}
	var out []TreeItem
	for pageNo := 1; pageNo <= maxPages; pageNo++ {
		q := url.Values{"ref": {ref}, "recursive": {"true"}, "per_page": {"100"}, "page": {strconv.Itoa(pageNo)}}
		var batch []TreeItem
		next, err := c.getJSONPage(ctx, fmt.Sprintf("/api/v4/projects/%d/repository/tree", projectID), q, &batch)
		if err != nil {
			return nil, err
		}
		out = append(out, batch...)
		if next == "" || len(batch) == 0 {
			return out, nil
		}
	}
	return nil, errors.New("gitlab pagination limit exceeded")
}

func (c *RESTClient) GetRawFile(ctx context.Context, projectID int64, filePath, ref string) (File, error) {
	q := url.Values{"ref": {ref}}
	apiPath := fmt.Sprintf("/api/v4/projects/%d/repository/files/%s/raw", projectID, url.PathEscape(filePath))
	req, err := c.request(ctx, http.MethodGet, apiPath, q)
	if err != nil {
		return File{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return File{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return File{}, readAPIError(resp)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return File{}, err
	}
	if len(body) > maxResponseBytes {
		return File{}, errors.New("gitlab response body is too large")
	}
	return File{Path: filePath, Content: body, LastCommit: resp.Header.Get("X-Gitlab-Last-Commit-Id")}, nil
}

func (c *RESTClient) getJSON(ctx context.Context, apiPath string, q url.Values, dst any) error {
	_, err := c.getJSONPage(ctx, apiPath, q, dst)
	return err
}

func (c *RESTClient) getJSONPage(ctx context.Context, apiPath string, q url.Values, dst any) (string, error) {
	req, err := c.request(ctx, http.MethodGet, apiPath, q)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", readAPIError(resp)
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes))
	if err := dec.Decode(dst); err != nil {
		return "", err
	}
	return resp.Header.Get("X-Next-Page"), nil
}

func (c *RESTClient) request(ctx context.Context, method, apiPath string, q url.Values) (*http.Request, error) {
	u := strings.TrimRight(c.cfg.BaseURL, "/") + apiPath
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.cfg.Token)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string { return fmt.Sprintf("gitlab API returned status %d", e.Status) }

func readAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return &APIError{Status: resp.StatusCode, Body: strings.TrimSpace(string(body))}
}

func ParseProject(input, baseURL string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("project is required")
	}
	if !strings.Contains(input, "://") {
		return strings.Trim(strings.TrimSuffix(input, ".git"), "/"), nil
	}
	u, err := url.Parse(input)
	if err != nil || u.Host == "" {
		return "", errors.New("invalid project URL")
	}
	if baseURL != "" {
		base, err := url.Parse(baseURL)
		if err == nil && !strings.EqualFold(base.Host, u.Host) {
			return "", errors.New("project URL belongs to another GitLab host")
		}
	}
	projectPath := strings.TrimPrefix(path.Clean(u.Path), "/")
	projectPath = strings.TrimSuffix(projectPath, ".git")
	projectPath = strings.TrimSuffix(projectPath, "/-/tree")
	if projectPath == "" || projectPath == "." {
		return "", errors.New("project path is required")
	}
	if i := strings.Index(projectPath, "/-/"); i >= 0 {
		projectPath = projectPath[:i]
	}
	return projectPath, nil
}
