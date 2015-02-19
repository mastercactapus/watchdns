package main

import (
	"github.com/coreos/fleet/etcd"
	"github.com/coreos/fleet/registry"
	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ServiceRegistry struct {
	Options  RegistryOptions
	etcd     *etcd.Client
	registry *registry.EtcdRegistry
	endCh    chan bool
	queryCh  chan Query
	hRateCh  chan bool
	domain   string
	running  bool
}

type RegistryOptions struct {
	Domain          string
	CheckResolution time.Duration
	FleetInterval   time.Duration
	CheckInterval   time.Duration
	CheckTimeout    time.Duration
	CheckConcurrent int
}

type Query struct {
	Question dns.Question
	Answer   chan []dns.RR
}

type HealthCheckResult struct {
	UnitId  string
	CheckId string
	Result  bool
}

func NewServiceRegistry(etcdPeers []string, prefix, string, timeout time.Duration, options *RegistryOptions) (*ServiceRegistry, error) {
	log.Debugln("Using etcd peers:", peers)
	cli, err := etcd.NewClient(etcdPeers, http.DefaultTransport.(*http.Transport), timeout)
	if err != nil {
		return nil, err
	}
	s := new(ServiceRegistry)
	s.etcd = cli
	s.Options = *options
	log.Debugln("Using fleet prefix:", prefix)
	s.registry = registry.NewEtcdRegistry(cli, prefix)
	return s, nil
}

//Start monitoring services
func (r *ServiceRegistry) Start() {
	r.hRateCh = make(chan bool, r.Options.CheckConcurrent)
	r.endCh = make(chan bool)
	r.queryCh = make(chan Query, 100)
	go r.mainLoop(r.endCh)
	<-r.endCh
	r.running = true
}

//Stop monitoring services
func (r *ServiceRegistry) Stop() {
	r.running = false
	r.endCh <- true
}

func (r *ServiceRegistry) mainLoop(endCh chan bool, queryCh chan Query) {
	fleetCh := time.NewTicker(r.Options.FleetInterval)
	healthCh := time.NewTicker(r.Options.CheckResolution)
	healthResultsCh := make(chan HealthCheckResult, 100)
	r.reloadFleet()
	endCh <- true //signal that we finished the initial reload
	for {
		select {
		case <-endCh:
			fleetCh.Stop()
			healthCh.Stop()
			break
		case <-fleetCh:
			r.reloadFleet()
		case <-healthCh:
			r.doHealthChecks()
		case result := <-healthResultsCh:

		case query := <-queryCh:

		}
	}
}

func (r *ServiceRegistry) reloadFleet() {
	//new services should be set with random offset for check times (so not all checks run at once)
	r.registry.UnitStates()
}

// doHealthChecks fires off all pending health checks (with expired timers)
// and returns the result to the healthCheckResult channel for processing
// hRateCh is used to limit the actual rates in the individual goroutines
// this is so that while health checks are running/pending we can process
// queries and other things
func (r *ServiceRegistry) doHealthChecks(results chan HealthCheckResult) {

}

func (r *ServiceRegistry) checkHttp(url string) bool {
	<-r.hRateCh
	cli := http.Client{Timeout: r.Options.CheckTimeout}
	resp, err := cli.Get(url)
	if err != nil {
		<-r.hRateCh
		return false
	}
	ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode/100 == 2 {
		<-r.hRateCh
		return true
	} else {
		<-r.hRateCh
		return false
	}
}
func (r *ServiceRegistry) checkTcp(address string) bool {
	r.hRateCh <- true
	conn, err := net.DialTimeout("tcp", address, r.Options.CheckTimeout)
	if err != nil {
		<-r.hRateCh
		return false
	}
	conn.SetLinger(0)
	conn.Close()
	<-r.hRateCh
	return true
}

func (r *ServiceRegistry) LookupA(query string) []dns.A {
	if !strings.HasSuffix(query, ".service."+r.domain) {
		return nil
	}

}
func (r *ServiceRegistry) LookupSrv(query string) []dns.SRV {

}
