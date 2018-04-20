package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"reflect"

	"github.com/go-chi/chi"
	"github.com/njones/logger"
)

var log = logger.New()
var display = logger.New().Color(logger.ColorCyan)

func ListenForSignals(config *configuration) {
	ctrl := make(chan os.Signal, 1)
	signal.Notify(ctrl, os.Interrupt)

	for {
		select {
		case <-ctrl:
			config.Web.http.shutdownFunc.Do(func() {
				if err := config.Web.http.server.Shutdown(context.Background()); err != nil {
					log.Printf("HTTP server Shutdown: %v", err)
				}
				close(config.Web.http.shutdownChan)
			})

			if !config.Server.ForceHTTP {
				config.Web.https.shutdownFunc.Do(func() {
					if err := config.Web.https.server.Shutdown(context.Background()); err != nil {
						log.Printf("HTTPS server Shutdown: %v", err)
					}
					close(config.Web.https.shutdownChan)
				})
			}
		}
	}
}

// ListenAndServe starts http and https servers
func ListenAndServe(config *configuration) (err error) {

	// if we are going to use HTTP only, then move all of the handlers and middleware
	// from HTTPS to HTTP
	if config.Server.ForceHTTP {
		chi.Walk(config.Web.https, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
			if len(middlewares) > 0 {

				// de-dup middlewares, using reflect... but this is on startup
				// so it's not super critical for speed...
				mm := make(map[uintptr]struct{})
				for _, m := range config.Web.http.Middlewares() {
					mm[reflect.ValueOf(m).Pointer()] = struct{}{} // just get the pointer string hex
				}
				var ms []func(http.Handler) http.Handler
				for _, m2 := range middlewares {
					if _, ok := mm[reflect.ValueOf(m2).Pointer()]; !ok {
						ms = append(ms, m2)
					}
				}

				config.Web.http.With(ms...).Method(method, route, handler)
				return nil
			}
			config.Web.http.Method(method, route, handler)
			return nil
		})
	}

	// if we start more than one server, then accept an error from either to
	// wrap, return and exit, so we start the servers in a go routine
	errs := make(chan error)
	go func(e chan error) {
		log.Println("serving...", logger.KV("scheme", "http"), logger.KV("port", config.Web.http.server.Addr))
		err := config.Web.http.server.ListenAndServe()
		if err != http.ErrServerClosed {
			config.Web.https.shutdownFunc.Do(func() {
				if err := config.Web.https.server.Shutdown(context.Background()); err != nil {
					log.Printf("HTTPS server Shutdown: %v", err)
				}
				close(config.Web.https.shutdownChan)
			})
			e <- err // send back an error if we're not shutting down
			return
		}
		<-config.Web.http.shutdownChan
		e <- nil
	}(errs)

	if !config.Server.ForceHTTP {
		go func(e chan error) {
			log.Println("serving...",
				logger.KV("scheme", "https"),
				logger.KV("port", config.Web.http.server.Addr),
			)
			err := config.Web.https.server.ListenAndServe()
			if err != http.ErrServerClosed {
				config.Web.http.shutdownFunc.Do(func() {
					if err := config.Web.https.server.Shutdown(context.Background()); err != nil {
						log.Printf("HTTPS server Shutdown: %v", err)
					}
					close(config.Web.https.shutdownChan)
				})
				e <- err // send back an error if we're not shutting down
				return
			}
			<-config.Web.https.shutdownChan
			e <- nil
		}(errs)
	}

	//TODO(njones): Grab both errors and wrap them together, right now we just
	// return the first one that wins
	return <-errs
}

// Main returns an error that can be caught by main()
func Main(filename string) error {
	f, err := os.Open(filename)
	log.OnErr(err).Fatalf("file open: %v", err)

	config := parseConfiguration(f)
	displayConfiguration(config)
	routeConfiguration(config)

	// Setup Signal handling
	go ListenForSignals(config)

	err = ListenAndServe(config)
	for _, service := range config.internal.shutdown {
		e := service.Shutdown()
		if err != nil {
			err = fmt.Errorf("%v: %v", e, err)
		}
	}
	return err
}

func main() {
	var configfile = flag.String("config", "config.toml", "the config toml location")

	flag.Parse()

	if err := Main(*configfile); err != nil {
		log.Fatal(err)
	}
}
