package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDeprecatedProviderPlaybackOverrideKeysAreIgnored(t *testing.T) {
	pan115 := new115(map[string]any{"cookie": "UID=1; CID=2", "force_proxy": "true"}, http.DefaultClient)
	if pan115.proxy {
		t.Fatalf("115 should keep safe direct mode; force_proxy is deprecated")
	}
	cd2 := newCloudDrive2(map[string]any{"url": "http://example.test/dav", "force_302": "true"}, http.DefaultClient)
	if !cd2.proxy {
		t.Fatalf("clouddrive2 should keep safe proxy mode; force_302 is deprecated")
	}
}

func TestCloudDrive2WebDAVListAndResolve(t *testing.T) {
	var gotAuth, gotDepth, gotRange string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "PROPFIND" && r.URL.Path == "/dav":
			gotAuth = r.Header.Get("Authorization")
			gotDepth = r.Header.Get("Depth")
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/dav/</d:href>
    <d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat>
  </d:response>
  <d:response>
    <d:href>/dav/115/</d:href>
    <d:propstat><d:prop><d:displayname>115</d:displayname><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat>
  </d:response>
  <d:response>
    <d:href>/dav/123/Movie.mkv</d:href>
    <d:propstat><d:prop><d:displayname>Movie.mkv</d:displayname><d:getcontentlength>789</d:getcontentlength><d:resourcetype/></d:prop></d:propstat>
  </d:response>
</d:multistatus>`))
		case r.Method == http.MethodGet && r.URL.Path == "/dav/123/Movie.mkv":
			gotAuth = r.Header.Get("Authorization")
			gotRange = r.Header.Get("Range")
			http.Redirect(w, r, "https://cdn.example.test/123/Movie.mkv?sign=1", http.StatusFound)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeCloudDrive2, map[string]any{"url": srv.URL + "/dav", "username": "u", "password": "p"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	entries, err := p.List(context.Background(), "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if gotDepth != "1" {
		t.Fatalf("Depth = %q, want 1", gotDepth)
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Fatalf("missing basic auth: %q", gotAuth)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %#v", entries)
	}
	if !entries[0].IsDir || entries[0].ID != "/115" {
		t.Fatalf("dir entry wrong: %#v", entries[0])
	}
	if entries[1].IsDir || entries[1].ID != "/123/Movie.mkv" || entries[1].Size != 789 {
		t.Fatalf("file entry wrong: %#v", entries[1])
	}
	link, err := p.Resolve(context.Background(), entries[1].ID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if link.URL != "https://cdn.example.test/123/Movie.mkv?sign=1" {
		t.Fatalf("bad url: %s", link.URL)
	}
	if link.Proxy || len(link.Headers) != 0 {
		t.Fatalf("clouddrive2 video should resolve to pure 302 link: %#v", link)
	}
	if gotRange != "bytes=0-0" {
		t.Fatalf("resolve should probe with a tiny range, got %q", gotRange)
	}
}

func TestCloudDrive2ResolveRejectsWebDAVProxyFallbackWithoutRedirect(t *testing.T) {
	var getSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "PROPFIND" && r.URL.Path == "/dav":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/dav/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response></d:multistatus>`))
		case r.Method == http.MethodGet && r.URL.Path == "/dav/123/Movie.mkv":
			getSeen = true
			w.Header().Set("Content-Range", "bytes 0-0/10")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("x"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeCloudDrive2, map[string]any{"url": srv.URL + "/dav", "username": "u", "password": "p"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Resolve(context.Background(), "/123/Movie.mkv")
	if err == nil || !strings.Contains(err.Error(), "without CDN Location") || !strings.Contains(err.Error(), "refusing WebDAV/proxy fallback") {
		t.Fatalf("resolve error = %v, want pure 302 refusal", err)
	}
	if !getSeen {
		t.Fatal("expected CloudDrive2 WebDAV direct-link probe")
	}
}

func TestOpenListWebDAVListAndResolve(t *testing.T) {
	var gotPath, gotDepth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/login" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/api/fs/get" {
			http.NotFound(w, r)
			return
		}
		if r.Method != "PROPFIND" || r.URL.Path != "/dav" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		gotPath = r.URL.Path
		gotDepth = r.Header.Get("Depth")
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/dav/</d:href>
    <d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat>
  </d:response>
  <d:response>
    <d:href>/dav/Cloud/Movie.mkv</d:href>
    <d:propstat><d:prop><d:displayname>Movie.mkv</d:displayname><d:getcontentlength>1024</d:getcontentlength><d:resourcetype/></d:prop></d:propstat>
  </d:response>
</d:multistatus>`))
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"url": srv.URL + "/dav"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if p.Type() != TypeOpenList {
		t.Fatalf("type = %q, want %q", p.Type(), TypeOpenList)
	}
	entries, err := p.List(context.Background(), "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if gotPath != "/dav" {
		t.Fatalf("path = %q, want /dav", gotPath)
	}
	if gotDepth != "1" {
		t.Fatalf("Depth = %q, want 1", gotDepth)
	}
	if len(entries) != 1 || entries[0].ID != "/Cloud/Movie.mkv" || entries[0].Size != 1024 {
		t.Fatalf("entries = %#v", entries)
	}
	_, err = p.Resolve(context.Background(), entries[0].ID)
	if err == nil || !strings.Contains(err.Error(), "pure 302 playback requires OpenList raw_url") {
		t.Fatalf("openlist video resolve should require raw_url instead of WebDAV proxy fallback, err=%v", err)
	}
}

func TestOpenListListUsesAPIUsernamePasswordWithoutWebDAVFallback(t *testing.T) {
	var loginSeen, listSeen, davSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/auth/login":
			loginSeen = true
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode login body: %v", err)
			}
			if body["username"] != "alice" || body["password"] != "secret" {
				t.Fatalf("login body = %#v", body)
			}
			_, _ = w.Write([]byte(`{"code":200,"data":{"token":"api-token"}}`))
		case "/api/fs/list":
			listSeen = true
			if r.Header.Get("Authorization") != "api-token" {
				t.Fatalf("Authorization = %q, want api-token", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"code":200,"data":{"content":[{"name":"Movies","is_dir":true,"size":0},{"name":"Movie.mkv","is_dir":false,"size":1024}],"total":2}}`))
		case "/dav":
			davSeen = true
			w.WriteHeader(http.StatusMultiStatus)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "username": "alice", "password": "secret"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	entries, err := p.List(context.Background(), "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !loginSeen || !listSeen {
		t.Fatalf("expected api login/list, login=%v list=%v", loginSeen, listSeen)
	}
	if davSeen {
		t.Fatal("openlist API credentials should not fall back to WebDAV")
	}
	if len(entries) != 2 || entries[0].ID != "/Movies" || !entries[0].IsDir || entries[1].ID != "/Movie.mkv" || entries[1].Size != 1024 {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestOpenListMutableProviderUsesAPI(t *testing.T) {
	var mkdirPath, renamePath, renameName, moveSrcDir, moveDstDir string
	var moveNames []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/fs/mkdir":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode mkdir body: %v", err)
			}
			mkdirPath = body["path"]
			if r.Header.Get("Authorization") != "alist-token" {
				t.Fatalf("mkdir Authorization = %q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"code":200,"message":"success"}`))
		case "/api/fs/rename":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode rename body: %v", err)
			}
			renamePath = body["path"]
			renameName = body["name"]
			_, _ = w.Write([]byte(`{"code":200,"message":"success"}`))
		case "/api/fs/move":
			var body struct {
				SrcDir string   `json:"src_dir"`
				DstDir string   `json:"dst_dir"`
				Names  []string `json:"names"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode move body: %v", err)
			}
			moveSrcDir = body.SrcDir
			moveDstDir = body.DstDir
			moveNames = body.Names
			_, _ = w.Write([]byte(`{"code":200,"message":"success"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	mutable, ok := p.(MutableProvider)
	if !ok {
		t.Fatal("openlist should support mutable provider")
	}
	created, err := mutable.Mkdir(context.Background(), "/电视剧", "欧美剧")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if mkdirPath != "/电视剧/欧美剧" || created.ID != "/电视剧/欧美剧" || !created.IsDir {
		t.Fatalf("mkdir path=%q entry=%#v", mkdirPath, created)
	}
	renamed, err := mutable.Rename(context.Background(), "/电视剧/欧美剧", "美剧")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if renamePath != "/电视剧/欧美剧" || renameName != "美剧" || renamed.ID != "/电视剧/美剧" {
		t.Fatalf("rename path=%q name=%q entry=%#v", renamePath, renameName, renamed)
	}
	moved, err := mutable.(MovableProvider).Move(context.Background(), "/待整理/Show.S01E01.mkv", "/动漫/国漫/Show/Season 01", "Show - S01E01.mkv")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if moveSrcDir != "/待整理" || moveDstDir != "/动漫/国漫/Show/Season 01" || len(moveNames) != 1 || moveNames[0] != "Show.S01E01.mkv" {
		t.Fatalf("move src=%q dst=%q names=%#v", moveSrcDir, moveDstDir, moveNames)
	}
	if renamePath != "/动漫/国漫/Show/Season 01/Show.S01E01.mkv" || renameName != "Show - S01E01.mkv" {
		t.Fatalf("post-move rename path=%q name=%q", renamePath, renameName)
	}
	if moved.ID != "/动漫/国漫/Show/Season 01/Show - S01E01.mkv" {
		t.Fatalf("moved entry = %#v", moved)
	}
}

func TestCloudDrive2MutableProviderUsesWebDAV(t *testing.T) {
	var mkcolSeen bool
	var destinations []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "MKCOL" && r.URL.Path == "/dav/TV":
			mkcolSeen = true
			w.WriteHeader(http.StatusCreated)
		case r.Method == "MOVE" && r.URL.Path == "/dav/TV":
			destinations = append(destinations, r.Header.Get("Destination"))
			if r.Header.Get("Overwrite") != "F" {
				t.Fatalf("Overwrite = %q, want F", r.Header.Get("Overwrite"))
			}
			w.WriteHeader(http.StatusCreated)
		case r.Method == "MOVE" && r.URL.Path == "/dav/Inbox/Movie.mkv":
			destinations = append(destinations, r.Header.Get("Destination"))
			if r.Header.Get("Overwrite") != "F" {
				t.Fatalf("Overwrite = %q, want F", r.Header.Get("Overwrite"))
			}
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeCloudDrive2, map[string]any{"url": srv.URL + "/dav", "username": "u", "password": "p"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	mutable, ok := p.(MutableProvider)
	if !ok {
		t.Fatal("clouddrive2 should support mutable provider")
	}
	if _, err := mutable.Mkdir(context.Background(), "", "TV"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := mutable.Rename(context.Background(), "/TV", "电视剧"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	moved, err := mutable.(MovableProvider).Move(context.Background(), "/Inbox/Movie.mkv", "/电影/外语电影/Movie (2026)", "Movie (2026).mkv")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if !mkcolSeen || len(destinations) != 2 {
		t.Fatalf("mkcol=%v destinations=%#v, want mkdir and two MOVE calls", mkcolSeen, destinations)
	}
	if destinations[0] != srv.URL+"/dav/%E7%94%B5%E8%A7%86%E5%89%A7" {
		t.Fatalf("rename Destination = %q", destinations[0])
	}
	if destinations[1] != srv.URL+"/dav/%E7%94%B5%E5%BD%B1/%E5%A4%96%E8%AF%AD%E7%94%B5%E5%BD%B1/Movie%20%282026%29/Movie%20%282026%29.mkv" {
		t.Fatalf("move Destination = %q", destinations[1])
	}
	if moved.ID != "/电影/外语电影/Movie (2026)/Movie (2026).mkv" {
		t.Fatalf("moved entry = %#v", moved)
	}
}

func TestOpenListListAPIFailureDoesNotFallbackToWebDAV(t *testing.T) {
	var davSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":500,"message":"bad password"}`))
		case "/dav":
			davSeen = true
			w.WriteHeader(http.StatusMultiStatus)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "username": "alice", "password": "bad"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.List(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "api login failed") || !strings.Contains(err.Error(), "bad password") {
		t.Fatalf("list error = %v, want api login failure", err)
	}
	if davSeen {
		t.Fatal("openlist API failure fell back to WebDAV")
	}
}

func TestOpenListResolveUsesAPIRawURLFor302Playback(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost || r.URL.Path != "/api/fs/get" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"https://cdn.example.test/movie.mkv?sign=1"}}`))
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := p.Resolve(context.Background(), "/Cloud/Movie.mkv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if gotPath != "/api/fs/get" {
		t.Fatalf("api path = %q, want /api/fs/get", gotPath)
	}
	if gotAuth != "alist-token" {
		t.Fatalf("Authorization = %q, want token", gotAuth)
	}
	if link.URL != "https://cdn.example.test/movie.mkv?sign=1" {
		t.Fatalf("url = %q", link.URL)
	}
	if link.Proxy {
		t.Fatalf("openlist raw_url without required headers should be 302 playback")
	}
}

func TestOpenListResolveCollapsesHostedRawURLRedirectToCDN(t *testing.T) {
	var probeSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/get":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"/d/Cloud/Movie.mkv?sign=1"}}`))
		case "/d/Cloud/Movie.mkv":
			probeSeen = true
			if r.Header.Get("Range") != "bytes=0-0" {
				t.Fatalf("probe Range = %q", r.Header.Get("Range"))
			}
			http.Redirect(w, r, "https://cdn.example.test/movie.mkv?sign=cdn", http.StatusFound)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := p.Resolve(context.Background(), "/Cloud/Movie.mkv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !probeSeen {
		t.Fatal("expected OpenList-hosted raw_url probe")
	}
	if link.URL != "https://cdn.example.test/movie.mkv?sign=cdn" || link.Proxy || len(link.Headers) != 0 {
		t.Fatalf("link = %#v, want collapsed CDN 302 playback", link)
	}
}

func TestOpenListResolveLogsInWithUsernamePasswordForAPIRawURL(t *testing.T) {
	var loginSeen bool
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/auth/login":
			loginSeen = true
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode login body: %v", err)
			}
			if body["username"] != "alice" || body["password"] != "secret" {
				t.Fatalf("login body = %#v", body)
			}
			_, _ = w.Write([]byte(`{"code":200,"data":{"token":"api-token"}}`))
		case "/api/fs/get":
			gotAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"https://cdn.example.test/movie.mkv?sign=1"}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "username": "alice", "password": "secret"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := p.Resolve(context.Background(), "/Cloud/Movie.mkv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !loginSeen {
		t.Fatalf("expected api login before fs/get")
	}
	if gotAuth != "api-token" {
		t.Fatalf("Authorization = %q, want api-token", gotAuth)
	}
	if link.URL != "https://cdn.example.test/movie.mkv?sign=1" || link.Proxy {
		t.Fatalf("link = %#v, want raw_url 302 playback", link)
	}
}

