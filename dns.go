package main

import (
	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"strings"
)

type dnsServer struct {
	registry *ServiceRegistry
}

func serveDns(r *ServiceRegistry, bindAddr string) {
	log.Info("Starting dns server", bindAddr)
	dns.ListenAndServe(bindAddr, "udp", &dnsServer{r})
}

func (d *dnsServer) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Answer = make([]dns.RR, 0, len(r.Question)*3)
	for _, q := range r.Question {
		log.Debugln("Query", q.String())
		switch q.Qtype {
		case dns.TypeA:
			ans := d.registry.LookupA(q.Name)
			log.Debugln("Answer[A]", ans)
			for _, rec := range ans {
				a := new(dns.A)
				a.Hdr = dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: uint32(rec.Ttl.Seconds())}
				a.A = rec.Server
				m.Answer = append(m.Answer, a)
			}
		case dns.TypeSRV:
			parts := strings.SplitN(q.Name, ".", 3)
			if len(parts) != 3 || len(parts[0]) < 2 || len(parts[1]) < 2 || parts[0][0] != '_' || parts[1][0] != '_' {
				log.Warn("Invalid SRV request:", q.Name)
				continue
			}
			ans := d.registry.LookupSrv(q.Name, parts[1][1:], parts[0][1:])
			log.Debugln("Answer[SRV]", ans)
			for _, rec := range ans {
				a := new(dns.SRV)
				a.Hdr = dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: uint32(rec.Ttl.Seconds())}
				a.Port = rec.Port
				a.Priority = rec.Priority
				a.Target = rec.Server.String()
				a.Weight = rec.Weight
				m.Answer = append(m.Answer, a)
			}
		}
	}
	w.WriteMsg(m)
}
