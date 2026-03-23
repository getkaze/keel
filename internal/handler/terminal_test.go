package handler

import (
	"net/http"
	"testing"
)

func makeWSRequest(host, origin string) *http.Request {
	r := &http.Request{
		Host:   host,
		Header: make(http.Header),
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}

func TestCheckWSOrigin_EmptyOrigin(t *testing.T) {
	r := makeWSRequest("myapp.example.com:8080", "")
	if !checkWSOrigin(r) {
		t.Error("expected true for empty origin (non-browser client)")
	}
}

func TestCheckWSOrigin_SameHost(t *testing.T) {
	r := makeWSRequest("myapp.example.com:8080", "http://myapp.example.com:8080")
	if !checkWSOrigin(r) {
		t.Error("expected true for same-host origin")
	}
}

func TestCheckWSOrigin_SameHostDifferentPort(t *testing.T) {
	r := makeWSRequest("myapp.example.com:8080", "http://myapp.example.com:3000")
	if !checkWSOrigin(r) {
		t.Error("expected true for same host different port")
	}
}

func TestCheckWSOrigin_Localhost(t *testing.T) {
	r := makeWSRequest("myapp.example.com:8080", "http://localhost:3000")
	if !checkWSOrigin(r) {
		t.Error("expected true for localhost origin")
	}
}

func TestCheckWSOrigin_127001(t *testing.T) {
	r := makeWSRequest("myapp.example.com:8080", "http://127.0.0.1:8080")
	if !checkWSOrigin(r) {
		t.Error("expected true for 127.0.0.1 origin")
	}
}

func TestCheckWSOrigin_IPv6Loopback(t *testing.T) {
	r := makeWSRequest("myapp.example.com:8080", "http://[::1]:8080")
	if !checkWSOrigin(r) {
		t.Error("expected true for ::1 origin")
	}
}

func TestCheckWSOrigin_CrossOrigin_Rejected(t *testing.T) {
	r := makeWSRequest("myapp.example.com:8080", "http://evil.example.com")
	if checkWSOrigin(r) {
		t.Error("expected false for cross-origin request")
	}
}

func TestCheckWSOrigin_InvalidOrigin(t *testing.T) {
	r := makeWSRequest("myapp.example.com:8080", "://invalid")
	if checkWSOrigin(r) {
		t.Error("expected false for invalid origin URL")
	}
}

func TestCheckWSOrigin_HTTPS(t *testing.T) {
	r := makeWSRequest("myapp.example.com:443", "https://myapp.example.com")
	if !checkWSOrigin(r) {
		t.Error("expected true for same host HTTPS")
	}
}
