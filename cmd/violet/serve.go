package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/MrMelon54/mjwt"
	"github.com/MrMelon54/violet/certs"
	"github.com/MrMelon54/violet/domains"
	errorPages "github.com/MrMelon54/violet/error-pages"
	"github.com/MrMelon54/violet/favicons"
	"github.com/MrMelon54/violet/proxy"
	"github.com/MrMelon54/violet/router"
	"github.com/MrMelon54/violet/servers"
	"github.com/MrMelon54/violet/servers/api"
	"github.com/MrMelon54/violet/servers/conf"
	"github.com/MrMelon54/violet/utils"
	"github.com/google/subcommands"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

type serveCmd struct{ configPath string }

func (s *serveCmd) Name() string     { return "serve" }
func (s *serveCmd) Synopsis() string { return "Serve reverse proxy server" }
func (s *serveCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&s.configPath, "conf", "", "/path/to/config.json : path to the config file")
}
func (s *serveCmd) Usage() string {
	return `serve [-conf <config file>]
  Serve reverse proxy server using information from config file
`
}

func (s *serveCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	log.Println("[Violet] Starting...")

	if s.configPath == "" {
		log.Println("[Violet] Error: config flag is missing")
		return subcommands.ExitUsageError
	}

	openConf, err := os.Open(s.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("[Violet] Error: missing config file")
		} else {
			log.Println("[Violet] Error: open config file: ", err)
		}
		return subcommands.ExitFailure
	}

	var conf startUpConfig
	err = json.NewDecoder(openConf).Decode(&conf)
	if err != nil {
		log.Println("[Violet] Error: invalid config file: ", err)
		return subcommands.ExitFailure
	}

	// working directory is the parent of the config file
	wd := filepath.Dir(s.configPath)
	normalLoad(conf, wd)
	return subcommands.ExitSuccess
}

func normalLoad(startUp startUpConfig, wd string) {
	// the cert and key paths are useless in self-signed mode
	if !startUp.SelfSigned {
		// create path to cert dir
		err := os.MkdirAll(filepath.Join(wd, "certs"), os.ModePerm)
		if err != nil {
			log.Fatal("[Violet] Failed to create certificate path")
		}
		// create path to key dir
		err = os.MkdirAll(filepath.Join(wd, "keys"), os.ModePerm)
		if err != nil {
			log.Fatal("[Violet] Failed to create certificate key path")
		}
	}

	// errorPageDir stores an FS interface for accessing the error page directory
	var errorPageDir fs.FS
	if startUp.ErrorPagePath != "" {
		errorPageDir = os.DirFS(startUp.ErrorPagePath)
		err := os.MkdirAll(startUp.ErrorPagePath, os.ModePerm)
		if err != nil {
			log.Fatalf("[Violet] Failed to create error page path '%s'", startUp.ErrorPagePath)
		}
	}

	// load the MJWT RSA public key from a pem encoded file
	mJwtVerify, err := mjwt.NewMJwtVerifierFromFile(filepath.Join(wd, "signer.public.pem"))
	if err != nil {
		log.Fatalf("[Violet] Failed to load MJWT verifier public key from file '%s': %s", filepath.Join(wd, "signer.public.pem"), err)
	}

	// open sqlite database
	db, err := sql.Open("sqlite3", filepath.Join(wd, "violet.db.sqlite"))
	if err != nil {
		log.Fatal("[Violet] Failed to open database")
	}

	certDir := os.DirFS(filepath.Join(wd, "certs"))
	keyDir := os.DirFS(filepath.Join(wd, "keys"))

	allowedDomains := domains.New(db)                              // load allowed domains
	acmeChallenges := utils.NewAcmeChallenge()                     // load acme challenge store
	allowedCerts := certs.New(certDir, keyDir, startUp.SelfSigned) // load certificate manager
	hybridTransport := proxy.NewHybridTransport()                  // load reverse proxy
	dynamicFavicons := favicons.New(db, startUp.InkscapeCmd)       // load dynamic favicon provider
	dynamicErrorPages := errorPages.New(errorPageDir)              // load dynamic error page provider
	dynamicRouter := router.NewManager(db, hybridTransport)        // load dynamic router manager

	// struct containing config for the http servers
	srvConf := &conf.Conf{
		ApiListen:   startUp.Listen.Api,
		HttpListen:  startUp.Listen.Http,
		HttpsListen: startUp.Listen.Https,
		RateLimit:   startUp.RateLimit,
		DB:          db,
		Domains:     allowedDomains,
		Acme:        acmeChallenges,
		Certs:       allowedCerts,
		Favicons:    dynamicFavicons,
		Signer:      mJwtVerify,
		ErrorPages:  dynamicErrorPages,
		Router:      dynamicRouter,
	}

	// create the compilable list and run a first time compile
	allCompilables := utils.MultiCompilable{allowedDomains, allowedCerts, dynamicFavicons, dynamicErrorPages, dynamicRouter}
	allCompilables.Compile()

	var srvApi, srvHttp, srvHttps *http.Server
	if srvConf.ApiListen != "" {
		srvApi = api.NewApiServer(srvConf, allCompilables)
		log.Printf("[API] Starting API server on: '%s'\n", srvApi.Addr)
		go utils.RunBackgroundHttp("API", srvApi)
	}
	if srvConf.HttpListen != "" {
		srvHttp = servers.NewHttpServer(srvConf)
		log.Printf("[HTTP] Starting HTTP server on: '%s'\n", srvHttp.Addr)
		go utils.RunBackgroundHttp("HTTP", srvHttp)
	}
	if srvConf.HttpsListen != "" {
		srvHttps = servers.NewHttpsServer(srvConf)
		log.Printf("[HTTPS] Starting HTTPS server on: '%s'\n", srvHttps.Addr)
		go utils.RunBackgroundHttps("HTTPS", srvHttps)
	}

	// Wait for exit signal
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
	fmt.Println()

	// Stop servers
	log.Printf("[Violet] Stopping...")
	n := time.Now()

	// close http servers
	if srvApi != nil {
		srvApi.Close()
	}
	if srvHttp != nil {
		srvHttp.Close()
	}
	if srvHttps != nil {
		srvHttps.Close()
	}

	log.Printf("[Violet] Took '%s' to shutdown\n", time.Now().Sub(n))
	log.Println("[Violet] Goodbye")
}
