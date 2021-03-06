package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/coreos/fleet/Godeps/_workspace/src/github.com/coreos/go-systemd/unit" //this is why you don't embed dependencies
	log "github.com/sirupsen/logrus"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type UnitVars struct {
	UnitName     string
	PrefixName   string
	InstanceName string
	MachineId    string
	HostName     string
}

type SrvOption struct {
	Service  string
	Protocol string
	Priority uint16
	Weight   uint16
	Port     uint16
}

type ServiceOption struct {
	Name          string
	Tags          []string
	SrvOptions    []*SrvOption
	CheckHttp     []*url.URL
	CheckTcp      []*net.TCPAddr
	CheckInterval time.Duration
	CheckTimeout  time.Duration
}

func parseUnitName(name string) (prefix, instance, unitType string) {
	idx := strings.LastIndex(name, ".")
	if idx == -1 {
		return name, "", ""
	}
	unitType = name[idx:]
	sIdx := strings.Index(name[:idx], "@")
	if sIdx == -1 {
		prefix = name[:idx]
	} else {
		prefix = name[:sIdx]
		instance = name[sIdx+1 : idx]
	}
	return
}

func parseSrvOption(val string) (*SrvOption, error) {
	var err error
	parts := strings.Split(val, ":")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid format '%s', should be in format <service>:<protocol>:<port>[:priority[:weight]]", val)
	}
	s := new(SrvOption)
	s.Service = parts[0]
	s.Protocol = parts[1]
	i, err := strconv.ParseUint(parts[2], 10, 16)
	if err != nil {
		return nil, fmt.Errorf("bad port specifier '%s': %s", parts[2], err.Error())
	}
	s.Port = uint16(i)
	if len(parts) >= 4 {
		i, err = strconv.ParseUint(parts[3], 10, 16)
		if err != nil {
			return nil, fmt.Errorf("bad priority specifier '%s': %s", parts[3], err.Error())
		}
		s.Priority = uint16(i)
	}
	if len(parts) >= 5 {
		i, err = strconv.ParseUint(parts[4], 10, 16)
		if err != nil {
			return nil, fmt.Errorf("bad priority specifier '%s': %s", parts[4], err.Error())
		}
		s.Weight = uint16(i)
	}
	return s, nil
}

func systemdUnescape(escaped string) string {
	escaped = strings.Replace(escaped, "-", "/", -1)
	var out bytes.Buffer
	var i int
	var val []byte
	var err error
	for {
		i = strings.IndexByte(escaped, '\\')
		if i == -1 {
			out.WriteString(escaped)
			break
		}
		if i+4 >= len(escaped) {
			out.WriteString(escaped[i:])
			break
		}
		if escaped[i+1] != 'x' {
			out.WriteString(escaped[:i+1])
			escaped = escaped[i+1:]
			continue
		}
		val, err = hex.DecodeString(escaped[i+2 : i+4])
		if err != nil || len(val) != 1 {
			out.WriteString(escaped[:i+1])
			escaped = escaped[i+1:]
			continue
		}
		out.WriteString(escaped[:i])
		out.WriteByte(val[0])
		escaped = escaped[i+4:]
	}
	return out.String()
}

func (v *UnitVars) ExpandValue(val string) string {
	var out bytes.Buffer
	var i int
	for {
		i = strings.IndexByte(val, '%')
		if i == -1 || i == len(val)-1 {
			out.WriteString(val)
			break
		}
		out.WriteString(val[:i])
		switch val[i+1] {
		case 'n':
			out.WriteString(v.UnitName)
		case 'N':
			out.WriteString(systemdUnescape(v.UnitName))
		case 'p':
			out.WriteString(v.PrefixName)
		case 'P':
			out.WriteString(systemdUnescape(v.PrefixName))
		case 'i':
			out.WriteString(v.InstanceName)
		case 'I':
			out.WriteString(systemdUnescape(v.InstanceName))
		case 'm':
			out.WriteString(v.MachineId)
		case 'H':
			out.WriteString(v.HostName)
		case '%':
			out.WriteString(val[i : i+1])
		default:
			out.WriteString(val[i : i+2])
		}
		val = val[i+2:]
	}
	return out.String()
}

func (vars *UnitVars) ServiceOption(defaults RegistryOptions, opts []*unit.UnitOption) *ServiceOption {
	o := new(ServiceOption)
	o.Name = vars.ExpandValue("%P")
	o.Tags = make([]string, 0, 4)
	o.SrvOptions = make([]*SrvOption, 0, 2)
	o.CheckHttp = make([]*url.URL, 0, 8)
	o.CheckTcp = make([]*net.TCPAddr, 0, 5)
	o.CheckInterval = defaults.CheckInterval
	if vars.InstanceName != "" {
		o.Tags = append(o.Tags, "i-"+vars.ExpandValue("%I"))
	}
	for _, v := range opts {
		if v.Section != "X-Watchdns" {
			continue
		}
		switch v.Name {
		case "CheckInterval":
			i, err := time.ParseDuration(v.Value)
			if err != nil {
				log.Warnf("Could not parse CheckInterval value '%s' in unit %s: %s\n", v.Value, vars.UnitName, err.Error())
				continue
			}
			o.CheckInterval = i
		case "CheckTimeout":
			i, err := time.ParseDuration(v.Value)
			if err != nil {
				log.Warnf("Could not parse CheckTimeout value '%s' in unit %s: %s\n", v.Value, vars.UnitName, err.Error())
				continue
			}
			o.CheckTimeout = i
		case "Name":
			o.Name = vars.ExpandValue(v.Value)
		case "Tag":
			o.Tags = append(o.Tags, vars.ExpandValue(v.Value))
		case "Srv":
			srv, err := parseSrvOption(vars.ExpandValue(v.Value))
			if err != nil {
				log.Warnf("Could not parse Srv value '%s' in unit %s: %s\n", v.Value, vars.UnitName, err.Error())
				continue
			}
			o.SrvOptions = append(o.SrvOptions, srv)
		case "CheckHttp":
			u, err := url.Parse(vars.ExpandValue(v.Value))
			if err != nil {
				log.Warnf("Could not parse CheckHttp value '%s' in unit %s: %s\n", v.Value, vars.UnitName, err.Error())
				continue
			}
			o.CheckHttp = append(o.CheckHttp, u)
		case "CheckTcp":
			addr, err := net.ResolveTCPAddr("tcp", vars.ExpandValue(v.Value))
			if err != nil {
				log.Warnf("Could not parse CheckTcp value '%s' in unit %s: %s\n", v.Value, vars.UnitName, err.Error())
				continue
			}
			o.CheckTcp = append(o.CheckTcp, addr)
		default:
			log.Warnf("Skipping unknown field '%s' in unit %s\n", v.Name, vars.UnitName)
		}
	}
	return o
}
