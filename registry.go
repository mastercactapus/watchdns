package main

import (
	"errors"
	"github.com/coreos/fleet/etcd"
	"github.com/coreos/fleet/registry"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net"
	"net/http"
	"time"
)

type ServiceRegistry struct {
	Options    RegistryOptions
	etcd       etcd.Client
	registry   *registry.EtcdRegistry
	endCh      chan bool
	queryACh   chan QueryA
	querySrvCh chan QuerySrv
	hRateCh    chan bool
	domain     string
	running    bool
	units      map[string]*ServiceEntry
	lookup     map[string][]*ServiceEntry
}

type RegistryOptions struct {
	Domain          string
	CheckResolution time.Duration
	FleetInterval   time.Duration
	CheckInterval   time.Duration
	CheckTimeout    time.Duration
	CheckConcurrent int
}

type ServiceEntry struct {
	LastFleetCheck  time.Time
	LastHealthCheck time.Time
	UnitHash        string
	ServiceOption
	ServerAddress       net.IP
	PendingHealthChecks int
	FailedHealthChecks  int
	Online              bool
	Running             bool
}

type QuerySrv struct {
	AnswerCh chan []AnswerSrv
	Name     string
	Service  string
	Protocol string
}
type QueryA struct {
	AnswerCh chan []AnswerA
	Name     string
}
type AnswerSrv struct {
	Server net.IP
	SrvOption
	Ttl time.Duration
}
type AnswerA struct {
	Server net.IP
	Ttl    time.Duration
}

type HealthCheckResult struct {
	UnitId string
	Result bool
}

func NewServiceRegistry(etcdPeers []string, prefix string, timeout time.Duration, options *RegistryOptions) (*ServiceRegistry, error) {
	log.Debugln("Using etcd peers:", etcdPeers)
	cli, err := etcd.NewClient(etcdPeers, http.DefaultTransport.(*http.Transport), timeout)

	if err != nil {
		return nil, err
	}
	s := new(ServiceRegistry)
	s.etcd = cli
	s.units = make(map[string]*ServiceEntry, 100)
	s.Options = *options
	log.Debugln("Using fleet prefix:", prefix)
	s.registry = registry.NewEtcdRegistry(s.etcd, prefix)
	return s, nil
}

//Start monitoring services
func (r *ServiceRegistry) Start() {
	log.Info("Starting fleet and health check loop")
	r.hRateCh = make(chan bool, r.Options.CheckConcurrent)
	r.endCh = make(chan bool)
	r.queryACh = make(chan QueryA, 100)
	r.querySrvCh = make(chan QuerySrv, 100)
	go r.mainLoop(r.endCh)
	<-r.endCh
	r.running = true
}

//Stop monitoring services
func (r *ServiceRegistry) Stop() {
	r.running = false
	r.endCh <- true
}

func (r *ServiceRegistry) mainLoop(endCh chan bool) {
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
		case <-fleetCh.C:
			r.reloadFleet()
		case <-healthCh.C:
			r.doHealthChecks(healthResultsCh)
		case result := <-healthResultsCh:
			r.processHealthCheckResult(result)
		case queryA := <-r.queryACh:
			r.doLookupA(queryA)
		case querySrv := <-r.querySrvCh:
			r.doLookupSrv(querySrv)
		}
	}
}

func (r *ServiceRegistry) LookupA(name string) []AnswerA {
	ch := make(chan []AnswerA, 1)
	r.queryACh <- QueryA{ch, name}
	ans := <-ch
	close(ch)
	return ans
}
func (r *ServiceRegistry) LookupSrv(name, protocol, service string) []AnswerSrv {
	ch := make(chan []AnswerSrv, 1)
	r.querySrvCh <- QuerySrv{ch, name, protocol, service}
	ans := <-ch
	close(ch)
	return ans
}

func (r *ServiceRegistry) doLookupA(q QueryA) {
	entries := r.lookup[q.Name]
	if entries == nil || len(entries) == 0 {
		q.AnswerCh <- []AnswerA{}
		return
	}
	ans := make([]AnswerA, 0, len(entries))
	for _, e := range entries {
		if e.Running && e.Online && e.ServerAddress != nil {
			ans = append(ans, AnswerA{e.ServerAddress, e.CheckInterval})
		}
	}
	q.AnswerCh <- ans
}
func (r *ServiceRegistry) doLookupSrv(q QuerySrv) {
	entries := r.lookup[q.Name]
	if entries == nil || len(entries) == 0 {
		q.AnswerCh <- []AnswerSrv{}
		return
	}
	ans := make([]AnswerSrv, 0, len(entries)*3)
	for _, e := range entries {
		if !e.Running || !e.Online || e.ServerAddress == nil {
			continue
		}
		for _, s := range e.SrvOptions {
			if s.Service != q.Service || s.Protocol != q.Protocol {
				continue
			}
			ans = append(ans, AnswerSrv{e.ServerAddress, *s, e.CheckInterval})
		}
	}
	q.AnswerCh <- ans
}

func (r *ServiceRegistry) processHealthCheckResult(h HealthCheckResult) {
	entry := r.units[h.UnitId]
	if entry == nil {
		return
	}
	entry.PendingHealthChecks -= 1
	//a single failure immediately marks it as offline
	if h.Result == false {
		if entry.Online {
			log.Info("Unit failed health check:", h.UnitId)
		}
		entry.Online = false
		entry.FailedHealthChecks += 1
	} else if entry.PendingHealthChecks == 0 && entry.FailedHealthChecks == 0 {
		entry.Online = true
	}
}

