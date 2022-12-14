package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/miekg/dns"
	"github.com/ow-api/ovrstat/ovrstat"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func main() {
	cloudfrontUrl, err := lookupCloudfrontURL()

	if err != nil {
		log.Fatalln("Unable to find url:", err)
	}

	log.Println("URL:", cloudfrontUrl)

	ips, err := lookupIpv6Dns(cloudfrontUrl)

	if err != nil {
		log.Fatalln("Unable to find ips:", err)
	}

	log.Println("IPs:", ips)

	localIps, err := checkIpv6Local()

	if err != nil {
		log.Fatalln("Unable to get local ips")
	}

	log.Println("Local IPs:", localIps)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	localAddr, err := net.ResolveIPAddr("ip", "")

	if err != nil {
		return
	}

	// You also need to do this to make it work and not give you a
	// "mismatched local address type ip"
	// This will make the ResolveIPAddr a TCPAddr without needing to
	// say what SRC port number to use.
	localTCPAddr := net.TCPAddr{
		IP: localAddr.IP,
	}

	defaultDialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	dialer := &net.Dialer{
		LocalAddr: &localTCPAddr,
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			oldNetwork := network
			oldAddr := addr

			if addr == "playoverwatch.com:443" {
				network = "tcp6"
				addr = "[" + ips[r.Intn(len(ips))] + "]:443"
			}

			log.Println("Dial using", addr)

			c, err := dialer.DialContext(ctx, network, addr)

			if err != nil {
				log.Println("Fallback")
				c, err = defaultDialer.DialContext(ctx, oldNetwork, oldAddr)
			}

			return c, err
		},
		TLSClientConfig: &tls.Config{
			ServerName: "playoverwatch.com",
		},
	}

	http.DefaultClient = &http.Client{
		Transport: transport,
	}

	stats, err := ovrstat.Stats(ovrstat.PlatformPC, "cats-11481")

	if err != nil {
		log.Fatalln("Error retrieving:", err)
	}

	log.Println(stats.Name+" is level", stats.Endorsement)
}

func lookupCloudfrontURL() (string, error) {
	res, err := http.Get("https://playoverwatch.com/en-us/")

	if err != nil {
		return "", err
	}

	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)

	links := doc.Find("link").Map(func(i int, s *goquery.Selection) string {
		v, exists := s.Attr("href")

		if !exists {
			return ""
		}

		return v
	})

	for _, link := range links {
		u, err := url.Parse(link)

		if err != nil {
			continue
		}

		if strings.HasSuffix(u.Host, "cloudfront.net") {
			return u.Host, nil
		}
	}

	return "", errors.New("no cloudfront url")
}

func lookupIpv6Dns(host string) ([]string, error) {
	c := &dns.Client{
		Net: "udp",
	}

	msg := new(dns.Msg)
	msg.Id = dns.Id()
	msg.RecursionDesired = true
	msg.Question = make([]dns.Question, 1)
	msg.Question[0] = dns.Question{Name: dns.Fqdn(host), Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}

	in, _, err := c.Exchange(msg, "8.8.8.8:53")

	result := make([]string, 0)

	if err != nil {
		return nil, err
	}

	if in != nil && in.Rcode != dns.RcodeSuccess {
		return result, errors.New(dns.RcodeToString[in.Rcode])
	}

	for _, record := range in.Answer {
		if t, ok := record.(*dns.AAAA); ok {
			result = append(result, t.AAAA.String())
		}
	}
	return result, err
}

func checkIpv6Local() ([]string, error) {
	ifaces, err := net.Interfaces()

	if err != nil {
		return nil, err
	}

	ret := make([]string, 0)

	for _, i := range ifaces {
		addrs, err := i.Addrs()

		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP

			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			log.Println("Check", ip.String())

			if ip == nil || ip.To4() == nil || isPrivateIP(ip) {
				continue
			}

			log.Println("Checking", ip.String())

			if ipnet, ok := addr.(*net.IPNet); ok {
				for ip := ipnet.IP.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
					ret = append(ret, ip.String())
				}

				continue
			}

			ret = append(ret, ip.String())
		}
	}

	return ret, nil
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

var privateIPBlocks []*net.IPNet

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8",    // IPv4 loopback
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"169.254.0.0/16", // RFC3927 link-local
		"::1/128",        // IPv6 loopback
		"fe80::/10",      // IPv6 link-local
		"fc00::/7",       // IPv6 unique local addr
	} {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Errorf("parse error on %q: %v", cidr, err))
		}
		privateIPBlocks = append(privateIPBlocks, block)
	}
}

func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}
