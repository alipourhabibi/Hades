package goproxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoImportHandler_RejectsInjectionInPath(t *testing.T) {
	// The path is reflected into an HTML attribute and served as text/html on
	// the registry's own origin, so an unescaped path is a stored-session XSS.
	h := newTestHandler(t, &fakeAuthorizer{}, nil)

	payloads := []string{
		`/gen/go/alice/"><script>alert(1)</script>`,
		`/gen/go/"><img src=x onerror=alert(1)>/mod`,
		`/gen/go/alice/mod"onmouseover="alert(1)`,
		`/gen/go/../../etc/passwd`,
		`/gen/go/alice`,           // too few segments
		`/gen/go/alice/mod/extra`, // too many segments
		`/gen/go/alice/mod<>`,     // disallowed characters
	}

	for _, p := range payloads {
		t.Run(p, func(t *testing.T) {
			rec := httptest.NewRecorder()
			// The URL is built directly rather than through httptest.NewRequest,
			// which parses its argument as a request line and rejects a path
			// containing a space before the handler ever sees it.
			r := httptest.NewRequest(http.MethodGet, "http://example.com/placeholder?go-get=1", nil)
			r.URL.Path = p
			h.GoImportHandler().ServeHTTP(rec, r)

			assert.Equal(t, http.StatusNotFound, rec.Code)
			assert.NotContains(t, rec.Body.String(), "<script>")
			assert.NotContains(t, rec.Body.String(), "onerror")
		})
	}
}

func TestGoImportHandler_ServesValidPath(t *testing.T) {
	h := newTestHandler(t, &fakeAuthorizer{}, nil)

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "http://example.com/gen/go/alice/mymod?go-get=1", nil)
	h.GoImportHandler().ServeHTTP(rec, r)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `content="example.com/gen/go/alice/mymod mod http://example.com/go"`)
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
}

func TestGoImportHandler_OnlyAcceptsKnownSchemes(t *testing.T) {
	// X-Forwarded-Proto is attacker-controlled unless a proxy overwrites it.
	h := newTestHandler(t, &fakeAuthorizer{}, nil)

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "http://example.com/gen/go/alice/mymod?go-get=1", nil)
	r.Header.Set("X-Forwarded-Proto", `javascript:alert(1)//`)
	h.GoImportHandler().ServeHTTP(rec, r)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "javascript:")
	assert.Contains(t, rec.Body.String(), "http://example.com/go")
}

func TestGoImportHandler_HonoursHTTPSForwardedProto(t *testing.T) {
	h := newTestHandler(t, &fakeAuthorizer{}, nil)

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "http://example.com/gen/go/alice/mymod?go-get=1", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	h.GoImportHandler().ServeHTTP(rec, r)

	assert.Contains(t, rec.Body.String(), "https://example.com/go")
}

func TestGoImportHandler_RequiresGoGetParam(t *testing.T) {
	h := newTestHandler(t, &fakeAuthorizer{}, nil)

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "http://example.com/gen/go/alice/mymod", nil)
	h.GoImportHandler().ServeHTTP(rec, r)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestUnescapeModulePath(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "alice", want: "alice"},
		{in: "my-module_v2", want: "my-module_v2"},
		// GOPROXY encodes uppercase as "!" plus the lowercase letter.
		{in: "!alice", want: "Alice"},
		{in: "!my!mod", want: "MyMod"},
		{in: "a!bc", want: "aBc"},
		// Malformed encodings must not silently produce a different name.
		{in: "trailing!", wantErr: true},
		{in: "!1bad", wantErr: true},
		{in: "Unescaped", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := unescapeModulePath(tc.in)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseModulePath_UnescapesCase(t *testing.T) {
	h := newTestHandler(t, &fakeAuthorizer{}, nil)

	owner, mod, err := h.parseModulePath("example.com/gen/go/!alice/!my!mod")

	require.NoError(t, err)
	assert.Equal(t, "Alice", owner)
	assert.Equal(t, "MyMod", mod)
}

func TestParseModulePath_RejectsWrongPrefix(t *testing.T) {
	h := newTestHandler(t, &fakeAuthorizer{}, nil)

	_, _, err := h.parseModulePath("evil.com/gen/go/alice/mymod")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "does not start with"))
}
