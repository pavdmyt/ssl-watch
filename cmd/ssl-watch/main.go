package main

import (
	"crypto/tls"
	"github.com/gorilla/mux"
	"net"
	"net/http"
	"time"
)

func processDomain(app *App, domain string, ips []net.IP) Endpoints {

	host, port, err := net.SplitHostPort(domain)
	if err != nil {
		host = domain
		port = "443"
	}

	if len(ips) == 0 {
		ips = resolveDomain(app, host, app.config.LookupTimeout)
	}
	endpoints := Endpoints{}

	for _, ip := range ips {

		dialer := net.Dialer{Timeout: app.config.ConnectionTimeout, Deadline: time.Now().Add(app.config.ConnectionTimeout + 5*time.Second)}

		if IsIPv4(ip.String()) {

			endpoint := Endpoint{}
			connection, err := tls.DialWithDialer(&dialer, "tcp", ip.String()+":"+port, &tls.Config{ServerName: host, InsecureSkipVerify: true})
			if err != nil {
				app.log.Error(ip.String(), err)
				endpoint.alive = false
				endpoints[ip.String()] = endpoint
				continue
			}

			cert := connection.ConnectionState().PeerCertificates[0]
			endpoint.alive = true
			endpoint.expiry = cert.NotAfter
			endpoint.CN = cert.Subject.CommonName
			endpoint.AltNamesCount = len(cert.DNSNames)
			err = cert.VerifyHostname(host)
			if err != nil {
				endpoint.valid = false
			} else {
				endpoint.valid = true
			}
			connection.Close()
			endpoints[ip.String()] = endpoint
		}
	}
	return endpoints

}

func main() {

	app := NewApp()

	go func() {

		for domain, ips := range app.Domains {

			eps := processDomain(app, domain, StrToIp(ips))
			app.metrics.Set(domain, eps)

		}
		time.Sleep(app.config.ScrapeInterval)
	}()

	app.log.Info("config file is set to be at " + app.config.ConfigFile)
	app.log.Info("scrape interval is " + app.config.ScrapeInterval.String())
	app.log.Info("connection timeout is " + app.config.ConnectionTimeout.String())
	app.log.Info("lookup timeout is " + app.config.LookupTimeout.String())
	app.log.Info("starting http server on port " + app.config.Port)

	rtr := mux.NewRouter()
	rtr.HandleFunc("/metrics", app.ShowMetrics).Methods("GET")
	http.Handle("/", rtr)
	app.log.Fatal("http server stopped", http.ListenAndServe(":"+app.config.Port, nil))

}
