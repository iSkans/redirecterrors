// Package redirecterrors traefik plugin to do external redirect on HTTP errors
package redirecterrors

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Config the plugin configuration.
type Config struct {
	Status              []string          `json:"status,omitempty"`
	Target              string            `json:"target,omitempty"`
	OutputStatus        int               `json:"outputStatus,omitempty"`
	OutputAddHeaders    map[string]string `json:"outputAddHeaders,omitempty"`
	OutputRemoveHeaders []string         `json:"outputRemoveHeaders,omitempty"`
	OutputAddCookies    []string         `json:"outputAddCookies,omitempty"`
	OutputRemoveCookies []string         `json:"outputRemoveCookies,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		Status:       []string{},
		Target:       "",
		OutputStatus: 302,
	}
}

// RedirectErrors a RedirectErrors plugin.
type RedirectErrors struct {
	name                string
	next                http.Handler
	httpCodeRanges      HTTPCodeRanges
	target              string
	outputStatus        int
	outputAddHeaders    map[string]string
	outputRemoveHeaders []*regexp.Regexp
	outputAddCookies    []string
	outputRemoveCookies []*regexp.Regexp
}

// New creates a new RedirectErrors plugin.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	if len(config.Target) == 0 {
		return nil, fmt.Errorf("target url must be set")
	}

	httpCodeRanges, err := NewHTTPCodeRanges(config.Status)
	if err != nil {
		return nil, err
	}

	// Compile regex patterns for header removal
	var removePatterns []*regexp.Regexp
	for _, pattern := range config.OutputRemoveHeaders {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern '%s': %w", pattern, err)
		}
		removePatterns = append(removePatterns, re)
	}

	// Compile regex patterns for cookie removal
	var removeCookiePatterns []*regexp.Regexp
	for _, pattern := range config.OutputRemoveCookies {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid cookie regex pattern '%s': %w", pattern, err)
		}
		removeCookiePatterns = append(removeCookiePatterns, re)
	}

	return &RedirectErrors{
		httpCodeRanges:      httpCodeRanges,
		next:                next,
		name:                name,
		target:              config.Target,
		outputStatus:        config.OutputStatus,
		outputAddHeaders:    config.OutputAddHeaders,
		outputRemoveHeaders: removePatterns,
		outputAddCookies:    config.OutputAddCookies,
		outputRemoveCookies: removeCookiePatterns,
	}, nil
}

func (a *RedirectErrors) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	catcher := newCodeCatcher(rw, a.httpCodeRanges)

	a.next.ServeHTTP(catcher, req)
	if !catcher.isFilteredCode() {
		return
	}
	code := catcher.getCode()
	println("Caught HTTP status code", code, "redirecting")

	// try to cobble together the original URL
	proto := req.Header.Get("X-Forwarded-Proto")
	host := req.Header.Get("X-Forwarded-Host")
	fullURL := req.URL.String()
	if len(proto) != 0 && len(host) != 0 {
		fullURL = proto + "://" + host
		fullURL += req.URL.RequestURI()
	} else {
		println("Missing proxy headers!")
	}

	location := a.target
	location = strings.ReplaceAll(location, "{status}", strconv.Itoa(code))
	location = strings.ReplaceAll(location, "{url}", url.QueryEscape(fullURL))

	println("New location:", location)

	// First, copy all headers from the catcher to the response writer
	for key, values := range catcher.getHeaders() {
		for _, value := range values {
			rw.Header().Add(key, value)
		}
	}

	// Set the Location header
	rw.Header().Set("Location", location)

	// Add custom headers
	for key, value := range a.outputAddHeaders {
		rw.Header().Set(key, value)
	}

	// Remove headers matching regex patterns (case-insensitive)
	for key := range rw.Header() {
		for _, re := range a.outputRemoveHeaders {
			if re.MatchString(key) {
				rw.Header().Del(key)
				println("Removing header:", key)
				break
			}
		}
	}

	// Add cookies from outputAddCookies
	for _, cookie := range a.outputAddCookies {
		rw.Header().Add("Set-Cookie", cookie)
		println("Adding cookie:", extractCookieName(cookie))
	}

	// Remove cookies matching regex patterns from outputRemoveCookies
	// Check request cookies for matches and add deletion Set-Cookie headers
	if len(a.outputRemoveCookies) > 0 {
		removedCookies := make(map[string]bool)
		for _, cookie := range req.Cookies() {
			cookieName := cookie.Name
			for _, re := range a.outputRemoveCookies {
				if re.MatchString(cookieName) {
					if !removedCookies[cookieName] {
						// Build deletion cookie with default Path/Domain
						deletionCookie := cookieName + "=; Path=/; Max-Age=0; HttpOnly; Secure"
						rw.Header().Add("Set-Cookie", deletionCookie)
						removedCookies[cookieName] = true
						println("Removing cookie:", cookieName)
					}
					break
				}
			}
		}
	}

	rw.WriteHeader(a.outputStatus)
	_, err := io.WriteString(rw, "Redirecting")
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
}

// extractCookieName extracts the cookie name from a Set-Cookie header value.
func extractCookieName(cookieStr string) string {
	// Cookie format: "name=value; attributes"
	// Find the first '=' to separate name from value
	parts := strings.SplitN(cookieStr, "=", 2)
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}
