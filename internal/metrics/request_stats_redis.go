package metrics

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultStatsSuccessKey = "ds2api:stats:success"
	defaultStatsFailedKey  = "ds2api:stats:failed"
	legacyStatsSuccessKey  = "ds2api:stats:success_calls"
	legacyStatsFailedKey   = "ds2api:stats:failed_calls"
	defaultStatsHashKey    = "ds2api:stats"
)

type redisCounterStore struct {
	endpoint   redisEndpoint
	successKey string
	failedKey  string
}

type redisEndpoint struct {
	network  string
	addr     string
	username string
	password string
	db       int
	useTLS   bool
	host     string
}

func newRedisCounterStoreFromEnv() *redisCounterStore {
	rawURL := strings.TrimSpace(os.Getenv("REDIS_URL"))
	if rawURL == "" {
		return nil
	}
	ep, err := parseRedisURL(rawURL)
	if err != nil {
		return nil
	}
	store := &redisCounterStore{
		endpoint:   ep,
		successKey: envOr("DS2API_STATS_SUCCESS_KEY", defaultStatsSuccessKey),
		failedKey:  envOr("DS2API_STATS_FAILED_KEY", defaultStatsFailedKey),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := store.withConn(ctx, func(rw *bufio.ReadWriter) error {
		return writeCommandAndExpectSimple(rw, "+PONG", "PING")
	}); err != nil {
		return nil
	}
	return store
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func (r *redisCounterStore) IncrSuccess() error {
	return r.incr(r.successKey, defaultStatsHashKey, "success")
}

func (r *redisCounterStore) IncrFailed() error {
	return r.incr(r.failedKey, defaultStatsHashKey, "failed")
}

func (r *redisCounterStore) Snapshot() (int64, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var success int64
	var failed int64
	err := r.withConn(ctx, func(rw *bufio.ReadWriter) error {
		var err error
		success, err = readCounterWithFallback(
			rw,
			[]string{r.successKey, legacyStatsSuccessKey},
			[]hashCounterLookup{
				{key: defaultStatsHashKey, fields: []string{"success", "success_calls", "successCount"}},
				{key: r.successKey, fields: []string{"success", "success_calls", "value"}},
			},
		)
		if err != nil {
			return err
		}
		failed, err = readCounterWithFallback(
			rw,
			[]string{r.failedKey, legacyStatsFailedKey},
			[]hashCounterLookup{
				{key: defaultStatsHashKey, fields: []string{"failed", "failed_calls", "failedCount"}},
				{key: r.failedKey, fields: []string{"failed", "failed_calls", "value"}},
			},
		)
		return err
	})
	return success, failed, err
}

func (r *redisCounterStore) incr(key, hashKey, field string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return r.withConn(ctx, func(rw *bufio.ReadWriter) error {
		_, err := writeCommandAndReadInteger(rw, "INCR", key)
		if err != nil {
			return err
		}
		if strings.TrimSpace(hashKey) != "" && strings.TrimSpace(field) != "" {
			_, err = writeCommandAndReadInteger(rw, "HINCRBY", hashKey, field, "1")
			if err != nil && !isWrongTypeErr(err) {
				return err
			}
		}
		return nil
	})
}

func readCounter(rw *bufio.ReadWriter, key string) (int64, error) {
	val, err := writeCommandAndReadBulkString(rw, "GET", key)
	if err != nil {
		if isWrongTypeErr(err) {
			return 0, nil
		}
		return 0, err
	}
	if strings.TrimSpace(val) == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
	if err != nil {
		return 0, err
	}
	return n, nil
}

type hashCounterLookup struct {
	key    string
	fields []string
}

func readCounterWithFallback(rw *bufio.ReadWriter, keys []string, hashLookups []hashCounterLookup) (int64, error) {
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			continue
		}
		v, err := readCounter(rw, key)
		if err != nil {
			return 0, err
		}
		if v != 0 {
			return v, nil
		}
	}
	for _, lookup := range hashLookups {
		if strings.TrimSpace(lookup.key) == "" || len(lookup.fields) == 0 {
			continue
		}
		v, err := readCounterFromHash(rw, lookup.key, lookup.fields...)
		if err != nil {
			return 0, err
		}
		if v != 0 {
			return v, nil
		}
	}
	return 0, nil
}

