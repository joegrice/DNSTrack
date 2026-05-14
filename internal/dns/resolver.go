package dns

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/miekg/dns"
)

const queryTimeout = 5 * time.Second

type Result struct {
	Domain         string
	RecordType     string
	ResponseTimeMs float64
	Success        bool
	Error          string
}

func StringToQType(s string) uint16 {
	switch strings.ToUpper(s) {
	case "A":
		return dns.TypeA
	case "AAAA":
		return dns.TypeAAAA
	case "MX":
		return dns.TypeMX
	case "TXT":
		return dns.TypeTXT
	case "CNAME":
		return dns.TypeCNAME
	case "NS":
		return dns.TypeNS
	case "SOA":
		return dns.TypeSOA
	case "PTR":
		return dns.TypePTR
	case "SRV":
		return dns.TypeSRV
	default:
		return dns.TypeA
	}
}

func ResolveUDP(serverIP string, domain string, qtype uint16) Result {
	queryDomain := domain
	if !strings.HasSuffix(queryDomain, ".") {
		queryDomain += "."
	}

	c := new(dns.Client)
	c.Timeout = queryTimeout

	m := new(dns.Msg)
	m.SetQuestion(queryDomain, qtype)
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

func ResolveDoH(dohURL, domain string, qtype uint16) Result {
	queryDomain := domain
	if !strings.HasSuffix(queryDomain, ".") {
		queryDomain += "."
	}

	m := new(dns.Msg)
	m.SetQuestion(queryDomain, qtype)
	m.RecursionDesired = true

	msgBuf, err := m.Pack()
	if err != nil {
		return Result{
			Domain:   domain,
			Success:  false,
			Error:    fmt.Sprintf("pack query: %v", err),
		}
	}

	client := &http.Client{Timeout: queryTimeout}
	req, err := http.NewRequest("POST", dohURL, bytes.NewReader(msgBuf))
	if err != nil {
		return Result{
			Domain:   domain,
			Success:  false,
			Error:    fmt.Sprintf("create request: %v", err),
		}
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start).Seconds() * 1000
	if err != nil {
		return Result{
			Domain:         domain,
			ResponseTimeMs: elapsed,
			Success:        false,
			Error:          err.Error(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{
			Domain:         domain,
			ResponseTimeMs: elapsed,
			Success:        false,
			Error:          fmt.Sprintf("http %d", resp.StatusCode),
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{
			Domain:         domain,
			ResponseTimeMs: elapsed,
			Success:        false,
			Error:          fmt.Sprintf("read body: %v", err),
		}
	}

	r := new(dns.Msg)
	if err := r.Unpack(body); err != nil {
		return Result{
			Domain:         domain,
			ResponseTimeMs: elapsed,
			Success:        false,
			Error:          fmt.Sprintf("unpack response: %v", err),
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

func ResolveWithWarmup(ctx context.Context, serverIP string, domains []string, recordTypes []uint16) []Result {
	if len(serverIP) == 0 {
		return nil
	}

	var results []Result

	for _, domain := range domains {
		for _, qtype := range recordTypes {
			select {
			case <-ctx.Done():
				return results
			default:
			}

			r := ResolveUDP(serverIP, domain, qtype)
			r.RecordType = dns.TypeToString[qtype]
			results = append(results, r)

			time.Sleep(50 * time.Millisecond)
		}
	}

	return results
}

func ResolveDoHWithWarmup(ctx context.Context, dohURL string, domains []string, recordTypes []uint16) []Result {
	if len(dohURL) == 0 {
		return nil
	}

	var results []Result

	for _, domain := range domains {
		for _, qtype := range recordTypes {
			select {
			case <-ctx.Done():
				return results
			default:
			}

			r := ResolveDoH(dohURL, domain, qtype)
			r.RecordType = dns.TypeToString[qtype]
			results = append(results, r)

			time.Sleep(50 * time.Millisecond)
		}
	}

	return results
}

func PickIP(ips []string) string {
	if len(ips) == 0 {
		return ""
	}
	return ips[rand.Intn(len(ips))]
}
