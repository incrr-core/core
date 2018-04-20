package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/artktec/autocert-s3-cache"
	"github.com/go-chi/chi"
	"github.com/golang/groupcache"
	"github.com/segmentio/ksuid"
	"golang.org/x/crypto/acme/autocert"
)

// contextEqualizer is the interface for determing if
// the expected and returned contexts are equal
type contextEqualizer interface {
	Equal(string, string) bool
}

// contextSkipper is a context that can skip
// chunks of the increment cycle
type contextSkipper interface {
	SkipTo() (uint64, error)
}

// webServer holds all of the information needed to run the web
// parts of the service, there are two servers that can be started
// HTTP and HTTPS.
type webServer struct {
	serverID string
	canServe bool

	cache *groupcache.Group // holds the incrr atomic increment key

	APIDomains []string
	local      *localDB // holds the local increment key
	remote     remoteDB // holds the remote (DB) increment key

	http  *serveHTTP
	https *serveHTTPS
}

// serveHTTP is the struct that holds the state for the HTTP server
type serveHTTP struct {
	chi.Router
	server       *http.Server
	shutdownChan chan struct{} // graceful shutdown channel
	shutdownFunc *sync.Once
}

// serveHTTPS is the struct that holds the state for the HTTPS server
type serveHTTPS struct {
	chi.Router
	server       *http.Server
	shutdownChan chan struct{} // graceful shutdown channel
	shutdownFunc *sync.Once
}

// isInDomain checks if a request is within a domain
func (web *webServer) isInDomain(r *http.Request, domains []string) bool {
	for _, v := range domains {
		if strings.ToLower(v) == strings.ToLower(strings.Split(r.Host, ":")[0]) {
			return true
		}
	}
	return false
}

// UseDomains responds only to requests on the passed in domains
func (web *webServer) UseDomains(domains []string) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if web.isInDomain(r, domains) {
				h.ServeHTTP(w, r)
				return
			}
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		})
	}
}

// PublicNSHandlerCheck makes sure that the request to the public handler
// falls within valid parameters otherwise it will return a BadRequest error
func (web *webServer) PublicNSHandlerCheck(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := url.Parse(r.RequestURI)
		if log.OnErr(err).Printf("parsing the validation url: %v", err).HasErr {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}

		base := strings.TrimSuffix(u.Path, filepath.Ext(u.Path))

		if len(strings.Split(u.Path, "/")) > 5 {
			err = fmt.Errorf("path has too many seperators & %v", err)
		}

		if len(base) > 300 {
			err = fmt.Errorf("path is too long & %v", err)
		}

		if len(filepath.Ext(u.Path)) > 0 && filepath.Ext(u.Path) != ".json" {
			err = fmt.Errorf("path does not support this extension & %v", err)
		}

		if utf8.RuneCountInString(base) != len(base) {
			err = fmt.Errorf("path has non-ASCII character(s) & %v", err)
		}

		if strings.ContainsAny(base, "~`!@#$%^&*()_+=-{}|[]\\:\";'<>?,.") {
			err = fmt.Errorf("path has URL special character(s) & %v", err)
		}

		if err != nil {
			responseOnErr(w, ErrBadRequest{err})
			return
		}

		h.ServeHTTP(w, r)
	})
}

// PublicNSHandler handles all of the api traffic that serves public namespaced keys
func (web *webServer) PublicNSHandler(w http.ResponseWriter, r *http.Request) {
	if !web.canServe {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
	}

	var err error

	// 1. See if we have the key locally, and GET that value
	// 2. If no key locally, see if we have the key remotely
	// 3. Check that the value is in Groupcache, if not then the key was evicted
	// 4. Replicas should take care of members joining and leaving the pool

	var idxB, idx, hasKey = []byte{}, uint64(0), false
	ns := "pub/" + chi.URLParam(r, "*") // adds the pub/ prefix back in for consistency
	if idxB = web.local.Get([]byte(ns)); len(idxB) == 0 {
		hasKey = web.remote.HasKey(ns)
		if !hasKey {
			// if we can't find the key, then we pull the latest...
			// this should be okay because we should be looking
			// for pretty old keys, and new keys should be in memory
			web.remote.Keys()
			hasKey = web.remote.HasKey(ns)
		}
	}

	if len(idxB) > 0 {
		hasKey = true
		idx, err = strconv.ParseUint(string(idxB), 10, 64)
		if err != nil {
			log.Printf("[public NS] strconv idx: %v", err)
			responseOnErr(w, ErrInternalService{err})
			return
		}
	}

RestartCount:
	max := idx + 10000
	for ; idx < max; idx++ {
		var cacheCtx contextEqualizer
		var respCtx contextResponder
		var respStr string

		// check remote if don't have local, but do remote OR if you're sending the very first request
		ts := strconv.FormatInt(time.Now().UnixNano(), 10)
		if (hasKey && len(idxB) == 0) || (max-idx) == 0 {
			cacheCtx = &remoteContext{respContext: &respContext{ServerID: web.serverID, Timestamp: ts}}
			respCtx = &remoteContext{}
		} else {
			cacheCtx = &respContext{ServerID: web.serverID, Timestamp: ts}
			respCtx = &respContext{}
		}

		if err = web.cache.Get(cacheCtx, fmt.Sprintf("%d:%s", idx, ns), groupcache.StringSink(&respStr)); err != nil {
			log.Printf("[public NS] cache response: %v", err)
			responseOnErr(w, ErrInternalService{err})
			return
		}

		if err = json.Unmarshal([]byte(respStr), &respCtx); err != nil {
			log.Printf("[public NS] json unmarshal: %v", err)
			responseOnErr(w, ErrInternalService{err})
			return
		}

		// see if we can claim a number
		resp := respCtx.Data()
		ctxAreEqual := cacheCtx.Equal(resp.ServerID, resp.Timestamp)

		if ctxAreEqual && resp.Number == strconv.FormatUint(idx, 10) {
			// the JSON sent back from Groupcache can "change" type, so we
			// get the Response() back to figure out what type the response
			// really was meant to be
			if skip, ok := respCtx.Response().(contextSkipper); ok {
				ctxSkipTo, err := skip.SkipTo()
				if err != nil {
					responseOnErr(w, ErrInternalService{err})
					return
				}
				idx = ctxSkipTo
				goto RestartCount // yup, it's been considered and accepted
			}

			fmt.Fprintf(w, "%d", idx)
			return
		}
	}

	responseOnErr(w, ErrBadRequest{errMaxIncrementRange})
}

