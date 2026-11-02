package try

// Origin is the address the trial is served on.
//
// Govard's proxy redirects every plain HTTP request to HTTPS, so the storefront
// only exists on https://<domain>; there is no usable loopback URL to fall back
// to. Trusted and Resolvable describe the environment rather than the address,
// so the caller can tell the user what a browser will do.
type Origin struct {
	URL        string
	HostHeader string
	TLS        bool
	Trusted    bool
	Resolvable bool
}

// ResolveOrigin describes how a browser will reach the trial.
//
// HTTPS on the project domain is the only served origin. When the name does not
// resolve locally, or the local CA is not trusted, the address still exists but
// the caller must say so: a printed URL that cannot open is worse than none.
func ResolveOrigin(domain string, resolvable func(string) bool, caTrusted func(string) bool) Origin {
	canResolve := resolvable != nil && resolvable(domain)
	trusted := caTrusted != nil && caTrusted(domain)

	return Origin{
		URL:        "https://" + domain,
		HostHeader: domain,
		TLS:        true,
		Trusted:    trusted,
		Resolvable: canResolve,
	}
}
