// Package server wires together storage, auth, and the HTTP API into a
// runnable Cinc Collector instance. It is the same package used by the
// cinc-collector binary and by tests that embed a server directly.
package server

import "time"

// Options configures a Server. The zero value is not valid; callers should
// start from Options{} and set at least DB.DSN, then call New.
type Options struct {
	// ListenAddr is the TCP address the HTTP server binds to, e.g. ":8891".
	ListenAddr string

	// TLSCertFile and TLSKeyFile, if both set, enable TLS termination.
	TLSCertFile string
	TLSKeyFile  string

	DB DBOptions

	Storage StorageOptions

	// StatsUser and StatsPassword gate GET /_stats with HTTP Basic auth.
	// If StatsPassword is empty, a random password is generated at startup
	// and printed once, matching cinc-server-ng's approach: no default
	// password is ever baked into the binary.
	StatsUser     string
	StatsPassword string

	// BodyLimit caps request body size, applied to every route.
	BodyLimit int64

	ShutdownDelay   time.Duration
	ShutdownTimeout time.Duration

	// Now, if set, overrides time.Now for testing.
	Now func() time.Time
}

// DBOptions configures the connection to the SereneDB instance backing this
// server. SereneDB speaks the PostgreSQL wire protocol, so DSN is a normal
// postgres:// connection string.
type DBOptions struct {
	DSN      string
	MaxConns int32
}

// StorageOptions configures the blob storage backend used for InSpec
// profile tarballs.
type StorageOptions struct {
	// Backend selects the blob storage implementation: "local" or "s3".
	Backend string

	// Local backend options.
	LocalPath string

	// S3 backend options (also used for MinIO via a custom Endpoint).
	S3Bucket   string
	S3Endpoint string
	S3Region   string
}

func (o *Options) withDefaults() Options {
	out := *o
	if out.ListenAddr == "" {
		out.ListenAddr = ":8891"
	}
	if out.DB.MaxConns == 0 {
		out.DB.MaxConns = 10
	}
	if out.Storage.Backend == "" {
		out.Storage.Backend = "local"
	}
	if out.Storage.LocalPath == "" {
		out.Storage.LocalPath = "./data/profiles"
	}
	if out.BodyLimit == 0 {
		out.BodyLimit = 64 << 20 // 64 MiB: generous for converge/compliance JSON payloads.
	}
	if out.ShutdownTimeout == 0 {
		out.ShutdownTimeout = 30 * time.Second
	}
	if out.Now == nil {
		out.Now = time.Now
	}
	return out
}
