package main

import (
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/njones/logger"
)

// server
const defaultEnvironment = "dev"
const defaultHTTPPort = ":80"
const defaultHTTPSPort = ":443"

// webServer
const defaultHealthcheckURL = "/.healthcheck"
const defaultPublicNSURL = "/pub/*"

// groupcache
const defaultGroupcacheReplicas = 50
const defaultGroupcacheBasePath = "/_groupcache/"
const defaultGroupcacheServer = "localhost"
const defaultGroupcacheCtxHeaderID = "Grp-Ctx-I"
const defaultGroupcacheCtxHeaderTS = "Grp-Ctx-T"
const defaultGroupcacheCtxHeaderKind = "Grp-Ctx-K"

// localDB
const defaultBucketName = "incrr"

// remoteDB
const defaultRevTimestampEpochDate = "01JAN2068"

// shutdowner is a function that will be called on a server shutdown
type shutdowner interface {
	Shutdown() error
}

// serverCerts holds the private key and cert for HTTPS
type serverCerts struct {
	PrivateKey  string `toml:"private_key"`
	Certificate string `toml:"certificate"`
}

// serverPorts for overiding the default ports
type serverPorts struct {
	HTTP  string `toml:"http"`
	HTTPS string `toml:"https"`
}

// serverURLs are the paths used for the router
type serverURLs struct {
	HealthcheckURL string `toml:"healthcheck"`
}

// serverSite is set up for the API website
type serverAPI struct {
	PublicNSURL string   `toml:"public_prefix"`
	Domains     []string `toml:"domains"`
}

// serverDatastores are the datastores
type serverDatastores struct {
	LocalDB  *localDB `toml:"local"`
	RemoteDB remoteDB

	UseRemoteDB   string                            `toml:"use_remote_db"`
	RemoteOptions map[string]map[string]interface{} `toml:"remote"`
}

// configuration holds all of the options that can
// be filled in by a TOML file
type configuration struct {
	Environment string `toml:"environment"`
	ShowConfig  bool   `toml:"show_config"` // show the config values on startup
	Server      struct {
		ForceHTTP bool        `toml:"force_http"`
		API       serverAPI   `toml:"api"`
		Certs     serverCerts `toml:"certs"`
		Ports     serverPorts `toml:"ports"`
		URLs      serverURLs  `toml:"urls"`

		LetsEncrypt struct {
			Email           string   `toml:"email"`
			CertificateURI  string   `toml:"crt_uri"`
			DomainWhitelist []string `toml:"whitelist"`
		} `toml:"letsencrypt"`
	} `toml:"server"`

	Web        *webServer
	Groupcache *groupcacheServer `toml:"groupcache"`
	Datastore  serverDatastores  `toml:"datastore"`

	internal struct {
		shutdown []shutdowner
		metadata toml.MetaData
	}
}

// leftpad is embedded...
func leftpad(num int, s, v string) string {
	n := num - len(s)
	if n <= 0 {
		n = 1
	}
	return s + strings.Repeat(" ", n) + v
}

// parseConfiguration takes a config file name and decodes the TOML
// to a configuration struct. Any errors are fatal errors
func parseConfiguration(r io.Reader) *configuration {
	config := &configuration{}

	metadata, err := toml.DecodeReader(r, config)
	if err != nil {
		log.Fatalf("config toml: %v", err)
	}

	for _, val := range metadata.Undecoded() {
		log.Printf("[config] un-decoded key: %v", val)
	}

	config.internal.metadata = metadata

	config.Web = setupWebServer(config)
	config.Groupcache = setupGroupcacheServer(config)
	config.Datastore.LocalDB = setupLocalDB(config)
	config.Datastore.RemoteDB = setupRemoteDB(config)

	setDefaults(config)

	config.Web.canServe = webCanServe(config)

	return config
}

