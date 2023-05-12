package upstream

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"time"

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

func mimicBind9(r *request.Request) {
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
}

func exchangeWithFilter(c *dns.Client, m *dns.Msg, a string) (r *dns.Msg, rtt time.Duration, err error) {
	var co *dns.Conn

	co, err = c.Dial(a)

	if err != nil {
		return nil, 0, err
	}
	defer co.Close()

	opt := m.IsEdns0()
	// If EDNS0 is used use that for size.
	if opt != nil && opt.UDPSize() >= dns.MinMsgSize {
		co.UDPSize = opt.UDPSize()
	}
	// Otherwise use the client's configured UDP size.
	if opt == nil && c.UDPSize >= dns.MinMsgSize {
		co.UDPSize = c.UDPSize
	}

	// write with the appropriate write timeout
	t := time.Now()
	writeDeadline := t.Add(2 * time.Second)
	readDeadline := t.Add(2 * time.Second)

	ctx := context.Background()
	if deadline, ok := ctx.Deadline(); ok {
		if deadline.Before(writeDeadline) {
			writeDeadline = deadline
		}
		if deadline.Before(readDeadline) {
			readDeadline = deadline
		}
	}
	co.SetWriteDeadline(writeDeadline)
	co.SetReadDeadline(readDeadline)

	co.TsigSecret, co.TsigProvider = c.TsigSecret, c.TsigProvider

	if err = co.WriteMsg(m); err != nil {
		return nil, 0, err
	}

	for {
		r, err = co.ReadMsg()
		// Ignore replies with mismatched IDs because they might be
		// responses to earlier queries that timed out.
		if err != nil || (r.Id == m.Id && r.CheckingDisabled && len(r.Extra) > 0) {
			break
		}
	}
	rtt = time.Since(t)
	return r, rtt, err
}

// ServeDNS implements upstream.ServeDNS
func (c *client) ServeDNS(r *request.Request) (m *dns.Msg, err error) {
	switch c.config.Filter {
	case "kernel":
		mimicBind9(r)
		fallthrough
	case "":
		m, _, err = c.c.Exchange(r.R, c.config.Addr)
		if err != nil {
			log.Printf("serve dns: %v", err)
			return nil, err
		}
	case "user":
		mimicBind9(r)
		m, _, err = exchangeWithFilter(c.c, r.R, c.config.Addr)
	default:
		m = nil
		err = fmt.Errorf("unexpected filter: %v", c.config.Filter)
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
