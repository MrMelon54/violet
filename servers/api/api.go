package api

import (
	"encoding/json"
	"github.com/MrMelon54/mjwt"
	"github.com/MrMelon54/mjwt/claims"
	"github.com/MrMelon54/violet/servers/conf"
	"github.com/MrMelon54/violet/utils"
	"github.com/julienschmidt/httprouter"
	"net/http"
	"time"
)

// NewApiServer creates and runs a http server containing all the API
// endpoints for the software
//
// `/compile` - reloads all domains, routes and redirects
func NewApiServer(conf *conf.Conf, compileTarget utils.MultiCompilable) *http.Server {
	r := httprouter.New()

	// Endpoint for compile action
	r.POST("/compile", checkAuthWithPerm(conf.Signer, "violet:compile", func(rw http.ResponseWriter, req *http.Request, _ httprouter.Params, b AuthClaims) {
		// Trigger the compile action
		compileTarget.Compile()
		rw.WriteHeader(http.StatusAccepted)
	}))

	// Endpoint for domains
	domainFunc := domainManage(conf.Signer, conf.Domains)
	r.PUT("/domain/:domain", domainFunc)
	r.DELETE("/domain/:domain", domainFunc)

	// Endpoint code for target routes/redirects
	targetApis := SetupTargetApis(conf.Signer, conf.Router)

	// Endpoint for routes
	r.GET("/route", checkAuthWithPerm(conf.Signer, "violet:route", func(rw http.ResponseWriter, req *http.Request, params httprouter.Params, b AuthClaims) {
		routes, active, err := conf.Router.GetAllRoutes()
		if err != nil {
			apiError(rw, http.StatusInternalServerError, "Failed to get routes from database")
			return
		}
		rw.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(rw).Encode(map[string]any{
			"routes": routes,
			"active": active,
		})
	}))
	r.POST("/route", targetApis.CreateRoute)
	r.DELETE("/route", targetApis.DeleteRoute)

	// Endpoint for redirects
	r.GET("/redirect", checkAuthWithPerm(conf.Signer, "violet:redirect", func(rw http.ResponseWriter, req *http.Request, params httprouter.Params, b AuthClaims) {
		redirects, active, err := conf.Router.GetAllRedirects()
		if err != nil {
			apiError(rw, http.StatusInternalServerError, "Failed to get redirects from database")
			return
		}
		rw.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(rw).Encode(map[string]any{
			"redirects": redirects,
			"active":    active,
		})
	}))
	r.POST("/redirect", targetApis.CreateRedirect)
	r.DELETE("/redirect", targetApis.DeleteRedirect)

	// Endpoint for acme-challenge
	acmeChallengeFunc := acmeChallengeManage(conf.Signer, conf.Domains, conf.Acme)
	r.PUT("/acme-challenge/:domain/:key/:value", acmeChallengeFunc)
	r.DELETE("/acme-challenge/:domain/:key", acmeChallengeFunc)

	// Create and run http server
	return &http.Server{
		Addr:              conf.ApiListen,
		Handler:           r,
		ReadTimeout:       time.Minute,
		ReadHeaderTimeout: time.Minute,
		WriteTimeout:      time.Minute,
		IdleTimeout:       time.Minute,
		MaxHeaderBytes:    2500,
	}
}

// apiError outputs a generic JSON error message
func apiError(rw http.ResponseWriter, code int, m string) {
	rw.WriteHeader(code)
	_ = json.NewEncoder(rw).Encode(map[string]string{
		"error": m,
	})
}

func domainManage(verify mjwt.Verifier, domains utils.DomainProvider) httprouter.Handle {
	return checkAuthWithPerm(verify, "violet:domains", func(rw http.ResponseWriter, req *http.Request, params httprouter.Params, b AuthClaims) {
		// add domain with active state
		domains.Put(params.ByName("domain"), req.Method == http.MethodPut)
		domains.Compile()
	})
}

func acmeChallengeManage(verify mjwt.Verifier, domains utils.DomainProvider, acme utils.AcmeChallengeProvider) httprouter.Handle {
	return checkAuthWithPerm(verify, "violet:acme-challenge", func(rw http.ResponseWriter, req *http.Request, params httprouter.Params, b AuthClaims) {
		domain := params.ByName("domain")
		if !domains.IsValid(domain) {
			utils.RespondVioletError(rw, http.StatusBadRequest, "Invalid ACME challenge domain")
			return
		}
		if req.Method == http.MethodPut {
			acme.Put(domain, params.ByName("key"), params.ByName("value"))
		} else {
			acme.Delete(domain, params.ByName("key"))
		}
		rw.WriteHeader(http.StatusAccepted)
	})
}

// validateDomainOwnershipClaims validates if the claims contain the
// `owns=<fqdn>` field with the matching top level domain
func validateDomainOwnershipClaims(a string, perms *claims.PermStorage) bool {
	if fqdn, ok := utils.GetTopFqdn(a); ok {
		if perms.Has("owns=" + fqdn) {
			return true
		}
	}
	return false
}