func TestOpenListResolveRejectsProxyWhenAPIRawURLNeedsHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/fs/get" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"/dav/Cloud/Movie.mkv","header":{"Cookie":"sid=abc"}}}`))
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Resolve(context.Background(), "/Cloud/Movie.mkv")
	if err == nil || !strings.Contains(err.Error(), "refusing WebDAV/proxy fallback") || !strings.Contains(err.Error(), "Cookie") {
		t.Fatalf("resolve error = %v, want pure 302 refusal with header names", err)
	}
}

func TestOpenListResolveRejectsHostedRawURLWithoutCDNRedirect(t *testing.T) {
	var probeSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/get":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"/d/Cloud/Movie.mkv?sign=1"}}`))
		case "/d/Cloud/Movie.mkv":
			probeSeen = true
			w.Header().Set("Content-Range", "bytes 0-0/10")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("x"))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Resolve(context.Background(), "/Cloud/Movie.mkv")
	if err == nil || !strings.Contains(err.Error(), "OpenList-hosted raw_url") || !strings.Contains(err.Error(), "no CDN Location") {
		t.Fatalf("resolve error = %v, want hosted raw_url refusal", err)
	}
	if !probeSeen {
		t.Fatal("expected OpenList-hosted raw_url probe")
	}
}

func TestOpenListResolveDoesNotFallbackToWebDAVWhenAPIRawURLFails(t *testing.T) {
	var davSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/get":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":500,"message":"driver cannot provide raw_url"}`))
		case "/dav/Cloud/Movie.mkv":
			davSeen = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Resolve(context.Background(), "/Cloud/Movie.mkv")
	if err == nil || !strings.Contains(err.Error(), "pure 302 playback requires OpenList raw_url") {
		t.Fatalf("resolve error = %v, want raw_url requirement", err)
	}
	if davSeen {
		t.Fatal("openlist video resolve fell back to WebDAV after raw_url failure")
	}
}

func TestOpenListRootURLDefaultsToDAV(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/dav/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response></d:multistatus>`))
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"url": srv.URL + "/"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.List(context.Background(), ""); err != nil {
		t.Fatalf("list: %v", err)
	}
	if gotPath != "/dav" {
		t.Fatalf("path = %q, want /dav", gotPath)
	}
}

