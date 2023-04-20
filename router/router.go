package router

import (
	"fmt"
	"github.com/MrMelon54/violet/target"
	"github.com/julienschmidt/httprouter"
	"net/http"
	"net/http/httputil"
	"strings"
)

type Router struct {
	route    map[string]*httprouter.Router
	redirect map[string]*httprouter.Router
	notFound http.Handler
	proxy    *httputil.ReverseProxy
}

func New() *Router {
	return &Router{
		route:    make(map[string]*httprouter.Router),
		redirect: make(map[string]*httprouter.Router),
		notFound: http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			_, _ = fmt.Fprintf(rw, "%d %s\n", http.StatusNotFound, http.StatusText(http.StatusNotFound))
		}),
	}
}

func (r *Router) hostRoute(host string) *httprouter.Router {
	h := r.route[host]
	if h == nil {
		h = httprouter.New()
		r.route[host] = h
	}
	return h
}

func (r *Router) hostRedirect(host string) *httprouter.Router {
	h := r.redirect[host]
	if h == nil {
		h = httprouter.New()
		r.redirect[host] = h
	}
	return h
}

func (r *Router) AddService(host string, t target.Route) {
	r.AddRoute(host, "/", t)
}

func (r *Router) AddRoute(host string, path string, t target.Route) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	r.hostRoute(host).Handler(http.MethodGet, path, t)
}

func (r *Router) AddRedirect(host, path string, t target.Redirect) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	r.hostRedirect(host).Handler(http.MethodGet, path, t)
}

func (r *Router) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	host := req.Host
	if r.serveRedirectHTTP(rw, req, host) {
		return
	}
	if r.serveRouteHTTP(rw, req, host) {
		return
	}

	parentHostDot := strings.IndexByte(host, '.')
	if parentHostDot == -1 {
		r.notFound.ServeHTTP(rw, req)
		return
	}

	wildcardHost := "*" + host[parentHostDot:]

	if r.serveRedirectHTTP(rw, req, wildcardHost) {
		return
	}
	if r.serveRouteHTTP(rw, req, wildcardHost) {
		return
	}
}

func (r *Router) serveRouteHTTP(rw http.ResponseWriter, req *http.Request, host string) bool {
	h := r.route[host]
	if h != nil {
		lookup, params, _ := h.Lookup(req.Method, req.URL.Path)
		if lookup != nil {
			lookup(rw, req, params)
			return true
		}
	}
	return false
}

func (r *Router) serveRedirectHTTP(rw http.ResponseWriter, req *http.Request, host string) bool {
	h := r.redirect[host]
	if h != nil {
		lookup, params, _ := h.Lookup(req.Method, req.URL.Path)
		if lookup != nil {
			lookup(rw, req, params)
			return true
		}
	}
	return false
}
