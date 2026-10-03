package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestGetConf(t *testing.T) {
	conf := GetConf()

	if conf.ClientID == "" {
		t.Fatal("ClientID is empty")
	}
	if conf.Endpoint.AuthURL == "" || conf.Endpoint.TokenURL == "" {
		t.Fatalf("Endpoint is incomplete: %+v", conf.Endpoint)
	}
	if conf.Endpoint.AuthStyle != oauth2.AuthStyleInParams {
		t.Fatalf("AuthStyle = %v, want AuthStyleInParams", conf.Endpoint.AuthStyle)
	}
	if len(conf.Scopes) == 0 {
		t.Fatal("Scopes is empty")
	}
	for _, s := range conf.Scopes {
		if strings.TrimSpace(s) == "" {
			t.Fatalf("Scopes contains a blank entry: %q", conf.Scopes)
		}
	}
}

func TestNewTokenURL(t *testing.T) {
	got := NewTokenURL()

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("NewTokenURL() = %q, not a valid URL: %v", got, err)
	}
	if u.Scheme != "https" || u.Host != "hardcover.app" || u.Path != "/account/api/keys/new" {
		t.Fatalf("NewTokenURL() = %q, want the New API Key page", got)
	}

	wantScope := strings.Join(scopes, " ")
	if gotScope := u.Query().Get("scope"); gotScope != wantScope {
		t.Fatalf("scope param = %q, want %q", gotScope, wantScope)
	}
}

func TestCheckCallback(t *testing.T) {
	const state = "test-state"

	tests := []struct {
		name     string
		query    url.Values
		wantOK   bool
		wantErr  bool
		wantCode string
	}{
		{
			name: "valid success response",
			query: url.Values{
				"state": {state},
				"iss":   {issuer},
				"code":  {"the-code"},
			},
			wantOK:   true,
			wantCode: "the-code",
		},
		{
			name: "state mismatch is ignored as not ours",
			query: url.Values{
				"state": {"someone-elses-state"},
				"iss":   {issuer},
				"code":  {"the-code"},
			},
			wantOK: false,
		},
		{
			name: "wrong issuer is rejected",
			query: url.Values{
				"state": {state},
				"iss":   {"https://evil.example.com"},
				"code":  {"the-code"},
			},
			wantOK:  true,
			wantErr: true,
		},
		{
			name: "authorization server error without description",
			query: url.Values{
				"state": {state},
				"iss":   {issuer},
				"error": {"access_denied"},
			},
			wantOK:  true,
			wantErr: true,
		},
		{
			name: "authorization server error with description",
			query: url.Values{
				"state":             {state},
				"iss":               {issuer},
				"error":             {"access_denied"},
				"error_description": {"the user said no"},
			},
			wantOK:  true,
			wantErr: true,
		},
		{
			name: "missing code",
			query: url.Values{
				"state": {state},
				"iss":   {issuer},
			},
			wantOK:  true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, ok, err := checkCallback(tt.query, state)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (err=%v)", ok, tt.wantOK, err)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && code != tt.wantCode {
				t.Fatalf("code = %q, want %q", code, tt.wantCode)
			}
		})
	}
}

// newTestListener opens a loopback listener the same way Login does, for
// tests that drive awaitCode directly.
func newTestListener(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return l
}

func TestAwaitCodeSuccess(t *testing.T) {
	listener := newTestListener(t)
	defer listener.Close()

	const state = "s1"
	results := make(chan struct {
		code string
		err  error
	}, 1)
	go func() {
		code, err := awaitCode(context.Background(), listener, state)
		results <- struct {
			code string
			err  error
		}{code, err}
	}()

	callbackURL := fmt.Sprintf("http://%s/callback?state=%s&iss=%s&code=good-code",
		listener.Addr(), state, url.QueryEscape(issuer))
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("GET callback: %v", err)
	}
	resp.Body.Close()

	select {
	case r := <-results:
		if r.err != nil {
			t.Fatalf("awaitCode err = %v, want nil", r.err)
		}
		if r.code != "good-code" {
			t.Fatalf("awaitCode code = %q, want %q", r.code, "good-code")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("awaitCode did not return in time")
	}
}

