package main

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/golang/groupcache"
)

// groupcacheServer holds the groupcache configuration information
type groupcacheServer struct {
	Server   string   `toml:"self"`
	Replicas int      `toml:"replicas"`
	Pool     []string `toml:"http_pool"`
	BasePath string   `toml:"base_path"`

	Header struct {
		ID        string `toml:"id"`
		Timestamp string `toml:"ts"`
		Kind      string
	} `toml:"header"`

	*groupcache.HTTPPool

	internal struct {
		pattern string
		scheme  string
		self    string
	}
}

func (gs *groupcacheServer) configDisplay(padd int, config *configuration) {
	display.Printf(leftpad(padd, "[config] Groupcache URL:", "%v"), gs.internal.pattern)
	display.Printf(leftpad(padd, "[config] Groupcache Scheme:", "%v"), gs.internal.scheme)
	display.Printf(leftpad(padd, "[config] Groupcache Self:", "%v"), gs.internal.self)
	display.Printf(leftpad(padd, "[config] Groupcache Replicas:", "%v"), gs.Replicas)
	display.Printf(leftpad(padd, "[config] Groupcache Pool:", "%v"), gs.Pool)
	display.Printf(leftpad(padd, "[config] Groupcache BasePath:", "%v"), gs.BasePath)
	display.Printf(leftpad(padd, "[config] Groupcache Ctx Header ID:", "%v"), gs.Header.ID)
	display.Printf(leftpad(padd, "[config] Groupcache Ctx Header Ts:", "%v"), gs.Header.Timestamp)
}

// contextResponder is the interface to return context data
type contextResponder interface {
	Data() *respContext
	Response() contextResponder
}

// respContext is the context used for normal groupcache responses
type respContext struct {
	ServerID  string `json:"id"`
	Timestamp string `json:"ts"`
	Number    string `json:"#" sub:"%s"` // a string because JSON doesn't support unit64
}

// Meta returns a JSON string of data using a custom
// json marshaler so it can add a string to be
// filled in later
func (rc respContext) Meta() string {
	type subsitute string
	v := reflect.ValueOf(&rc).Elem()

	var out = make([]string, v.NumField())
	for i := 0; i < v.NumField(); i++ {
		vf, tf := v.Field(i), v.Type().Field(i)
		val := vf.Interface()
		tag := tf.Tag.Get("json")
		sub := tf.Tag.Get("sub")
		if len(sub) > 0 {
			val = subsitute(sub)
		}
		switch val.(type) {
		case int64:
			out[i] = fmt.Sprintf(`"%s":%d`, tag, val)
		case subsitute:
			out[i] = fmt.Sprintf(`"%s":"%s"`, tag, val)
		default:
			out[i] = fmt.Sprintf(`"%s":%#v`, tag, val)
		}
	}
	return "{" + strings.Join(out, ", ") + "}"
}

// Equal does a comparison of values and returns a bool. It
// is used to see if values back from the Groupcache response
// are the same that were sent in the context
func (rc *respContext) Equal(id, ts string) bool { return rc.ServerID == id && rc.Timestamp == ts }

// Response unwraps the response given back and returns the
// minimal object that the JSON satisfies
func (rc *respContext) Response() contextResponder { return rc }

// Data returns a concrete struct to get values out of
func (rc *respContext) Data() *respContext { return rc }

// remoteContext is the context used for groupcache responses that need to lookup remote values
type remoteContext struct {
	*respContext
	Skip *string `json:">"`
}

// Meta returns the JSON string of the object with the proper values
// to be filled in
func (rc remoteContext) Meta() string {
	meta := rc.respContext.Meta()
	return fmt.Sprintf(meta[:len(meta)-2], `%s", ">":"%s"}`)
}

// Response unwraps the response given back and returns the
// minimal object that the JSON satisfies
func (rc *remoteContext) Response() contextResponder {
	if rc.Skip == nil {
		return rc.respContext
	}
	return rc
}

// SkipTo parses the skip
func (rc *remoteContext) SkipTo() (uint64, error) {
	if rc.Skip == nil {
		return 0, errSkipNilValue
	}

	skipTo, err := strconv.ParseUint(*rc.Skip, 10, 64)
	if err != nil {
		return 0, err
	}
	return skipTo, nil
}

// groupcacheRT is the RoundTripper and it's context values
type groupcacheRT map[string]string

// RoundTrip satisfies the RoundTriper interface, and adds the context values as a header to the HTTP request
func (rt groupcacheRT) RoundTrip(r *http.Request) (w *http.Response, err error) {
	for k, v := range rt {
		r.Header.Add(k, v)
	}
	return http.DefaultTransport.RoundTrip(r)
}

