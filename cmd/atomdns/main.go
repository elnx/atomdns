package main

import (
	"log"
	"os"
	"sync"
	"time"

	"github.com/miekg/dns"

	"github.com/Xuanwo/atomdns/config"
	"github.com/Xuanwo/atomdns/server"
)

const allInterfaces = "[::]:53"

func main() {
	if len(os.Args) < 2 {
		log.Fatal("no config input")
	}

	cfg, err := config.Load(os.Args[1])
	if err != nil {
		log.Fatalf("config load failed: %v", err)
	}

	s, err := server.New(cfg)
	if err != nil {
		log.Fatalf("server new failed: %v", err)
	}

	if len(cfg.Listen) == 0 {
		cfg.Listen = []string{allInterfaces}
		log.Printf("WARNING: no listen address found, listening on %v by default", allInterfaces)
	}
	wg := sync.WaitGroup{}
	for _, v := range cfg.Listen {
		log.Printf("listening on %v", v)
		wg.Add(1)
		go func(addr string) {
			err = dns.ListenAndServe(addr, "udp", s)
			if err != nil {
				log.Printf("dns server exited: %v", err)
				wg.Done()
			}
		}(v)
	}

	go func() {
		for range time.Tick(time.Minute) {
			log.Printf("cache hit rate = %.4f", float64(server.CacheHitCount)/float64(server.QueryCount))
		}
	}()

	wg.Wait()
}
