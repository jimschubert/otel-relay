package proxy

import (
	"errors"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	relay "github.com/jimschubert/otel-relay/inspector"
)

type HTTPProxy struct {
	listenAddr   string
	upstreamAddr string
	server       *http.Server
	inspector    *relay.Inspector
	serveErr     chan error
	doneChan     chan struct{}
}

func NewHTTPProxy(listenAddr, upstreamAddr string, insp *relay.Inspector) *HTTPProxy {
	proxy := &HTTPProxy{
		listenAddr:   listenAddr,
		upstreamAddr: upstreamAddr,
		inspector:    insp,
		serveErr:     make(chan error, 1),
		doneChan:     make(chan struct{}),
	}

	return proxy
}

func (p *HTTPProxy) Protocol() string {
	return "http"
}

func (p *HTTPProxy) Start() error {
	var reverseProxy *httputil.ReverseProxy
	if p.upstreamAddr != "" {
		upstreamURL, err := url.Parse(p.upstreamAddr)
		if err != nil {
			log.Printf("Error parsing upstream URL: %v", err)
		}

		reverseProxy = httputil.NewSingleHostReverseProxy(upstreamURL)
	}

	p.server = &http.Server{
		Addr: p.listenAddr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p.inspector.InspectHttpRequest(r)
			if reverseProxy == nil {
				w.WriteHeader(http.StatusOK)
				return
			}
			reverseProxy.ServeHTTP(w, r)
		}),
	}

	go func() {
		if err := p.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			p.serveErr <- err
		}
		close(p.serveErr)
	}()

	return nil
}

func (p *HTTPProxy) Err() error {
	if p.serveErr == nil {
		return nil
	}
	return <-p.serveErr
}

func (p *HTTPProxy) Stop() error {
	var err error
	if p.server != nil {
		err = p.server.Close()
	}
	return err
}
