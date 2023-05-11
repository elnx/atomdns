package upstream

import (
	"crypto/tls"
	"fmt"
	"log"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/miekg/dns"

	"github.com/Xuanwo/atomdns/pkg/request"
)

type client struct {
	config *Config

	// DoT related config
	TLSServerName string `hcl:"tls_server_name,optional"`

	c *dns.Client
}

// Name implements upstream.Name
func (c *client) Name() string {
	return c.config.Name
}

// ServeDNS implements upstream.ServeDNS
func (c *client) ServeDNS(r *request.Request) (m *dns.Msg, err error) {
	// mimicking bind9...
	r.R.CheckingDisabled = true
	r.R.Extra = r.R.Extra[:0]
	r.R.SetEdns0(dns.DefaultMsgSize, true)
	// o := new(dns.OPT)
	// o.Hdr.Name = "."
	// o.Hdr.Rrtype = dns.TypeOPT
	// o.SetUDPSize(dns.DefaultMsgSize)
	// o.SetDo()
	// e := new(dns.EDNS0_COOKIE)
	// e.Code = dns.EDNS0COOKIE
	// e.Cookie = "1234567812345678"
	// o.Option = append(o.Option, e)
	// r.R.Extra = append(r.R.Extra, o)

	m, _, err = c.c.Exchange(r.R, c.config.Addr)
	if err != nil {
		log.Printf("serve dns: %v", err)
		return nil, err
	}
	return
}

// NewTCPClient create a new tcp client.
func NewTCPClient(cfg *Config) (u Upstream, err error) {
	c := &client{config: cfg}
	c.c = &dns.Client{
		Net: "tcp",
	}
	return c, nil
}

// NewDoTClient create a new dot client
func NewDoTClient(cfg *Config) (u Upstream, err error) {
	c := &client{config: cfg}

	var diags hcl.Diagnostics
	diags = gohcl.DecodeBody(cfg.Options, nil, c)
	if diags.HasErrors() {
		return nil, fmt.Errorf("new domain list: %w", diags)
	}

	c.c = &dns.Client{
		Net: "tcp-tls",
		TLSConfig: &tls.Config{
			ServerName: "",
		},
	}
	return c, nil
}

// NewUDPClient create a new udp client.
func NewUDPClient(cfg *Config) (u Upstream, err error) {
	c := &client{config: cfg}
	c.c = &dns.Client{}
	return c, nil
}
