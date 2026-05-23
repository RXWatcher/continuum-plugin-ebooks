// Package httproutes adapts a stdlib http.Handler to the SDK's HttpRoutes.v1.
package httproutes

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"

	pluginv1 "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

// maxBodyBytes caps the request body the host may hand us (the body is fully
// buffered in memory).
const maxBodyBytes = 8 << 20 // 8 MiB

// isHTTPToken reports whether s is a valid RFC7230 method token. httptest /
// http.ReadRequest panic on a method with spaces/control chars, and method
// comes straight from the untrusted RPC payload.
func isHTTPToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 0x21 || r > 0x7e || strings.ContainsRune("()<>@,;:\\\"/[]?={} \t", r) {
			return false
		}
	}
	return true
}

func errResponse(code int32, msg string) *pluginv1.HandleHTTPResponse {
	return &pluginv1.HandleHTTPResponse{
		StatusCode: code,
		Body:       []byte(`{"error":{"message":"` + msg + `"}}`),
		Headers:    map[string]string{"Content-Type": "application/json"},
	}
}

type Server struct {
	pluginv1.UnimplementedHttpRoutesServer
	handler atomic.Pointer[http.Handler]
}

func NewServer() *Server { return &Server{} }

func (s *Server) SetHandler(h http.Handler) {
	if h == nil {
		s.handler.Store(nil)
		return
	}
	s.handler.Store(&h)
}

// ServeHTTP exposes the active handler to a standalone HTTP listener so
// operators can reverse-proxy a hostname (e.g. ebooks.example.com) directly
// to this plugin's public routes (OPDS, kosync, Kobo, Kindle inbound).
// Before SetHandler has been called, returns 503 in the same shape as Handle.
//
// SECURITY: strips inbound X-Silo-* headers before invoking the handler.
// These headers are the host plane's trust channel (X-Silo-User-Id,
// X-Silo-User-Role, etc. — injected by the host's plugin proxy after
// session validation). A client connecting directly to the standalone port
// must never be able to forge them, otherwise auth checks inside handlers
// would accept attacker-supplied identity. Stripping them puts the request
// in the same shape as an anonymous, public-route request; any handler that
// requires authenticated identity will naturally 401.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hPtr := s.handler.Load()
	if hPtr == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"code":"not_ready","message":"plugin not configured"}}`))
		return
	}
	for k := range r.Header {
		if strings.HasPrefix(strings.ToLower(k), "x-silo-") {
			r.Header.Del(k)
		}
	}
	(*hPtr).ServeHTTP(w, r)
}

func (s *Server) Handle(ctx context.Context, req *pluginv1.HandleHTTPRequest) (resp *pluginv1.HandleHTTPResponse, _ error) {
	// Defense in depth: a panic in request reconstruction or the downstream
	// handler must not take down the gRPC serving goroutine.
	defer func() {
		if rec := recover(); rec != nil {
			resp = errResponse(http.StatusInternalServerError, "internal error")
		}
	}()

	hPtr := s.handler.Load()
	if hPtr == nil {
		return &pluginv1.HandleHTTPResponse{
			StatusCode: http.StatusServiceUnavailable,
			Body:       []byte(`{"error":{"code":"not_ready","message":"plugin not configured"}}`),
			Headers:    map[string]string{"Content-Type": "application/json"},
		}, nil
	}
	h := *hPtr

	if b := req.GetBody(); len(b) > maxBodyBytes {
		return errResponse(http.StatusRequestEntityTooLarge, "request body too large"), nil
	}

	rawQuery := ""
	if req.GetQuery() != nil {
		vals := url.Values{}
		for k, v := range req.GetQuery().GetFields() {
			// Use the scalar value, not v.String() (the protobuf debug form:
			// a number arrives as "number_value:50", silently corrupting
			// ?limit= / ?cursor= so pagination breaks).
			switch val := v.AsInterface().(type) {
			case string:
				vals.Set(k, val)
			case bool:
				vals.Set(k, strconv.FormatBool(val))
			case float64:
				vals.Set(k, strconv.FormatFloat(val, 'f', -1, 64))
			}
		}
		rawQuery = vals.Encode()
	}

	method := req.GetMethod()
	if method == "" {
		method = http.MethodGet
	}
	if !isHTTPToken(method) {
		return errResponse(http.StatusBadRequest, "invalid method"), nil
	}

	u := &url.URL{Path: req.GetPath(), RawQuery: rawQuery}
	// http.NewRequestWithContext returns an error (rather than panicking like
	// httptest.NewRequest) on an unparseable method/URL, and propagates the
	// gRPC context so a client disconnect / deadline cancels downstream work.
	httpReq, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(req.GetBody()))
	if err != nil {
		return errResponse(http.StatusBadRequest, "invalid request"), nil
	}
	httpReq.RequestURI = u.RequestURI()
	for k, v := range req.GetHeaders() {
		httpReq.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httpReq)

	res := rec.Result()
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	headers := map[string]string{}
	for k, vs := range res.Header {
		if len(vs) > 0 {
			headers[k] = vs[0]
		}
	}
	return &pluginv1.HandleHTTPResponse{
		StatusCode: int32(rec.Code),
		Headers:    headers,
		Body:       body,
	}, nil
}