// HealthcheckHandler returns 200 OK when things are healthy
func (web *webServer) HealthcheckHandler(w http.ResponseWriter, r *http.Request) {
	if !web.canServe {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
	}

	http.Error(w, http.StatusText(http.StatusOK), http.StatusOK)
}

// responseOnErr returns a standard HTTP error code response based on the error type. Wrap
// errors with a supported type for the expected response
func responseOnErr(w http.ResponseWriter, err error) {
	switch err.(type) {
	case ErrInternalService:
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	case ErrBadRequest:
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
	default:
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
	}
}

// setupWebServer does the configuration setup for the webServer object
func setupWebServer(config *configuration) *webServer {
	ws := config.Web
	if ws == nil {
		ws = new(webServer)
	}

	ws.serverID = ksuid.New().String()

	if len(config.Server.Ports.HTTP) == 0 {
		config.Server.Ports.HTTP = defaultHTTPPort
	}

	ws.http = new(serveHTTP)
	ws.http.Router = chi.NewRouter()
	ws.http.Use(log.HTTPMiddleware)
	ws.http.server = &http.Server{Addr: config.Server.Ports.HTTP, Handler: ws.http.Router}

	if len(config.Server.Ports.HTTPS) == 0 {
		config.Server.Ports.HTTPS = defaultHTTPSPort
	}

	ws.https = new(serveHTTPS)
	ws.https.Router = chi.NewRouter()
	ws.https.Use(log.HTTPMiddleware)
	ws.https.server = &http.Server{Addr: config.Server.Ports.HTTPS, Handler: ws.https.Router}

	if config.Server.ForceHTTP {
		return ws

	}
	// if we're gonna do HTTPS then load certs or grab some LetsEncrypt certs to serve uo TLS

	var tlsConfig *tls.Config
	crt, key, uri := config.Server.Certs.Certificate, config.Server.Certs.PrivateKey, config.Server.LetsEncrypt.CertificateURI

	crtL, keyL, uriL := len(crt), len(key), len(uri)
	switch {
	case (crtL > 0 || keyL > 0) && uriL > 0:
		log.Fatal("must use only server.certs.private_key and server.certs.certificate and not letencrypt.uri or vice versa")
	case crtL > 0 && keyL == 0, crtL == 0 && keyL > 0:
		log.Fatal("must have both server.certs.private_key and server.certs.certificate")
	case crtL == 0 && keyL == 0 && uriL == 0 && !config.Server.ForceHTTP:
		log.Fatal("must have a server.certs.private_key and server.certs.certificate or letsencrypt.uri, or use force_http")
	}

	if crtL > 0 {
		cert, err := tls.LoadX509KeyPair(crt, key)
		log.OnErr(err).Fatalf("load tls certificate and private key: %v", err)

		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
	}

	if uriL > 0 {
		if len(config.Server.LetsEncrypt.Email) == 0 {
			log.Fatalf("letsencrypt needs an email")
		}

		u, err := url.Parse(uri)
		log.OnErr(err).Fatalf("letsencrypt uri: %v", err)

		var certStore autocert.Cache
		switch strings.ToLower(u.Scheme) {
		case "s3":
			certStore, err = s3cache.NewDSN(uri)
			log.OnErr(err).Fatalf("letsencrypt s3 cache: %v", err)
		case "file":
			certStore = autocert.DirCache(u.Path)
		default:
			log.Field("scheme", u.Scheme).Fatalf("letsencrypt invalid store scheme")
		}

		manager := &autocert.Manager{
			Email:      config.Server.LetsEncrypt.Email,
			Cache:      certStore,
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(config.Server.LetsEncrypt.DomainWhitelist...),
		}
		tlsConfig = &tls.Config{
			GetCertificate: manager.GetCertificate,
		}

		ws.http.Handle("/", manager.HTTPHandler(nil))
	}

	ws.https.server.TLSConfig = tlsConfig

	return ws
}
