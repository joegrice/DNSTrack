package dns

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/miekg/dns"
)

const queryTimeout = 5 * time.Second

type Result struct {
	Domain         string
	ResponseTimeMs float64
	Success        bool
	Error          string
}

func ResolveUDP(serverIP string, domain string) Result {
	queryDomain := domain
	if !strings.HasSuffix(queryDomain, ".") {
		queryDomain += "."
	}

	c := new(dns.Client)
	c.Timeout = queryTimeout

	m := new(dns.Msg)
	m.SetQuestion(queryDomain, dns.TypeA)
	m.RecursionDesired = true

	start := time.Now()
	r, _, err := c.Exchange(m, serverIP)
	elapsed := time.Since(start).Seconds() * 1000

	if err != nil {
		return Result{
			Domain:         domain,
			ResponseTimeMs: elapsed,
			Success:        false,
			Error:          err.Error(),
		}
	}

	if r.Rcode != dns.RcodeSuccess {
		return Result{
			Domain:         domain,
			ResponseTimeMs: elapsed,
			Success:        false,
			Error:          fmt.Sprintf("rcode: %d", r.Rcode),
		}
	}

	return Result{
		Domain:         domain,
		ResponseTimeMs: elapsed,
		Success:        true,
	}
}

func WarmUp(serverIP string) {
	domains := []string{"google.com.", "cloudflare.com."}
	for _, domain := range domains {
		c := new(dns.Client)
		c.Timeout = 3 * time.Second
		m := new(dns.Msg)
		m.SetQuestion(domain, dns.TypeA)
		m.RecursionDesired = true
		c.Exchange(m, serverIP)
	}
}

func ResolveWithWarmup(ctx context.Context, serverIP string, domains []string) []Result {
	if len(serverIP) == 0 {
		return nil
	}

	var results []Result

	for _, domain := range domains {
		select {
		case <-ctx.Done():
			return results
		default:
		}

		r := ResolveUDP(serverIP, domain)
		results = append(results, r)
	}

	return results
}

func PickIP(ips []string) string {
	if len(ips) == 0 {
		return ""
	}
	return ips[rand.Intn(len(ips))]
}
