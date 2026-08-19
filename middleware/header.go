package middleware

import (
	"encoding/base64"
	"net/http"
	"strings"
)

// Header middleware adds a header to request. Headers carrying credentials, i.e. Authorization, Www-Authenticate,
// Cookie, Cookie2, Proxy-Authorization and Proxy-Authenticate, are dropped as soon as a redirect leaves the host the
// request started from, the same way the standard client treats them. Any other header, a custom one carrying a
// secret included, is set on every hop, see SecretHeader for the protected version.
func Header(key, value string) func(http.RoundTripper) http.RoundTripper {
	return headerHandler(key, value, credentialHeader(key))
}

// SecretHeader middleware adds a header carrying a credential to request. The header is set while the redirect chain
// stays on the host the request started from, or on one of its subdomains, and dropped once the chain leaves it,
// including a value the caller set on the request itself.
func SecretHeader(key, value string) func(http.RoundTripper) http.RoundTripper {
	return headerHandler(key, value, true)
}

// JSON sets Content-Type and Accept headers to json
func JSON(next http.RoundTripper) http.RoundTripper {
	fn := func(req *http.Request) (*http.Response, error) {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		return next.RoundTrip(req)
	}
	return RoundTripperFunc(fn)
}

// BasicAuth middleware adds basic auth to request. Credentials are set while the redirect chain stays on the host
// the request started from, or on one of its subdomains, and dropped once the chain leaves it.
func BasicAuth(user, passwd string) func(http.RoundTripper) http.RoundTripper {
	return func(next http.RoundTripper) http.RoundTripper {
		value := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+passwd))
		fn := func(req *http.Request) (*http.Response, error) {
			if !onOriginalHost(req) {
				dropHeader(req.Header, "Authorization", value)
				return next.RoundTrip(req)
			}
			req.SetBasicAuth(user, passwd)
			return next.RoundTrip(req)
		}
		return RoundTripperFunc(fn)
	}
}

// headerHandler makes the middleware setting a single header, secret ones only while the request is on its original host
func headerHandler(key, value string, secret bool) func(http.RoundTripper) http.RoundTripper {
	return func(next http.RoundTripper) http.RoundTripper {
		fn := func(req *http.Request) (*http.Response, error) {
			if secret && !onOriginalHost(req) {
				dropHeader(req.Header, key, value)
				return next.RoundTrip(req)
			}
			req.Header.Set(key, value)
			return next.RoundTrip(req)
		}
		return RoundTripperFunc(fn)
	}
}

// dropHeader removes the credential from the request going to a host outside of the original one.
//
// The client copies the headers of the original request to every hop, dropping the credential ones on the way out,
// so for a key it doesn't recognise as a credential the whole header goes, the copy of the caller's own value
// included. For the keys it does recognise, values on this hop can belong to the destination instead, set by the
// cookie jar or by a CheckRedirect hook, so only the value the middleware sets is removed.
func dropHeader(header http.Header, key, value string) {
	if !credentialHeader(key) {
		header.Del(key)
		return
	}

	vals := header.Values(key)
	kept := make([]string, 0, len(vals))
	for _, v := range vals {
		if v != value {
			kept = append(kept, v)
		}
	}
	if len(kept) == 0 {
		header.Del(key)
		return
	}
	header[http.CanonicalHeaderKey(key)] = kept
}

// credentialHeader reports if the header carries credentials, the set matching the one the standard client
// strips on a redirect to another host
func credentialHeader(key string) bool {
	switch http.CanonicalHeaderKey(key) {
	case "Authorization", "Www-Authenticate", "Cookie", "Cookie2", "Proxy-Authorization", "Proxy-Authenticate":
		return true
	}
	return false
}

// onOriginalHost reports if the request is still on the host the redirect chain started from, or on one of its
// subdomains. A request outside of a redirect chain is always on its own host. Once the chain left the original
// host the result stays negative for the rest of it, matching the standard client.
//
// The chain is walked over Response.Request, set by the standard transport and expected from any custom one.
// A redirect the origin can't be established for is treated as a hop away from it, and so is a hop between the
// unicode and the punycode form of the same internationalised host, which is compared as it is written.
func onOriginalHost(req *http.Request) bool {
	if req.Response == nil { // not a redirect
		return true
	}

	origin := req
	for origin.Response != nil {
		if origin.Response.Request == nil { // broken chain, the origin is unknown
			return false
		}
		origin = origin.Response.Request
	}

	originHost := strings.ToLower(origin.URL.Hostname())
	for r := req; r != origin; r = r.Response.Request {
		if !domainOrSubdomain(strings.ToLower(r.URL.Hostname()), originHost) {
			return false
		}
	}
	return true
}

// domainOrSubdomain reports whether sub is the same domain as parent or a subdomain of it
func domainOrSubdomain(sub, parent string) bool {
	if sub == parent {
		return true
	}
	if strings.ContainsAny(sub, ":%") { // IPv6 address or a zone, never a hostname
		return false
	}
	if !strings.HasSuffix(sub, parent) {
		return false
	}
	return sub[len(sub)-len(parent)-1] == '.'
}
