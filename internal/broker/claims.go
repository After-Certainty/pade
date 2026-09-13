package broker

// Claims are the verified workload identity fields used for authorization.
// Email is intentionally omitted from policy matching.
type Claims struct {
	Subject  string
	Audience string
	// Issuer is the verified issuer URL from the selected trusted verifier.
	Issuer string
	// IssuerAlias is the operator-configured trusted-issuer alias that verified
	// the token. Empty for legacy single-issuer policies. Never derived solely
	// from an unverified JWT peek.
	IssuerAlias  string
	CloudAgentID string
	AgentRuntime string
	RepoURL      string
	RepoURLs     []string
	RepoCount    int
	OwnerUserID  string
	TeamID       string
	JTI          string
}