func TestAwaitCodeIgnoresStaleRequestThenSucceeds(t *testing.T) {
	listener := newTestListener(t)
	defer listener.Close()

	const state = "s2"
	results := make(chan struct {
		code string
		err  error
	}, 1)
	go func() {
		code, err := awaitCode(context.Background(), listener, state)
		results <- struct {
			code string
			err  error
		}{code, err}
	}()

	// A stale/foreign callback (wrong state) should be rejected but not
	// terminate the wait.
	staleURL := fmt.Sprintf("http://%s/callback?state=wrong&iss=%s&code=stale-code",
		listener.Addr(), url.QueryEscape(issuer))
	staleResp, err := http.Get(staleURL)
	if err != nil {
		t.Fatalf("GET stale callback: %v", err)
	}
	if staleResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("stale callback status = %d, want %d", staleResp.StatusCode, http.StatusBadRequest)
	}
	staleResp.Body.Close()

	select {
	case r := <-results:
		t.Fatalf("awaitCode returned early for a stale request: code=%q err=%v", r.code, r.err)
	case <-time.After(100 * time.Millisecond):
	}

	goodURL := fmt.Sprintf("http://%s/callback?state=%s&iss=%s&code=good-code",
		listener.Addr(), state, url.QueryEscape(issuer))
	goodResp, err := http.Get(goodURL)
	if err != nil {
		t.Fatalf("GET good callback: %v", err)
	}
	goodResp.Body.Close()

	select {
	case r := <-results:
		if r.err != nil || r.code != "good-code" {
			t.Fatalf("awaitCode = (%q, %v), want (%q, nil)", r.code, r.err, "good-code")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("awaitCode did not return in time")
	}
}

func TestAwaitCodeContextCancelled(t *testing.T) {
	listener := newTestListener(t)
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := awaitCode(ctx, listener, "s3")
	if err == nil {
		t.Fatal("awaitCode err = nil, want context.Canceled")
	}
}

func TestLogOut(t *testing.T) {
	t.Run("nil token does nothing", func(t *testing.T) {
		called := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}))
		defer srv.Close()

		if err := logOutAt(context.Background(), srv.URL, nil); err != nil {
			t.Fatalf("logOutAt = %v, want nil", err)
		}
		if called {
			t.Fatal("server was contacted for a nil token")
		}
	})

	t.Run("empty token does nothing", func(t *testing.T) {
		called := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}))
		defer srv.Close()

		if err := logOutAt(context.Background(), srv.URL, &oauth2.Token{}); err != nil {
			t.Fatalf("logOutAt = %v, want nil", err)
		}
		if called {
			t.Fatal("server was contacted for an empty token")
		}
	})

	t.Run("revokes the refresh token", func(t *testing.T) {
		var gotToken, gotHint, gotClientID string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			gotToken = r.PostForm.Get("token")
			gotHint = r.PostForm.Get("token_type_hint")
			gotClientID = r.PostForm.Get("client_id")
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		token := &oauth2.Token{AccessToken: "access-abc", RefreshToken: "refresh-xyz"}
		if err := logOutAt(context.Background(), srv.URL, token); err != nil {
			t.Fatalf("logOutAt = %v, want nil", err)
		}
		if gotToken != "refresh-xyz" || gotHint != "refresh_token" {
			t.Fatalf("got token=%q hint=%q, want token=%q hint=%q", gotToken, gotHint, "refresh-xyz", "refresh_token")
		}
		if gotClientID != clientID {
			t.Fatalf("got client_id=%q, want %q", gotClientID, clientID)
		}
	})

	t.Run("skips revoke without a refresh token", func(t *testing.T) {
		called := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}))
		defer srv.Close()

		// personal API keys have no refresh token and can't be revoked via OAuth
		token := &oauth2.Token{AccessToken: "hc_pat_abc"}
		if err := logOutAt(context.Background(), srv.URL, token); err != nil {
			t.Fatalf("logOutAt = %v, want nil", err)
		}
		if called {
			t.Fatal("server was contacted for a token without a refresh token")
		}
	})

	t.Run("non-200 response is surfaced as an error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("invalid_token"))
		}))
		defer srv.Close()

		err := logOutAt(context.Background(), srv.URL, &oauth2.Token{AccessToken: "abc", RefreshToken: "def"})
		if err == nil {
			t.Fatal("logOutAt err = nil, want an error for a non-200 response")
		}
		if !strings.Contains(err.Error(), "invalid_token") {
			t.Fatalf("logOutAt err = %v, want it to mention the response body", err)
		}
	})
}