// setupGroupcacheServer sets up the groupcache server, some defaults need
// to be set before the server can be setup
func setupGroupcacheServer(config *configuration) *groupcacheServer {
	gcache := config.Groupcache
	if gcache == nil {
		gcache = new(groupcacheServer)
	}

	if gcache.Replicas == 0 {
		gcache.Replicas = defaultGroupcacheReplicas
	}

	if len(gcache.Header.ID) == 0 {
		gcache.Header.ID = defaultGroupcacheCtxHeaderID
	}

	if len(gcache.Header.Timestamp) == 0 {
		gcache.Header.Timestamp = defaultGroupcacheCtxHeaderTS
	}

	if len(gcache.Header.Kind) == 0 {
		gcache.Header.Kind = defaultGroupcacheCtxHeaderKind
	}

	if gcache.Replicas < 30 {
		log.Warnf("groupcache replicas set at %d, but should be about 50 or above", gcache.Replicas)
	}

	if len(gcache.BasePath) == 0 {
		gcache.BasePath = defaultGroupcacheBasePath
	}
	gcache.internal.pattern = strings.TrimRight(gcache.BasePath, "/*") + "/*"
	// This is the heart of the application. It's pretty simple, until you hit
	// some edge cases, which happens with a lot of things in programming.
	//
	// The basic use case is: A server does a `groupcache.Get` with a key that
	// is composed of a namespace and a number that it would like to claim,
	// a context with the servers ID and a timestamp. If the response back from
	// Groupcache is the same ServerID, Timestamp and Namespace/Number value,
	// then the calling server can claim the number and respond with it. If the
	// values all don't match then the server will increment the number and try
	// again until the values match.
	//
	// This should work for most cases, but there are three times when
	// additional actions may need to take place to find the next incrementing
	// number.
	//
	// 1. There is a race condition, it seems like the server can't get a new
	//    number because another server always seems to claim the next
	//    incrementing number.
	// 2. There is a key eviction from Groupcache because the key is too old
	// 3. A server(s) leaves or joins and doesn't have the key yet or
	//    replicated for some reason.
	//
	// To handle case one, we pass in a reservation context a few iterations
	// ahead, The reserved context will block Groupcache output for a key until
	// either the key is satisfied before, or it hits the reserved key, If
	// a key is found before the reservation, then a pointer to the found key
	// is sent to the waiting keys, and they start from that location looking
	// for new incrementing numbers.
	config.Web.cache = groupcache.NewGroup("incr", 64<<30, groupcache.GetterFunc(func(ctxi groupcache.Context, key string, dest groupcache.Sink) error {
		var resp string

		kk := strings.Split(key, ":")
		keyNo, keyNS := kk[0], strings.Join(kk[1:], ":")

		switch ctx := ctxi.(type) {
		case *respContext:
			resp = ctx.Meta()
		case *remoteContext:
			valB, err := config.Datastore.RemoteDB.Get([]byte(keyNS))
			if err != nil {
				return fmt.Errorf("[groupcache]:grp:remote rem64 get: %v", err)
			}
			valStr := string(valB)
			if len(valStr) > 0 {
				rem64, err := strconv.ParseUint(valStr, 10, 64)
				if err != nil {
					return fmt.Errorf("[groupcache]:grp:remote rem64 parse: %v", err)
				}

				key64, err := strconv.ParseUint(keyNo, 10, 64)
				if err != nil {
					return fmt.Errorf("[groupcache]:grp:remote key64 parse: %v", err)
				}

				if rem64 > key64 {
					return dest.SetString(fmt.Sprintf(ctx.Meta(), keyNo, strconv.FormatUint(rem64+1, 10)))
				}
			}
			resp = ctx.respContext.Meta()
		}

		if err := config.Datastore.LocalDB.Incr([]byte(keyNS), []byte(keyNo)); err != nil { // save the key locally, since we're handling it
			return fmt.Errorf("local: %v", err)
		}

		if err := config.Datastore.RemoteDB.Set([]byte(keyNS), []byte(keyNo)); err != nil { // save the key remotely
			return fmt.Errorf("remote: %v", err)
		}

		return dest.SetString(fmt.Sprintf(resp, keyNo))
	},
	))

	if len(gcache.Server) == 0 {
		addrs, err := publicAddresses()
		log.OnErr(err).Warnf("finding public address: %v", err)
		for _, addr := range addrs {
			if addr.name != "localhost" {
				gcache.Server = addr.ip.String()
				break
			}
		}
		if len(gcache.Server) == 0 {
			gcache.Server = defaultGroupcacheServer
		}
	}

	httpp := strings.Split(config.Server.Ports.HTTP, ":")
	if len(httpp) != 2 {
		log.Fatal(`The config http port is invalid. Should be like ":80"`)
	}
	httpsp := strings.Split(config.Server.Ports.HTTPS, ":")
	if len(httpsp) != 2 {
		log.Fatal(`The config https port is invalid. Should be like ":443"`)
	}

	if len(gcache.internal.scheme) == 0 {
		gcache.internal.scheme = "https"
		if config.Server.ForceHTTP {
			gcache.internal.scheme = "http"
		}
	}
	// use https port unless it's known to be http
	var port = httpsp[1]
	if gcache.internal.scheme == "http" {
		port = httpp[1]
	}
	gcache.internal.self = fmt.Sprintf("%s://%s:%s", gcache.internal.scheme, gcache.Server, port)

	opts := &groupcache.HTTPPoolOptions{BasePath: gcache.BasePath, Replicas: config.Groupcache.Replicas}
	gcache.HTTPPool = groupcache.NewHTTPPoolOpts(gcache.internal.self, opts)
	gcache.Transport = func(ctx groupcache.Context) http.RoundTripper {
		var id, ts, kind string
		switch c := ctx.(type) {
		case *respContext:
			id, ts, kind = c.ServerID, c.Timestamp, "local"
		case *remoteContext:
			id, ts, kind = c.ServerID, c.Timestamp, "remote"
		}

		return groupcacheRT{
			gcache.Header.ID:        id,
			gcache.Header.Timestamp: ts,
			gcache.Header.Kind:      kind,
		}
	}
	gcache.Context = func(r *http.Request) groupcache.Context {
		var respCtx interface{}
		rc := &respContext{
			ServerID:  r.Header.Get(gcache.Header.ID),
			Timestamp: r.Header.Get(gcache.Header.Timestamp),
		}

		switch r.Header.Get(gcache.Header.Kind) {
		case "local":
			respCtx = rc
		case "remote":
			respCtx = &remoteContext{respContext: rc}
		}

		return groupcache.Context(respCtx)
	}

	gcache.Set(gcache.Pool...) // the initial set

	return gcache
}
