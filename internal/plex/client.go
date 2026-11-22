package plex

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const Product = "Q2 Plex"
const CloudURL = "https://clients.plex.tv"
const pageSize = 25

type State struct {
	ClientID    string `json:"clientId"`
	PrivateKey  string `json:"privateKey"`
	KeyID       string `json:"keyId"`
	Token       string `json:"token,omitempty"`
	TokenTime   int64  `json:"tokenTime,omitempty"`
	ServerName  string `json:"serverName,omitempty"`
	ServerURL   string `json:"serverUrl,omitempty"`
	ServerToken string `json:"serverToken,omitempty"`
	LibraryID   string `json:"libraryId,omitempty"`
}

type Pin struct {
	ID        int64  `json:"id"`
	Code      string `json:"code"`
	AuthToken string `json:"authToken"`
	ExpiresIn int64  `json:"expiresIn"`
}
type Connection struct {
	URI   string `json:"uri"`
	Local bool   `json:"local"`
	Relay bool   `json:"relay"`
}
type Resource struct {
	Name        string       `json:"name"`
	Provides    string       `json:"provides"`
	AccessToken string       `json:"accessToken"`
	Connections []Connection `json:"connections"`
}
type Item struct {
	Key              string   `json:"key"`
	RatingKey        string   `json:"ratingKey"`
	Type             string   `json:"type"`
	Title            string   `json:"title"`
	ParentTitle      string   `json:"parentTitle"`
	ParentRatingKey  string   `json:"parentRatingKey"`
	Thumb            string   `json:"thumb"`
	ParentThumb      string   `json:"parentThumb"`
	GrandparentThumb string   `json:"grandparentThumb"`
	Composite        string   `json:"composite"`
	PlaylistType     string   `json:"playlistType"`
	UserRating       *float64 `json:"userRating"`
	GrandparentTitle string   `json:"grandparentTitle"`
	Duration         int64    `json:"duration"`
	LeafCount        int      `json:"leafCount"`
	Media            []struct {
		Part []struct {
			Key       string `json:"key"`
			Container string `json:"container"`
		} `json:"Part"`
	} `json:"Media"`
}
type Section struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Type  string `json:"type"`
}
type container struct {
	MediaContainer struct {
		Directory []Section `json:"Directory"`
		Metadata  []Item    `json:"Metadata"`
		TotalSize int       `json:"totalSize"`
	} `json:"MediaContainer"`
}
type Client struct {
	State State
	HTTP  *http.Client
	Home  string
	Path  string
	Cloud string
}