func TestOpenListURLForKeepsNonASCIIPathSingleEncoded(t *testing.T) {
	p := newOpenList(map[string]any{"url": "http://example.test:5244/dav/"}, nil)
	got := p.urlFor("/动画电影/爱宠大机密2 (2019) {tmdb-412117}")
	if strings.Contains(got, "%25E") {
		t.Fatalf("url is double-escaped: %s", got)
	}
	want := "http://example.test:5244/dav/%E5%8A%A8%E7%94%BB%E7%94%B5%E5%BD%B1/%E7%88%B1%E5%AE%A0%E5%A4%A7%E6%9C%BA%E5%AF%862%20%282019%29%20%7Btmdb-412117%7D"
	if got != want {
		t.Fatalf("url = %s, want %s", got, want)
	}
}

func TestOpenListDAVStatusErrorIncludesBodyHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte("请先填写有效Cookie并保存"))
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"url": srv.URL + "/dav"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.List(context.Background(), "")
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "请先填写有效Cookie并保存") || !strings.Contains(err.Error(), "WebDAV 地址") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnsupportedProvider(t *testing.T) {
	if _, err := New("dropbox", nil, nil); err != ErrUnsupported {
		t.Fatalf("want ErrUnsupported, got %v", err)
	}
	if _, err := New("quark", nil, nil); err != ErrUnsupported {
		t.Fatalf("quark should be unsupported, got %v", err)
	}
	if IsCloudType("quark") {
		t.Fatal("quark should not be an active cloud provider")
	}
}

var _ = time.Second
