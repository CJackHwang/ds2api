package config

import (
	"bufio"
	"context"
	"encoding/json"
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

const defaultRedisConfigKey = "ds2api:config"

type redisConfigStore struct {
	key      string
	endpoint redisEndpoint
}

type redisEndpoint struct {
	network  string
	addr     string
	username string
	password string
	db       int
}

func redisEnabled() bool {
	return strings.TrimSpace(os.Getenv("REDIS_URL")) != ""
}

func redisConfigKey() string {
	if key := strings.TrimSpace(os.Getenv("DS2API_REDIS_CONFIG_KEY")); key != "" {
		return key
	}
	return defaultRedisConfigKey
}

func loadConfigFromRedis() (Config, *redisConfigStore, error) {
	store, err := newRedisConfigStore(strings.TrimSpace(os.Getenv("REDIS_URL")), redisConfigKey())
	if err != nil {
		return Config{}, nil, err
	}

	raw, err := store.LoadJSON()
	if err != nil {
		return Config{}, store, err
	}
	if strings.TrimSpace(raw) == "" {
		return Config{}, store, nil
	}

	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return Config{}, store, err
	}
	cfg.DropInvalidAccounts()
	return cfg, store, nil
}

func newRedisConfigStore(redisURL, key string) (*redisConfigStore, error) {
	ep, err := parseRedisURL(redisURL)
	if err != nil {
		return nil, err
	}
	rs := &redisConfigStore{key: key, endpoint: ep}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rs.withConn(ctx, func(conn net.Conn, rw *bufio.ReadWriter) error {
		return writeCommandAndExpectOK(rw, "PING")
	}); err != nil {
		return nil, err
	}
	return rs, nil
}

func parseRedisURL(raw string) (redisEndpoint, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return redisEndpoint{}, err
	}
	if u.Scheme != "redis" {
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
	}, nil
}

func (r *redisConfigStore) LoadJSON() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out := ""
	err := r.withConn(ctx, func(conn net.Conn, rw *bufio.ReadWriter) error {
		v, err := writeCommandAndReadBulkString(rw, "GET", r.key)
		if err != nil {
			return err
		}
		out = v
		return nil
	})
	return out, err
}

func (r *redisConfigStore) SaveJSON(raw string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return r.withConn(ctx, func(conn net.Conn, rw *bufio.ReadWriter) error {
		return writeCommandAndExpectOK(rw, "SET", r.key, raw)
	})
}

func (r *redisConfigStore) withConn(ctx context.Context, fn func(net.Conn, *bufio.ReadWriter) error) error {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, r.endpoint.network, r.endpoint.addr)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			Logger.Warn("[config] redis close failed", "error", closeErr)
		}
	}()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return err
		}
	}
	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	if r.endpoint.password != "" {
		var authErr error
		if r.endpoint.username != "" {
			authErr = writeCommandAndExpectOK(rw, "AUTH", r.endpoint.username, r.endpoint.password)
		} else {
			authErr = writeCommandAndExpectOK(rw, "AUTH", r.endpoint.password)
		}
		if authErr != nil {
			return authErr
		}
	}
	if r.endpoint.db > 0 {
		if err := writeCommandAndExpectOK(rw, "SELECT", strconv.Itoa(r.endpoint.db)); err != nil {
			return err
		}
	}
	return fn(conn, rw)
}

func writeCommandAndExpectOK(rw *bufio.ReadWriter, parts ...string) error {
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
	if !strings.HasPrefix(line, "+OK") && !strings.HasPrefix(line, "+PONG") {
		return fmt.Errorf("unexpected redis response: %s", strings.TrimSpace(line))
	}
	return nil
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