func (r *ServiceRegistry) updateEntry(unitName, machineId, machineIp string, entry *ServiceEntry) error {
	log.Debugln("Updating unit: " + unitName + ":" + machineId)
	u, err := r.registry.Unit(unitName)
	if err != nil {
		log.Warn("Could not read unit from fleet:", err)
		return err
	}
	if u == nil {
		return errors.New("unit data missing")
	}
	vars := new(UnitVars)
	vars.HostName = machineIp
	vars.PrefixName, vars.InstanceName, _ = parseUnitName(unitName)
	vars.UnitName = unitName
	vars.MachineId = machineId
	svc := vars.ServiceOption(r.Options, u.Unit.Options)

	entry.ServiceOption = *svc
	return nil
}

func (r *ServiceRegistry) reloadFleet() {
	log.Debugln("reload fleet")
	machines, err := r.registry.Machines()
	if err != nil {
		log.Warn("Failed to get list of machines:", err)
		return
	}
	//new services should be set with random offset for check times (so not all checks run at once)
	units, err := r.registry.UnitStates()
	if err != nil {
		log.Warn("Failed to get list of units:", err)
		return
	}
	ips := make(map[string]string, len(machines))
	for _, v := range machines {
		ips[v.ID] = v.PublicIP
	}

	r.lookup = make(map[string][]*ServiceEntry, len(units)*3)
	for _, v := range units {
		var entry *ServiceEntry
		if r.units[v.UnitName+":"+v.MachineID] != nil {
			entry = r.units[v.UnitName+":"+v.MachineID]
			if entry.UnitHash != v.UnitHash {
				err := r.updateEntry(v.UnitName, v.MachineID, ips[v.MachineID], entry)
				if err != nil {
					continue
				}
			}
		} else {
			entry = new(ServiceEntry)
			r.units[v.UnitName+":"+v.MachineID] = entry
			err := r.updateEntry(v.UnitName, v.MachineID, ips[v.MachineID], entry)
			if err != nil {
				continue
			}
		}
		entry.UnitHash = v.UnitHash
		entry.ServerAddress = net.ParseIP(ips[v.MachineID])
		if v.ActiveState == "active" {
			entry.Running = true
		} else {
			entry.Running = false
		}

		r.addToLookupTable(entry.Name+".service."+r.Options.Domain, entry)
		for _, t := range entry.Tags {
			r.addToLookupTable(t+"."+entry.Name+".service."+r.Options.Domain, entry)
		}
		for _, s := range entry.SrvOptions {
			r.addToLookupTable("_"+s.Service+"._"+s.Protocol+"."+r.Options.Domain, entry)
		}
	}
}
func (r *ServiceRegistry) addToLookupTable(fqdn string, entry *ServiceEntry) {
	if r.lookup[fqdn] == nil {
		r.lookup[fqdn] = make([]*ServiceEntry, 0, 10)
	}
	r.lookup[fqdn] = append(r.lookup[fqdn], entry)
}

// doHealthChecks fires off all pending health checks (with expired timers)
// and returns the result to the healthCheckResult channel for processing
// hRateCh is used to limit the actual rates in the individual goroutines
// this is so that while health checks are running/pending we can process
// queries and other things
func (r *ServiceRegistry) doHealthChecks(resultCh chan HealthCheckResult) {
	for id, entry := range r.units {
		//only start health checks if we are at or past the interval
		if time.Now().Sub(entry.LastHealthCheck) < entry.CheckInterval {
			continue
		}
		//skip while checks are still being performed
		if entry.PendingHealthChecks > 0 {
			continue
		}
		entry.LastHealthCheck = time.Now()
		entry.FailedHealthChecks = 0
		entry.PendingHealthChecks = len(entry.CheckHttp) + len(entry.CheckTcp)
		//short-circuit if there are no health checks
		if entry.PendingHealthChecks == 0 {
			entry.Online = true
			continue
		}
		for _, u := range entry.CheckHttp {
			go r.checkHttp(u.String(), id, resultCh)
		}
		for _, a := range entry.CheckTcp {
			go r.checkTcp(a.String(), id, resultCh)
		}
	}
}

func (r *ServiceRegistry) checkHttp(url string, unitId string, resultCh chan HealthCheckResult) {
	r.hRateCh <- true
	cli := http.Client{Timeout: r.Options.CheckTimeout}
	resp, err := cli.Get(url)
	if err != nil {
		resultCh <- HealthCheckResult{unitId, false}
		goto done
	}
	ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode/100 == 2 {
		resultCh <- HealthCheckResult{unitId, true}
	} else {
		resultCh <- HealthCheckResult{unitId, false}
	}
done:
	<-r.hRateCh
}

func (r *ServiceRegistry) checkTcp(address string, unitId string, resultCh chan HealthCheckResult) {
	r.hRateCh <- true
	conn, err := net.DialTimeout("tcp", address, r.Options.CheckTimeout)
	if err != nil {
		resultCh <- HealthCheckResult{unitId, false}
		goto done
	}

	conn.Close()
	resultCh <- HealthCheckResult{unitId, true}
done:
	<-r.hRateCh
}
