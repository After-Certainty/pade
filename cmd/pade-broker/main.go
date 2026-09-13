package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/After-Certainty/pade/internal/binding"
	"github.com/After-Certainty/pade/internal/broker"
	"github.com/After-Certainty/pade/internal/providerset"
	"github.com/After-Certainty/pade/internal/version"
)

func main() {
	var (
		showVersion = flag.Bool("version", false, "print version and exit")
		listen      = flag.String("listen", "",
			"listen address. Empty: use PORT env as 0.0.0.0:PORT, else "+broker.DefaultListenAddr+". Non-loopback plaintext requires -tls-termination=proxy or -tls-cert/-tls-key")
		policy   = flag.String("policy", "", "path to broker-policy.yaml (required)")
		bindings = flag.String("bindings", "", "path to server-side bindings.yaml (required)")
		certFile = flag.String("tls-cert", "", "TLS certificate file (broker-managed TLS; use with -tls-key)")
		keyFile  = flag.String("tls-key", "", "TLS private key file (broker-managed TLS; use with -tls-cert)")
		tlsTerm  = flag.String("tls-termination", "",
			`TLS ownership model: empty (default safe auto) or "proxy". `+
				`proxy asserts that a trusted upstream (Cloud Run, ingress, load balancer, etc.) terminates TLS `+
				`and that this plaintext listener is reachable only inside that deployment boundary. `+
				`PADE does not verify the proxy. Incompatible with -tls-cert/-tls-key.`)
		resolveTimeout = flag.Duration("resolve-timeout", 25*time.Second,
			"maximum time for a single /v1/resolve after accepting the request (default 25s, under HTTP write timeout)")
		maxConcurrent = flag.Int("max-concurrent-resolves", 32,
			"maximum in-flight /v1/resolve handlers; excess requests get 503 busy (no queue)")
	)
	flag.Parse()
	if *showVersion {
		fmt.Println(version.String())
		return
	}
	if *policy == "" || *bindings == "" {
		fmt.Fprintln(os.Stderr, "usage: pade-broker -policy FILE -bindings FILE [-listen ADDR] [-tls-cert FILE -tls-key FILE | -tls-termination=proxy] [-resolve-timeout DURATION] [-max-concurrent-resolves N]")
		os.Exit(2)
	}

	addr, err := broker.ResolveListenAddr(*listen, os.Getenv("PORT"))
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	listenCfg := broker.ListenConfig{
		Addr:           addr,
		CertFile:       *certFile,
		KeyFile:        *keyFile,
		TLSTermination: *tlsTerm,
	}
	mode, err := listenCfg.Validate()
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	pol, err := broker.LoadPolicy(*policy)
	if err != nil {
		log.Fatalf("policy: %v", err)
	}
	bindCfg, err := binding.Load(*bindings)
	if err != nil {
		log.Fatalf("bindings: %v", err)
	}

	trusted, err := pol.TrustedIssuers()
	if err != nil {
		log.Fatalf("policy oidc: %v", err)
	}
	// Legacy single-issuer Cursor default JWKS only — do not invent JWKS for
	// multi-issuer or non-Cursor issuers.
	if !pol.MultiIssuer() && len(trusted) == 1 && trusted[0].JWKSURL == "" {
		trusted[0].JWKSURL = "https://api.cursor.com/keys"
	}
	verifier, err := broker.NewVerifierSet(trusted)
	if err != nil {
		log.Fatalf("verifier: %v", err)
	}
	srv := &broker.Server{
		Policy:         pol,
		Verifier:       verifier,
		Registry:       providerset.Broker(),
		Bindings:       bindCfg,
		Logger:         log.New(os.Stderr, "pade-broker: ", log.LstdFlags),
		ResolveTimeout: *resolveTimeout,
		MaxConcurrent:  *maxConcurrent,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	issuerDesc := pol.OIDC.Issuer
	if pol.MultiIssuer() {
		aliases := make([]string, 0, len(pol.OIDC.Issuers))
		for a := range pol.OIDC.Issuers {
			aliases = append(aliases, a)
		}
		issuerDesc = "issuers=[" + strings.Join(aliases, ",") + "]"
	}
	log.Printf("pade-broker listening on %s transport=%s (%s)", addr, mode, issuerDesc)
	if err := broker.ListenAndServe(ctx, listenCfg, srv.Handler()); err != nil && err != context.Canceled {
		log.Fatal(err)
	}
}
