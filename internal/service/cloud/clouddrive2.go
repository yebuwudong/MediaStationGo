package cloud

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// cloudDrive2Provider bridges CloudDrive2 through its WebDAV endpoint.
//
// CloudDrive2 already integrates many cloud disks (115 / 123 / Aliyun / Quark
// and more). Treating it as a WebDAV-backed cloud provider lets MediaStationGo
// browse, mount and upload to those disks without carrying every provider's
// private chunk-upload protocol in this project.
type cloudDrive2Provider struct {
	base     *url.URL
	username string
	password string
	token    string
	ua       string
	client   *http.Client
	proxy    bool
}

func newCloudDrive2(cfg map[string]any, client *http.Client) *cloudDrive2Provider {
	rawURL := str(cfg["url"])
	if rawURL == "" {
		rawURL = str(cfg["server"])
	}
	u, _ := url.Parse(strings.TrimRight(rawURL, "/"))
	ua := str(cfg["ua"])
	if ua == "" {
		ua = defaultUA
	}
	proxy := true
	if _, ok := cfg["force_302"]; ok && boolish(cfg["force_302"]) {
		proxy = false
	}
	return &cloudDrive2Provider{
		base:     u,
		username: str(cfg["username"]),
		password: str(cfg["password"]),
		token:    str(cfg["token"]),
		ua:       ua,
		client:   client,
		proxy:    proxy,
	}
}

func (p *cloudDrive2Provider) Type() string { return TypeCloudDrive2 }

func (p *cloudDrive2Provider) Ping(ctx context.Context) error {
	_, err := p.List(ctx, "")
	return err
}

func (p *cloudDrive2Provider) List(ctx context.Context, dir string) ([]FileEntry, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	target := normalizeCloudDAVPath(dir)
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", p.urlFor(target), strings.NewReader(cloudDAVPropfindBody))
	if err != nil {
		return nil, err
	}
	p.auth(req)
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Accept", "application/xml,text/xml,*/*")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("clouddrive2: list %s returned http %d", target, resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var multi cloudDAVMultiStatus
	if err := xml.Unmarshal(body, &multi); err != nil {
		return nil, fmt.Errorf("clouddrive2: decode webdav: %w", err)
	}
	basePath := strings.TrimRight(p.base.EscapedPath(), "/")
	currentID := normalizeCloudDAVPath(target)
	out := make([]FileEntry, 0, len(multi.Responses))
	for _, item := range multi.Responses {
		entryPath, err := p.entryIDFromHref(item.Href, basePath)
		if err != nil || entryPath == "" || sameCloudDAVPath(entryPath, currentID) {
			continue
		}
		name := firstNonEmpty(item.PropStat.Prop.DisplayName, path.Base(strings.TrimRight(entryPath, "/")))
		if decoded, err := url.PathUnescape(name); err == nil {
			name = decoded
		}
		if name == "" || name == "." || name == "/" {
			continue
		}
		out = append(out, FileEntry{
			ID:    entryPath,
			Name:  name,
			IsDir: item.PropStat.Prop.ResourceType.Collection != nil || strings.HasSuffix(item.Href, "/"),
			Size:  parseDAVSize(item.PropStat.Prop.ContentLength),
		})
	}
	return out, nil
}

func (p *cloudDrive2Provider) Resolve(ctx context.Context, fileRef string) (*DirectLink, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	ref := normalizeCloudDAVPath(fileRef)
	if ref == "/" {
		return nil, errors.New("clouddrive2: file reference required")
	}
	headers := map[string]string{
		"User-Agent": p.ua,
	}
	if p.token != "" {
		headers["Authorization"] = p.token
	} else if p.username != "" {
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(p.username+":"+p.password))
	}
	return &DirectLink{URL: p.urlFor(ref), Headers: headers, Proxy: p.proxy}, nil
}

func (p *cloudDrive2Provider) validate() error {
	if p.base == nil || p.base.Scheme == "" || p.base.Host == "" {
		return errors.New("clouddrive2: missing WebDAV URL")
	}
	return nil
}

func (p *cloudDrive2Provider) auth(req *http.Request) {
	req.Header.Set("User-Agent", p.ua)
	if p.token != "" {
		req.Header.Set("Authorization", p.token)
		return
	}
	if p.username != "" {
		req.SetBasicAuth(p.username, p.password)
	}
}

func (p *cloudDrive2Provider) urlFor(remotePath string) string {
	u := *p.base
	basePath := strings.TrimRight(u.EscapedPath(), "/")
	segments := make([]string, 0)
	if basePath != "" && basePath != "/" {
		segments = append(segments, strings.Trim(basePath, "/"))
	}
	for _, part := range strings.Split(strings.Trim(normalizeCloudDAVPath(remotePath), "/"), "/") {
		if part != "" {
			segments = append(segments, url.PathEscape(part))
		}
	}
	u.RawPath = ""
	u.Path = "/" + strings.Join(segments, "/")
	return u.String()
}

func (p *cloudDrive2Provider) entryIDFromHref(href, basePath string) (string, error) {
	if href == "" {
		return "", nil
	}
	parsed, err := url.Parse(href)
	if err != nil {
		return "", err
	}
	hrefPath := parsed.EscapedPath()
	if hrefPath == "" {
		hrefPath = href
	}
	if basePath != "" && basePath != "/" {
		hrefPath = strings.TrimPrefix(hrefPath, basePath)
	}
	if decoded, err := url.PathUnescape(hrefPath); err == nil {
		hrefPath = decoded
	}
	return normalizeCloudDAVPath(hrefPath), nil
}

const cloudDAVPropfindBody = `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:">
  <d:prop>
    <d:displayname/>
    <d:getcontentlength/>
    <d:resourcetype/>
  </d:prop>
</d:propfind>`

type cloudDAVMultiStatus struct {
	Responses []cloudDAVResponse `xml:"response"`
}

type cloudDAVResponse struct {
	Href     string           `xml:"href"`
	PropStat cloudDAVPropStat `xml:"propstat"`
}

type cloudDAVPropStat struct {
	Prop cloudDAVProp `xml:"prop"`
}

type cloudDAVProp struct {
	DisplayName   string               `xml:"displayname"`
	ContentLength string               `xml:"getcontentlength"`
	ResourceType  cloudDAVResourceType `xml:"resourcetype"`
}

type cloudDAVResourceType struct {
	Collection *struct{} `xml:"collection"`
}

func normalizeCloudDAVPath(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	if p == "" || p == "." {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	cleaned := path.Clean(p)
	if cleaned == "." {
		return "/"
	}
	return cleaned
}

func sameCloudDAVPath(a, b string) bool {
	return strings.TrimRight(normalizeCloudDAVPath(a), "/") == strings.TrimRight(normalizeCloudDAVPath(b), "/")
}

func parseDAVSize(raw string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return n
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
