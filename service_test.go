package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestParseSrvOption(t *testing.T) {
	s, err := parseSrvOption("xmpp:tcp:4000")
	assert.NoError(t, err)
	assert.Equal(t, 4000, s.Port)
	assert.Equal(t, 0, s.Priority)
	assert.Equal(t, "tcp", s.Protocol)
	assert.Equal(t, "xmpp", s.Service)
	assert.Equal(t, 0, s.Weight)
	s, err = parseSrvOption("xmpp:tcp:4000:23")
	assert.NoError(t, err)
	assert.Equal(t, 4000, s.Port)
	assert.Equal(t, 23, s.Priority)
	assert.Equal(t, "tcp", s.Protocol)
	assert.Equal(t, "xmpp", s.Service)
	assert.Equal(t, 0, s.Weight)
	s, err = parseSrvOption("xmpp:tcp:4000:23:4")
	assert.NoError(t, err)
	assert.Equal(t, 4000, s.Port)
	assert.Equal(t, 23, s.Priority)
	assert.Equal(t, "tcp", s.Protocol)
	assert.Equal(t, "xmpp", s.Service)
	assert.Equal(t, 4, s.Weight)
}

func TestParseUnitName(t *testing.T) {
	p, i, ty := parseUnitName("foobar.service")
	assert.Equal(t, "foobar", p)
	assert.Equal(t, "", i)
	assert.Equal(t, ".service", ty)
	p, i, ty = parseUnitName("foobar@one.service")
	assert.Equal(t, "foobar", p)
	assert.Equal(t, "one", i)
	assert.Equal(t, ".service", ty)
	//should only parse, but not unescape
	p, i, ty = parseUnitName("foo\\x25bar@one-there.service")
	assert.Equal(t, "foo\\x25bar", p)
	assert.Equal(t, "one-there", i)
	assert.Equal(t, ".service", ty)
	p, i, ty = parseUnitName("foobar@.service")
	assert.Equal(t, "foobar", p)
	assert.Equal(t, "", i)
	assert.Equal(t, ".service", ty)
	p, i, ty = parseUnitName("foobar@1@f.service")
	assert.Equal(t, "foobar", p)
	assert.Equal(t, "1@f", i)
	assert.Equal(t, ".service", ty)

}

func TestSystemdUnescape(t *testing.T) {
	assert.Equal(t, "/ho-me/nathan/.local/Steam/steamap\\%@test\\ing", systemdUnescape(`-ho\x2dme-nathan-.local-Steam-steamap\\x25\x40test\x5cing`))
}

func TestUnitVars_ExpandValue(t *testing.T) {
	vars := &UnitVars{
		UnitName:     "example@bar.service",
		PrefixName:   "example",
		InstanceName: "bar",
		MachineId:    "0123456789abcdef0123456789abcdef",
		HostName:     "foobar.local",
	}

	assert.Equal(t, "foo%foobar.localbar", vars.ExpandValue("foo%%%Hbar"))
	assert.Equal(t, "foo%?%s0123456789abcdef0123456789abcdefr", vars.ExpandValue("foo%?%s%mr"), "host and escaped %")
	assert.Equal(t, "bar", vars.ExpandValue("%i"))
	assert.Equal(t, "example", vars.ExpandValue("%p"))
	assert.Equal(t, "example@bar.service", vars.ExpandValue("%n"))
}