func Load(dir string) (*Client, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	c := &Client{Home: dir, Path: filepath.Join(dir, "auth.json"), Cloud: CloudURL}
	if b, err := os.ReadFile(c.Path); err == nil {
		if err = json.Unmarshal(b, &c.State); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if c.State.ClientID == "" || c.State.PrivateKey == "" {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		id := make([]byte, 16)
		if _, err = rand.Read(id); err != nil {
			return nil, err
		}
		c.State.ClientID = hex.EncodeToString(id)
		c.State.PrivateKey = base64.RawURLEncoding.EncodeToString(priv)
		hash := sha256.Sum256(append(priv.Seed(), pub...))
		c.State.KeyID = hex.EncodeToString(hash[:])
		if err = c.Save(); err != nil {
			return nil, err
		}
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if b, err := os.ReadFile(filepath.Join(dir, "cacert.pem")); err == nil {
		pool.AppendCertsFromPEM(b)
	}
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:     &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout: 6 * time.Second, ResponseHeaderTimeout: 7 * time.Second,
		MaxIdleConns: 4, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second,
	}
	c.HTTP = &http.Client{Transport: transport, Timeout: 9 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	return c, nil
}
func (c *Client) Save() error {
	b, err := json.MarshalIndent(c.State, "", "  ")
	if err != nil {
		return err
	}
	path := c.Path + ".tmp"
	if err = os.WriteFile(path, b, 0600); err != nil {
		return err
	}
	return os.Rename(path, c.Path)
}
func (c *Client) headers(req *http.Request, token string) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Product", Product)
	req.Header.Set("X-Plex-Version", "0.3.0")
	req.Header.Set("X-Plex-Platform", "Linux")
	req.Header.Set("X-Plex-Device", "Shanling Q2")
	req.Header.Set("X-Plex-Client-Identifier", c.State.ClientID)
	if token != "" {
		req.Header.Set("X-Plex-Token", token)
	}
}
func (c *Client) Request(ctx context.Context, method, root, path, token string, body io.Reader, out any) error {
	u, err := url.Parse(root)
	if err != nil {
		return err
	}
	rel, err := url.Parse(path)
	if err != nil {
		return err
	}
	target := u.ResolveReference(rel)
	if !allowedRoot(root) {
		return errors.New("insecure server address refused")
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return err
	}
	c.headers(req, token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("plex HTTP %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out)
}
func allowedRoot(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && (ip.IsPrivate() || ip.IsLoopback())
}
func (c *Client) private() (ed25519.PrivateKey, error) {
	b, err := base64.RawURLEncoding.DecodeString(c.State.PrivateKey)
	if err != nil || len(b) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid saved device key")
	}
	return ed25519.PrivateKey(b), nil
}
func (c *Client) jwt(nonce string) (string, error) {
	priv, err := c.private()
	if err != nil {
		return "", err
	}
	header := map[string]any{"alg": "EdDSA", "typ": "JWT", "kid": c.State.KeyID}
	now := time.Now().Unix()
	claims := map[string]any{"aud": "plex.tv", "iss": c.State.ClientID, "iat": now, "exp": now + 300}
	if nonce != "" {
		claims["nonce"] = nonce
		claims["scope"] = "username,friendly_name"
	}
	h, _ := json.Marshal(header)
	p, _ := json.Marshal(claims)
	data := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(p)
	return data + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, []byte(data))), nil
}
func (c *Client) StartPIN(ctx context.Context) (Pin, error) {
	priv, err := c.private()
	if err != nil {
		return Pin{}, err
	}
	pub := priv.Public().(ed25519.PublicKey)
	body, _ := json.Marshal(map[string]any{"jwk": map[string]string{
		"kty": "OKP", "crv": "Ed25519", "x": base64.RawURLEncoding.EncodeToString(pub), "kid": c.State.KeyID, "alg": "EdDSA", "use": "sig",
	}})
	var pin Pin
	err = c.Request(ctx, "POST", c.Cloud, "/api/v2/pins", "", strings.NewReader(string(body)), &pin)
	if err == nil && (pin.ID == 0 || pin.Code == "") {
		err = errors.New("Plex returned no PIN")
	}
	return pin, err
}
func (c *Client) PollPIN(ctx context.Context, id int64) (string, error) {
	jwt, err := c.jwt("")
	if err != nil {
		return "", err
	}
	var result Pin
	err = c.Request(ctx, "GET", c.Cloud, "/api/v2/pins/"+strconv.FormatInt(id, 10)+"?deviceJWT="+url.QueryEscape(jwt), "", nil, &result)
	return result.AuthToken, err
}
func (c *Client) Refresh(ctx context.Context) error {
	var n struct {
		Nonce string `json:"nonce"`
	}
	if err := c.Request(ctx, "GET", c.Cloud, "/api/v2/auth/nonce", "", nil, &n); err != nil {
		return err
	}
	if n.Nonce == "" {
		return errors.New("missing nonce")
	}
	jwt, err := c.jwt(n.Nonce)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{"jwt": jwt})
	var result struct {
		AuthToken string `json:"auth_token"`
	}
	if err = c.Request(ctx, "POST", c.Cloud, "/api/v2/auth/token", "", strings.NewReader(string(body)), &result); err != nil {
		return err
	}
	if result.AuthToken == "" {
		return errors.New("missing refreshed token")
	}
	c.State.Token = result.AuthToken
	c.State.TokenTime = time.Now().Unix()
	return c.Save()
}
func (c *Client) Servers(ctx context.Context) ([]Resource, error) {
	var result []Resource
	err := c.Request(ctx, "GET", c.Cloud, "/api/v2/resources?includeHttps=1&includeRelay=0&includeIPv6=0", c.State.Token, nil, &result)
	if err != nil {
		return nil, err
	}
	servers := make([]Resource, 0, len(result))
	for _, r := range result {
		if strings.Contains(r.Provides, "server") {
			servers = append(servers, r)
		}
	}
	return servers, nil
}
func (c *Client) Discover(ctx context.Context) error {
	servers, err := c.Servers(ctx)
	if err != nil {
		return err
	}
	for _, srv := range servers {
		list := append([]Connection(nil), srv.Connections...)
		sort.SliceStable(list, func(i, j int) bool {
			score := func(x Connection) int {
				if x.Relay {
					return 3
				}
				if x.Local {
					return 0
				}
				return 1
			}
			return score(list[i]) < score(list[j])
		})
		for _, conn := range list {
			if conn.Relay || !allowedRoot(conn.URI) {
				continue
			}
			var lib container
			if err = c.Request(ctx, "GET", conn.URI, "/library/sections", srv.AccessToken, nil, &lib); err == nil {
				c.State.ServerName = srv.Name
				c.State.ServerURL = conn.URI
				c.State.ServerToken = srv.AccessToken
				if err = c.Save(); err != nil {
					return err
				}
				return nil
			}
		}
	}
	return errors.New("no reachable Plex Media Server")
}
func (c *Client) Libraries(ctx context.Context) ([]Section, error) {
	var result container
	err := c.Request(ctx, "GET", c.State.ServerURL, "/library/sections", c.State.ServerToken, nil, &result)
	if err != nil {
		return nil, err
	}
	var out []Section
	for _, s := range result.MediaContainer.Directory {
		if s.Type == "artist" {
			out = append(out, s)
		}
	}
	return out, nil
}
func (c *Client) Items(ctx context.Context, kind, id string, offset int) ([]Item, bool, error) {
	var path string
	switch kind {
	case "artists":
		path = "/library/sections/" + url.PathEscape(c.State.LibraryID) + "/all?type=8"
	case "albums":
		path = "/library/sections/" + url.PathEscape(c.State.LibraryID) + "/all?type=9"
	case "artist":
		path = "/library/metadata/" + url.PathEscape(id) + "/children"
	case "album":
		path = "/library/metadata/" + url.PathEscape(id) + "/children"
	case "playlist":
		if !ratingKey(id) {
			return nil, false, errors.New("invalid playlist ID")
		}
		path = "/playlists/" + url.PathEscape(id) + "/items"
	case "favorites":
		path = "/library/sections/" + url.PathEscape(c.State.LibraryID) + "/all?type=10&userRating=10"
	default:
		return nil, false, errors.New("unknown library view")
	}
	sep := "&"
	if !strings.Contains(path, "?") {
		sep = "?"
	}
	path += sep + "X-Plex-Container-Start=" + strconv.Itoa(offset) + "&X-Plex-Container-Size=" + strconv.Itoa(pageSize+1)
	var result container
	err := c.Request(ctx, "GET", c.State.ServerURL, path, c.State.ServerToken, nil, &result)
	if err != nil {
		return nil, false, err
	}
	items := result.MediaContainer.Metadata
	hasMore := len(items) > pageSize
	if hasMore {
		items = items[:pageSize]
	}
	return items, hasMore, nil
}
func (c *Client) Track(ctx context.Context, id string) (Item, error) {
	var result container
	err := c.Request(ctx, "GET", c.State.ServerURL, "/library/metadata/"+url.PathEscape(id), c.State.ServerToken, nil, &result)
	if err != nil {
		return Item{}, err
	}
	if len(result.MediaContainer.Metadata) == 0 {
		return Item{}, errors.New("track not found")
	}
	track := result.MediaContainer.Metadata[0]
	if track.Type != "track" || len(track.Media) == 0 || len(track.Media[0].Part) == 0 {
		return Item{}, errors.New("not a playable music track")
	}
	return track, nil
}
func (c *Client) Stream(ctx context.Context, id string, w http.ResponseWriter, r *http.Request) error {
	track, err := c.Track(ctx, id)
	if err != nil {
		return err
	}
	part := track.Media[0].Part[0].Key
	if !strings.HasPrefix(part, "/library/parts/") || strings.Contains(part, "..") {
		return errors.New("invalid media part path")
	}
	u, err := url.Parse(c.State.ServerURL)
	if err != nil {
		return err
	}
	rel, _ := url.Parse(part)
	u.Path = rel.Path
	u.RawQuery = rel.RawQuery
	req, err := http.NewRequestWithContext(ctx, r.Method, u.String(), nil)
	if err != nil {
		return err
	}
	c.headers(req, c.State.ServerToken)
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	// Audio streams can run for hours; the normal 9s metadata timeout must not
	// terminate playback. Reuse the verified TLS transport, but no total timeout.
	streamHTTP := &http.Client{Transport: c.HTTP.Transport, CheckRedirect: c.HTTP.CheckRedirect}
	resp, err := streamHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("media HTTP %d", resp.StatusCode)
	}
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := resp.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, err = io.CopyBuffer(w, resp.Body, make([]byte, 32<<10))
	return err
}

// AlbumTracks returns a bounded, ordered album track queue without loading an
// entire music library into RAM. Entries are paged from Plex in 25-track blocks.
// A response reaching maxItems is intentionally truncated to bound memory.
func (c *Client) AlbumTracks(ctx context.Context, albumID string, maxItems int) ([]Item, error) {
	if maxItems < 1 || maxItems > 250 {
		return nil, errors.New("invalid queue limit")
	}
	if albumID == "" {
		return nil, errors.New("missing album")
	}
	tracks := make([]Item, 0, min(maxItems, pageSize))
	for offset := 0; offset < maxItems; {
		page, more, err := c.Items(ctx, "album", albumID, offset)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		for _, item := range page {
			if item.Type == "track" && item.RatingKey != "" {
				tracks = append(tracks, item)
				if len(tracks) >= maxItems {
					return tracks, nil
				}
			}
		}
		offset += len(page)
		if !more {
			break
		}
	}
	return tracks, nil
}
