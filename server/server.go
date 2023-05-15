package server

import (
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
	"github.com/patrickmn/go-cache"

	"github.com/Xuanwo/atomdns/config"
	"github.com/Xuanwo/atomdns/match"
	"github.com/Xuanwo/atomdns/pkg/request"
	"github.com/Xuanwo/atomdns/upstream"
)

// CacheHitCount cache hit count
var CacheHitCount uint64

// QueryCount total query count
var QueryCount uint64

// Server is the dns server.
type Server struct {
	upsrteams map[string]upstream.Upstream // upstream_name -> upstream
	matchers  []match.Match
	rules     map[string]upstream.Upstream // match rules -> upstream

	c *cache.Cache
}

// New will create a new dns server
func New(cfg *config.Config) (s *Server, err error) {
	s = &Server{
		c: cache.New(600*time.Second, 30*time.Minute),
	}

	// Setup streams
	s.upsrteams = make(map[string]upstream.Upstream)
	for _, v := range cfg.Upstreams {
		up, err := upstream.New(v)
		if err != nil {
			return nil, fmt.Errorf("upsteam new: %w", err)
		}
		s.upsrteams[up.Name()] = up
	}

	// Setup matches.
	s.matchers = make([]match.Match, 0, len(cfg.Matches))
	for _, v := range cfg.Matches {
		if _, ok := cfg.Rules[v.Name]; !ok {
			log.Printf("WARNING: match [%v] is not used", v.Name)
		}
		m, err := match.New(v)
		if err != nil {
			return nil, fmt.Errorf("match new: %w", err)
		}
		s.matchers = append(s.matchers, m)

	}

	// Setup rules.
	s.rules = make(map[string]upstream.Upstream)
	for k, v := range cfg.Rules {
		s.rules[k] = s.upsrteams[v]
	}
	return s, nil
}

// ServeDNS implements dns.Handler
func (s *Server) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {

	atomic.AddUint64(&QueryCount, 1)

	req := &request.Request{R: r}

	if v, ok := s.c.Get(req.ID()); ok {
		m := v.(*dns.Msg)
		m.Id = r.Id
		log.Printf("cached %s => %v", req.ID(), m.Answer)
		atomic.AddUint64(&CacheHitCount, 1)
		err := w.WriteMsg(m)
		if err != nil {
			log.Printf("write msg: %v", err)
			return
		}
		return
	}

	up := s.rules["default"]

	for _, m := range s.matchers {
		if s.rules[m.Name()] != nil && m.IsMatch(req) {
			up = s.rules[m.Name()]
			// log.Printf("rule %s matched, served via %s", m.Name(), s.rules[m.Name()].Name())
			break
		}
	}
	// log.Printf("no rules matched, served via %s", s.rules["default"].Name())

	m, err := up.ServeDNS(req)
	if err != nil {
		m = new(dns.Msg)
		m.SetRcode(r, dns.RcodeServerFailure)
	}

	log.Printf("%v => %v => %v", r.Question, up.Name(), m.Answer)

	err = w.WriteMsg(m)
	if err != nil {
		log.Printf("dns response write failed: %v", err)
		return
	}

	if m.Rcode == dns.RcodeServerFailure {
		return
	}

	if len(m.Answer) > 0 {
		s.c.Set(req.ID(), m.Copy(), time.Duration(m.Answer[0].Header().Ttl)*time.Second)
	} else {
		s.c.Set(req.ID(), m.Copy(), 0)
	}
}