// setDefaults sets any remaining defaults
func setDefaults(config *configuration) {
	if len(config.Environment) == 0 {
		config.Environment = defaultEnvironment
	}

	// Server
	if len(config.Server.URLs.HealthcheckURL) == 0 {
		config.Server.URLs.HealthcheckURL = defaultHealthcheckURL
	}
	if len(config.Server.API.PublicNSURL) == 0 {
		config.Server.API.PublicNSURL = defaultPublicNSURL
	}

	// WebServer
	config.Web.http.shutdownFunc = &sync.Once{}
	config.Web.http.shutdownChan = make(chan struct{})
	config.Web.https.shutdownFunc = &sync.Once{}
	config.Web.https.shutdownChan = make(chan struct{})

	// Groupcache
	// See the Groupcache object for default values

	config.Web.local = config.Datastore.LocalDB   // connect the localDB to the webServer for access
	config.Web.remote = config.Datastore.RemoteDB // connect the remoteDB to the webServer for access
}

func routeConfiguration(config *configuration) {
	config.Web.http.Get(config.Server.URLs.HealthcheckURL, config.Web.HealthcheckHandler)
	config.Web.https.With(config.Web.PublicNSHandlerCheck, config.Web.UseDomains(config.Server.API.Domains)).Get(config.Server.API.PublicNSURL, config.Web.PublicNSHandler)
	config.Web.https.Handle(config.Groupcache.internal.pattern, config.Groupcache)
}

// displayConfiguration displays all of the config information
func displayConfiguration(config *configuration) {
	if !config.ShowConfig {
		display = display.Suppress(logger.LevelPrint)
	}

	padd := 37
	log.Printf(leftpad(10, "ServerID:", "%v"), config.Web.serverID)                 // always print
	log.Printf(leftpad(10, "Version:", "%s (%s.%s)"), verSemVer, verHash, verBuild) // always print
	display.Printf(leftpad(padd, "[config] Environment:", "%v"), config.Environment)
	display.Printf(leftpad(padd, "[config] Force HTTP:", "%v"), config.Server.ForceHTTP)
	display.Printf(leftpad(padd, "[config] Api Domains:", "%v"), config.Server.API.Domains)
	display.Printf(leftpad(padd, "[config] Healthcheck URL:", "%v"), config.Server.URLs.HealthcheckURL)
	display.Printf(leftpad(padd, "[config] PublicNS URL:", "%v"), config.Server.API.PublicNSURL)

	if disp, ok := interface{}(config.Groupcache).(configDisplay); ok {
		disp.configDisplay(padd, config)
	}
	if disp, ok := interface{}(config.Datastore.LocalDB).(configDisplay); ok {
		disp.configDisplay(padd, config)
	}
	if disp, ok := config.Datastore.RemoteDB.(configDisplay); ok {
		disp.configDisplay(padd, config)
	}
}

// webCanServe determines if the server is ready to start initially serving traffic
func webCanServe(config *configuration) bool {
	if config.Datastore.LocalDB.db == nil {
		return false
	}
	return true
}

// parseFileDSN is simple parsing for file schemas
// TODO(njones): make this better than it is currently
func parseFileDSN(s string) (rtn struct{ filename string }, err error) {
	uri, err := url.Parse(s)
	if err != nil {
		return rtn, err
	}
	if uri.Scheme != "file" {
		return rtn, fmt.Errorf("must use the file:// scheme")
	}

	rtn.filename = uri.Path
	return rtn, nil
}

// parseFileDSN is simple parsing for s3 schemas
// TODO(njones): make this better than it is currently
func parseS3DSN(dsn string) (rtn struct{ region, bucket, prefix string }, err error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return rtn, err
	}

	hparts := strings.Split(u.Hostname(), ".")

	if len(hparts) < 3 {
		return rtn, fmt.Errorf("the hostname doesn't appear to be correct")
	}

	// check for region
	switch regn := hparts[len(hparts)-3]; regn {
	case "s3":
		rtn.region = "us-east-1"
	default:
		if len(regn) > 3 && regn[:3] == "s3-" {
			rtn.region = regn[3:]
		} else {
			return rtn, fmt.Errorf("the region doesn't appear to be correct")
		}
	}

	// check for bucket name in domain
	if len(hparts[:len(hparts)-2]) > 1 {
		rtn.bucket = strings.Join(hparts[:len(hparts)-3], ".")
	}

	rtn.prefix = u.Path
	if len(rtn.bucket) == 0 {
		slash := strings.Index(rtn.prefix[1:], "/")
		rtn.bucket = (rtn.prefix[1:])[:slash]
		rtn.prefix = rtn.prefix[slash+1:]
	}
	return
}