func readCounterFromHash(rw *bufio.ReadWriter, hashKey string, fields ...string) (int64, error) {
	for _, field := range fields {
		if strings.TrimSpace(field) == "" {
			continue
		}
		val, err := writeCommandAndReadBulkString(rw, "HGET", hashKey, field)
		if err != nil {
			if isWrongTypeErr(err) {
				return 0, nil
			}
			return 0, err
		}
		if strings.TrimSpace(val) == "" {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		if err != nil {
			return 0, err
		}
		return n, nil
	}
	return 0, nil
}

func isWrongTypeErr(err error) bool {
	return err != nil && strings.Contains(strings.ToUpper(err.Error()), "WRONGTYPE")
}

func (r *redisCounterStore) withConn(ctx context.Context, fn func(*bufio.ReadWriter) error) error {
	dialer := net.Dialer{}
	var conn net.Conn
	var err error
	if r.endpoint.useTLS {
		tlsDialer := tls.Dialer{
			NetDialer: &dialer,
			Config: &tls.Config{
				MinVersion: tls.VersionTLS12,
				ServerName: r.endpoint.host,
			},
		}
		conn, err = tlsDialer.DialContext(ctx, r.endpoint.network, r.endpoint.addr)
	} else {
		conn, err = dialer.DialContext(ctx, r.endpoint.network, r.endpoint.addr)
	}
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return err
		}
	}

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	if r.endpoint.password != "" {
		if r.endpoint.username != "" {
			if err := writeCommandAndExpectSimple(rw, "+OK", "AUTH", r.endpoint.username, r.endpoint.password); err != nil {
				return err
			}
		} else {
			if err := writeCommandAndExpectSimple(rw, "+OK", "AUTH", r.endpoint.password); err != nil {
				return err
			}
		}
	}
	if r.endpoint.db > 0 {
		if err := writeCommandAndExpectSimple(rw, "+OK", "SELECT", strconv.Itoa(r.endpoint.db)); err != nil {
			return err
		}
	}
	return fn(rw)
}

func parseRedisURL(raw string) (redisEndpoint, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return redisEndpoint{}, err
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	switch scheme {
	case "redis", "wredis":
	case "rediss", "wrediss":
	default:
		return redisEndpoint{}, fmt.Errorf("unsupported REDIS_URL scheme: %s", u.Scheme)
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return redisEndpoint{}, errors.New("invalid REDIS_URL: empty host")
	}
	port := u.Port()
	if port == "" {
		port = "6379"
	}
	db := 0
	path := strings.Trim(strings.TrimSpace(u.Path), "/")
	if path != "" {
		n, err := strconv.Atoi(path)
		if err != nil {
			return redisEndpoint{}, fmt.Errorf("invalid redis db index %q: %w", path, err)
		}
		if n < 0 {
			return redisEndpoint{}, fmt.Errorf("invalid redis db index %d", n)
		}
		db = n
	}
	username := ""
	password := ""
	if u.User != nil {
		username = u.User.Username()
		if p, ok := u.User.Password(); ok {
			password = p
		}
	}
	return redisEndpoint{
		network:  "tcp",
		addr:     net.JoinHostPort(host, port),
		username: username,
		password: password,
		db:       db,
		useTLS:   scheme == "rediss" || scheme == "wrediss",
		host:     host,
	}, nil
}

func writeCommand(rw *bufio.ReadWriter, parts ...string) error {
	if _, err := rw.WriteString(fmt.Sprintf("*%d\r\n", len(parts))); err != nil {
		return err
	}
	for _, part := range parts {
		if _, err := rw.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(part), part)); err != nil {
			return err
		}
	}
	return rw.Flush()
}

func writeCommandAndExpectSimple(rw *bufio.ReadWriter, expectPrefix string, parts ...string) error {
	if err := writeCommand(rw, parts...); err != nil {
		return err
	}
	line, err := rw.ReadString('\n')
	if err != nil {
		return err
	}
	if strings.HasPrefix(line, "-") {
		return errors.New(strings.TrimSpace(strings.TrimPrefix(line, "-")))
	}
	if !strings.HasPrefix(line, expectPrefix) {
		return fmt.Errorf("unexpected redis response: %s", strings.TrimSpace(line))
	}
	return nil
}

func writeCommandAndReadInteger(rw *bufio.ReadWriter, parts ...string) (int64, error) {
	if err := writeCommand(rw, parts...); err != nil {
		return 0, err
	}
	line, err := rw.ReadString('\n')
	if err != nil {
		return 0, err
	}
	if strings.HasPrefix(line, "-") {
		return 0, errors.New(strings.TrimSpace(strings.TrimPrefix(line, "-")))
	}
	if !strings.HasPrefix(line, ":") {
		return 0, fmt.Errorf("unexpected redis response: %s", strings.TrimSpace(line))
	}
	return strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, ":")), 10, 64)
}

func writeCommandAndReadBulkString(rw *bufio.ReadWriter, parts ...string) (string, error) {
	if err := writeCommand(rw, parts...); err != nil {
		return "", err
	}
	header, err := rw.ReadString('\n')
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(header, "-") {
		return "", errors.New(strings.TrimSpace(strings.TrimPrefix(header, "-")))
	}
	if strings.HasPrefix(header, "$-1") {
		return "", nil
	}
	if !strings.HasPrefix(header, "$") {
		return "", fmt.Errorf("unexpected redis response: %s", strings.TrimSpace(header))
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(header, "$")))
	if err != nil {
		return "", err
	}
	if n < 0 {
		return "", nil
	}
	buf := make([]byte, n+2)
	if _, err := io.ReadFull(rw, buf); err != nil {
		return "", err
	}
	return string(buf[:n]), nil
}
