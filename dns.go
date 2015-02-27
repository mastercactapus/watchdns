package main

import (
	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"math/rand"
	"strings"
)

type dnsServer struct {
	registry    *ServiceRegistry
	shiftCounts map[string]int
}

func serveDns(r *ServiceRegistry, bindAddr string) {
	log.Info("Starting dns server", bindAddr)
	dns.ListenAndServe(bindAddr, "udp", &dnsServer{r, make(map[string]int)})
}

func (d *dnsServer) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Answer = make([]dns.RR, 0, len(r.Question)*3)
	m.Extra = make([]dns.RR, 0, len(r.Question)*3)
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
			ans := d.registry.LookupSrv(q.Name, parts[0][1:], parts[1][1:])
			log.Debugln("Answer[SRV]", ans)
			for _, rec := range ans {
				srv := new(dns.SRV)
				srv.Hdr = dns.RR_Header{Name: q.Name, Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: uint32(rec.Ttl.Seconds())}
				srv.Port = rec.Port
				srv.Priority = rec.Priority
				srv.Target = rec.Target
				srv.Weight = rec.Weight
				m.Answer = append(m.Answer, srv)

				a := new(dns.A)
				a.Hdr = dns.RR_Header{Name: rec.Target, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: uint32(rec.Ttl.Seconds())}
				a.A = rec.TargetIP
				m.Extra = append(m.Extra, a)
			}
		}
	}

	if d.registry.Options.RecordSort == "random" {
		p := rand.Perm(len(m.Answer))
		for i, v := range p {
			m.Answer[i] = m.Answer[v]
		}
	} else if d.registry.Options.RecordSort == "roundrobin" {
		shift := d.shiftCounts[r.Question[0].Name]
		d.shiftCounts[r.Question[0].Name] = (shift + 1) % len(m.Answer)
		for i := range m.Answer {
			m.Answer[i] = m.Answer[(i+shift)%len(m.Answer)]
		}
	}

	w.WriteMsg(m)
}
