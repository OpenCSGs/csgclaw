package modelprovider

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func downloadGeneratedImage(ctx context.Context, rawURL, baseURL string) ([]byte, error) {
	fail := func(status int, message string) error {
		return &ImageGenerationError{HTTPStatus: status, Code: "image_download_failed", Message: message}
	}
	provider, _ := url.Parse(baseURL)
	sameOrigin := func(u *url.URL) bool {
		return provider != nil && strings.EqualFold(u.Host, provider.Host) && u.Scheme == provider.Scheme
	}
	validURL := func(u *url.URL) bool {
		return u.Hostname() != "" && u.User == nil && (u.Scheme == "https" || (u.Scheme == "http" && sameOrigin(u)))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil || !validURL(req.URL) {
		return nil, fail(0, "The image service returned an unsupported download URL.")
	}

	// Use a separate transport so provider headers, cookies, and proxy credentials
	// cannot be forwarded to the image host. Check the actual address dialed.
	dialer := &net.Dialer{}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			if provider != nil && strings.EqualFold(address, net.JoinHostPort(provider.Hostname(), imageOriginPort(provider))) {
				return dialer.DialContext(ctx, network, address)
			}
			addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, resolved := range addresses {
				ip := resolved.IP
				if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || sharedAddressSpace.Contains(ip) {
					continue
				}
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
			}
			return nil, fmt.Errorf("image host is unreachable")
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 30 * time.Second, Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !validURL(req.URL) || (!sameOrigin(via[0].URL) && sameOrigin(req.URL)) {
			return http.ErrUseLastResponse
		}
		req.Header.Del("Referer")
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, imageTransportError(ctx.Err())
		}
		return nil, fail(0, "Could not download the image returned by the image service.")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fail(resp.StatusCode, fmt.Sprintf("The image download returned HTTP %d.", resp.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxGeneratedImageBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, imageTransportError(ctx.Err())
		}
		return nil, fail(resp.StatusCode, "The image download could not be read completely.")
	}
	if len(data) == 0 || len(data) > MaxGeneratedImageBytes {
		return nil, fail(resp.StatusCode, "The downloaded image is empty or exceeds the supported size limit.")
	}
	return data, nil
}

var sharedAddressSpace = &net.IPNet{IP: net.IPv4(100, 64, 0, 0).To4(), Mask: net.CIDRMask(10, 32)}

func imageOriginPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}
