// Package redistest is an in-process stand-in for the few Redis commands the
// emote dictionary writers and readers use, so their unit tests run without a
// server.
//
// It follows the server's execution model where the tests depend on it:
//
//   - one command, or one whole MULTI/EXEC block, runs at a time under a lock
//     (Redis runs commands on a single thread), so a concurrent reader sees
//     exactly the interleavings a real server allows;
//   - a plain (non-transactional) pipeline runs command by command and lets
//     other clients run between its commands;
//   - an error injected while a MULTI block is queued aborts the whole block
//     with EXECABORT, as Redis does for an OOM rejection at queue time;
//   - keys expire on a fake clock moved with Advance.
//
// Lua scripts cannot run here. A test registers a Go stand-in for a script
// with HandleScript, keyed by the script's exact source, so editing the Lua
// without updating the stand-in fails with NOSCRIPT instead of passing.
package redistest

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ScriptFunc is the Go stand-in for one Lua script. It runs under the server
// lock, so it is atomic like the script it replaces.
type ScriptFunc func(st *State, keys []string, args []string) (any, error)

// Server is the fake keyspace plus a command log.
type Server struct {
	mu        sync.Mutex
	state     State
	scripts   map[string]ScriptFunc // sha1 hex -> stand-in
	fail      func(cmd []string) error
	between   func()
	published []Message
	log       [][]string
}

// Message is one PUBLISH.
type Message struct {
	Channel string
	Payload string
}

// State is the keyspace. Script stand-ins use it directly.
type State struct {
	now     time.Time
	hashes  map[string]map[string]string
	strs    map[string]string
	expires map[string]time.Time
}

// New returns an empty server whose clock starts at a fixed instant.
func New() *Server {
	return &Server{
		state: State{
			now:     time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC),
			hashes:  map[string]map[string]string{},
			strs:    map[string]string{},
			expires: map[string]time.Time{},
		},
		scripts: map[string]ScriptFunc{},
	}
}

// Client returns a go-redis client that never dials: every command is served
// by s through a hook.
func (s *Server) Client() *redis.Client {
	c := redis.NewClient(&redis.Options{Addr: "redistest.invalid:0", MaxRetries: -1})
	c.AddHook(hook{s: s})
	return c
}

// HandleScript registers fn as the stand-in for the Lua source src.
func (s *Server) HandleScript(src string, fn ScriptFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scripts[scriptSHA(src)] = fn
}

// FailWhen makes every command for which fn returns an error fail with it.
// Inside MULTI the failure is a queue-time rejection: the block aborts.
func (s *Server) FailWhen(fn func(cmd []string) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = fn
}

// BetweenPipelineCommands runs fn (outside the lock) after each command of a
// plain pipeline, to let a concurrent reader in where Redis would.
func (s *Server) BetweenPipelineCommands(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.between = fn
}

// Advance moves the clock forward and expires due keys.
func (s *Server) Advance(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.now = s.state.now.Add(d)
	s.state.expireDue()
}

// SeedHash stores a hash. ttl <= 0 stores it without an expiry, like the
// legacy dictionaries this package's callers must cope with.
func (s *Server) SeedHash(key string, fields map[string]string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := make(map[string]string, len(fields))
	for k, v := range fields {
		h[k] = v
	}
	s.state.del(key)
	s.state.hashes[key] = h
	if ttl > 0 {
		s.state.expires[key] = s.state.now.Add(ttl)
	}
}

// Hash returns a copy of the hash at key (nil when absent).
func (s *Server) Hash(key string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.expireDue()
	h, ok := s.state.hashes[key]
	if !ok {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = v
	}
	return out
}

// TTL returns the remaining time to live of key. exists is false for a
// missing key; ttl is negative for a key without an expiry.
func (s *Server) TTL(key string) (ttl time.Duration, exists bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.expireDue()
	if !s.state.exists(key) {
		return 0, false
	}
	at, ok := s.state.expires[key]
	if !ok {
		return -1, true
	}
	return at.Sub(s.state.now), true
}

// Keys returns every live key, sorted.
func (s *Server) Keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.expireDue()
	var out []string
	for k := range s.state.hashes {
		out = append(out, k)
	}
	for k := range s.state.strs {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// KeysWithoutTTL returns every live key that has no expiry.
func (s *Server) KeysWithoutTTL() []string {
	var out []string
	for _, k := range s.Keys() {
		if ttl, ok := s.TTL(k); ok && ttl < 0 {
			out = append(out, k)
		}
	}
	return out
}

// Published returns the PUBLISH messages so far.
func (s *Server) Published() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Message(nil), s.published...)
}

// Commands returns every command served so far, lower-cased, with MULTI and
// EXEC included for transactions.
func (s *Server) Commands() [][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]string, len(s.log))
	for i, c := range s.log {
		out[i] = append([]string(nil), c...)
	}
	return out
}

// --- State helpers (also used by script stand-ins) --------------------------

// Now is the fake clock.
func (st *State) Now() time.Time { return st.now }

// Exists reports whether key is live.
func (st *State) Exists(key string) bool { return st.exists(key) }

// HSet sets one field, creating the hash when absent.
func (st *State) HSet(key, field, value string) {
	h, ok := st.hashes[key]
	if !ok {
		h = map[string]string{}
		st.hashes[key] = h
	}
	h[field] = value
}

// HDel removes one field; the hash goes away with its last field.
func (st *State) HDel(key, field string) {
	h, ok := st.hashes[key]
	if !ok {
		return
	}
	delete(h, field)
	if len(h) == 0 {
		st.del(key)
	}
}

// PExpire sets key's expiry when key exists.
func (st *State) PExpire(key string, ttl time.Duration) bool {
	if !st.exists(key) {
		return false
	}
	st.expires[key] = st.now.Add(ttl)
	return true
}

func (st *State) exists(key string) bool {
	if _, ok := st.hashes[key]; ok {
		return true
	}
	_, ok := st.strs[key]
	return ok
}

func (st *State) del(key string) bool {
	existed := st.exists(key)
	delete(st.hashes, key)
	delete(st.strs, key)
	delete(st.expires, key)
	return existed
}

func (st *State) expireDue() {
	for k, at := range st.expires {
		if !st.now.Before(at) {
			st.del(k)
		}
	}
}

// --- go-redis hook ----------------------------------------------------------

type hook struct{ s *Server }

func (h hook) DialHook(redis.DialHook) redis.DialHook {
	return func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("redistest: no network")
	}
}

func (h hook) ProcessHook(redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, cmd redis.Cmder) error {
		h.s.mu.Lock()
		defer h.s.mu.Unlock()
		h.s.run(cmd, true)
		return cmd.Err()
	}
}

func (h hook) ProcessPipelineHook(redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(_ context.Context, cmds []redis.Cmder) error {
		if len(cmds) >= 2 && cmds[0].Name() == "multi" && cmds[len(cmds)-1].Name() == "exec" {
			h.s.runTx(cmds)
		} else {
			for _, cmd := range cmds {
				h.s.mu.Lock()
				h.s.run(cmd, true)
				between := h.s.between
				h.s.mu.Unlock()
				if between != nil {
					between()
				}
			}
		}
		for _, cmd := range cmds {
			if err := cmd.Err(); err != nil {
				return err
			}
		}
		return nil
	}
}

func (s *Server) runTx(cmds []redis.Cmder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	body := cmds[1 : len(cmds)-1]
	// Queue-time rejection (OOM, wrong arity...) aborts the whole block.
	for _, cmd := range body {
		if s.fail == nil {
			break
		}
		if err := s.fail(argStrings(cmd)); err != nil {
			s.log = append(s.log, []string{"multi"}, []string{"discard(execabort)"})
			abort := Error("EXECABORT Transaction discarded because of previous errors.")
			for _, c := range cmds {
				c.SetErr(abort)
			}
			cmd.SetErr(err)
			return
		}
	}
	s.log = append(s.log, []string{"multi"})
	setOK(cmds[0])
	for _, cmd := range body {
		s.run(cmd, false)
	}
	s.log = append(s.log, []string{"exec"})
	if sc, ok := cmds[len(cmds)-1].(*redis.SliceCmd); ok {
		sc.SetVal(nil)
	}
}

func (s *Server) run(cmd redis.Cmder, checkFail bool) {
	args := argStrings(cmd)
	s.log = append(s.log, args)
	if checkFail && s.fail != nil {
		if err := s.fail(args); err != nil {
			cmd.SetErr(err)
			return
		}
	}
	s.state.expireDue()
	if err := s.dispatch(cmd, args); err != nil {
		var re redis.Error
		if !errors.As(err, &re) && err != redis.Nil {
			err = Error(err.Error())
		}
		cmd.SetErr(err)
	}
}

func (s *Server) dispatch(cmd redis.Cmder, args []string) error {
	st := &s.state
	name := args[0]
	switch name {
	case "ping":
		setStatus(cmd, "PONG")
	case "del", "unlink":
		var n int64
		for _, k := range args[1:] {
			if st.del(k) {
				n++
			}
		}
		setInt(cmd, n)
	case "exists":
		var n int64
		for _, k := range args[1:] {
			if st.exists(k) {
				n++
			}
		}
		setInt(cmd, n)
	case "hset":
		if len(args) < 4 || len(args)%2 != 0 {
			return errors.New("ERR wrong number of arguments for 'hset' command")
		}
		if _, isStr := st.strs[args[1]]; isStr {
			return errors.New("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
		var added int64
		for i := 2; i < len(args); i += 2 {
			if h, ok := st.hashes[args[1]]; !ok || !hasField(h, args[i]) {
				added++
			}
			st.HSet(args[1], args[i], args[i+1])
		}
		setInt(cmd, added)
	case "hdel":
		var n int64
		for _, f := range args[2:] {
			if h, ok := st.hashes[args[1]]; ok && hasField(h, f) {
				n++
				st.HDel(args[1], f)
			}
		}
		setInt(cmd, n)
	case "hget":
		h := st.hashes[args[1]]
		if v, ok := h[args[2]]; ok {
			setString(cmd, v)
		} else {
			cmd.SetErr(redis.Nil)
		}
	case "hexists":
		_, ok := st.hashes[args[1]][args[2]]
		setBool(cmd, ok)
	case "hgetall":
		out := map[string]string{}
		for k, v := range st.hashes[args[1]] {
			out[k] = v
		}
		if c, ok := cmd.(*redis.MapStringStringCmd); ok {
			c.SetVal(out)
		}
	case "hlen":
		setInt(cmd, int64(len(st.hashes[args[1]])))
	case "hkeys":
		var keys []string
		for k := range st.hashes[args[1]] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if c, ok := cmd.(*redis.StringSliceCmd); ok {
			c.SetVal(keys)
		}
	case "get":
		if v, ok := st.strs[args[1]]; ok {
			setString(cmd, v)
		} else {
			cmd.SetErr(redis.Nil)
		}
	case "incr", "decr", "incrby":
		delta := int64(1)
		if name == "decr" {
			delta = -1
		}
		if name == "incrby" {
			d, err := strconv.ParseInt(args[2], 10, 64)
			if err != nil {
				return errors.New("ERR value is not an integer or out of range")
			}
			delta = d
		}
		cur := int64(0)
		if v, ok := st.strs[args[1]]; ok {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return errors.New("ERR value is not an integer or out of range")
			}
			cur = n
		}
		cur += delta
		st.strs[args[1]] = strconv.FormatInt(cur, 10)
		setInt(cmd, cur)
	case "expire", "pexpire":
		n, err := strconv.ParseInt(args[2], 10, 64)
		if err != nil {
			return errors.New("ERR value is not an integer or out of range")
		}
		unit := time.Second
		if name == "pexpire" {
			unit = time.Millisecond
		}
		setBool(cmd, st.expireWithOption(args[1], time.Duration(n)*unit, args[3:]))
	case "ttl", "pttl":
		if !st.exists(args[1]) {
			setDuration(cmd, -2)
			return nil
		}
		at, ok := st.expires[args[1]]
		if !ok {
			setDuration(cmd, -1)
			return nil
		}
		setDuration(cmd, at.Sub(st.now))
	case "rename":
		src, dst := args[1], args[2]
		if !st.exists(src) {
			return errors.New("ERR no such key")
		}
		h, isHash := st.hashes[src]
		str := st.strs[src]
		at, hasTTL := st.expires[src]
		st.del(src)
		st.del(dst)
		if isHash {
			st.hashes[dst] = h
		} else {
			st.strs[dst] = str
		}
		if hasTTL {
			st.expires[dst] = at
		}
		setStatus(cmd, "OK")
	case "publish":
		s.published = append(s.published, Message{Channel: args[1], Payload: args[2]})
		setInt(cmd, 0)
	case "evalsha", "eval":
		return s.eval(cmd, args)
	default:
		return fmt.Errorf("redistest: command %q not supported", name)
	}
	return nil
}

func (s *Server) eval(cmd redis.Cmder, args []string) error {
	if len(args) < 3 {
		return errors.New("ERR wrong number of arguments")
	}
	sha := args[1]
	if args[0] == "eval" {
		sha = scriptSHA(args[1])
	}
	fn, ok := s.scripts[sha]
	if !ok {
		if args[0] == "evalsha" {
			return redis.ErrNoScript
		}
		return errors.New("redistest: no stand-in registered for this script source")
	}
	nkeys, err := strconv.Atoi(args[2])
	if err != nil || nkeys < 0 || 3+nkeys > len(args) {
		return errors.New("ERR bad number of keys")
	}
	keys := args[3 : 3+nkeys]
	rest := args[3+nkeys:]
	v, err := fn(&s.state, keys, rest)
	if err != nil {
		return err
	}
	if c, ok := cmd.(*redis.Cmd); ok {
		c.SetVal(v)
	}
	return nil
}

// expireWithOption implements EXPIRE/PEXPIRE with NX, XX, GT and LT.
func (st *State) expireWithOption(key string, ttl time.Duration, opts []string) bool {
	if !st.exists(key) {
		return false
	}
	cur, hasTTL := st.expires[key]
	next := st.now.Add(ttl)
	for _, o := range opts {
		switch strings.ToLower(o) {
		case "nx":
			if hasTTL {
				return false
			}
		case "xx":
			if !hasTTL {
				return false
			}
		case "gt":
			// A key without an expiry counts as an infinite TTL.
			if !hasTTL || !next.After(cur) {
				return false
			}
		case "lt":
			if hasTTL && !next.Before(cur) {
				return false
			}
		}
	}
	if ttl <= 0 {
		st.del(key)
		return true
	}
	st.expires[key] = next
	return true
}

func hasField(h map[string]string, f string) bool {
	_, ok := h[f]
	return ok
}

func argStrings(cmd redis.Cmder) []string {
	raw := cmd.Args()
	out := make([]string, len(raw))
	for i, a := range raw {
		switch v := a.(type) {
		case string:
			out[i] = v
		case []byte:
			out[i] = string(v)
		default:
			out[i] = fmt.Sprint(v)
		}
	}
	if len(out) > 0 {
		out[0] = strings.ToLower(out[0])
	}
	return out
}

func scriptSHA(src string) string {
	sum := sha1.Sum([]byte(src))
	return hex.EncodeToString(sum[:])
}

func setOK(cmd redis.Cmder) { setStatus(cmd, "OK") }

func setStatus(cmd redis.Cmder, v string) {
	if c, ok := cmd.(*redis.StatusCmd); ok {
		c.SetVal(v)
	}
}

func setInt(cmd redis.Cmder, v int64) {
	switch c := cmd.(type) {
	case *redis.IntCmd:
		c.SetVal(v)
	case *redis.Cmd:
		c.SetVal(v)
	}
}

func setBool(cmd redis.Cmder, v bool) {
	switch c := cmd.(type) {
	case *redis.BoolCmd:
		c.SetVal(v)
	case *redis.IntCmd:
		if v {
			c.SetVal(1)
		} else {
			c.SetVal(0)
		}
	}
}

func setString(cmd redis.Cmder, v string) {
	switch c := cmd.(type) {
	case *redis.StringCmd:
		c.SetVal(v)
	case *redis.Cmd:
		c.SetVal(v)
	}
}

// setDuration answers TTL/PTTL. Like go-redis's reply parser, -1 (no expiry)
// and -2 (no key) are raw values, not scaled by the command's precision.
func setDuration(cmd redis.Cmder, d time.Duration) {
	if c, ok := cmd.(*redis.DurationCmd); ok {
		c.SetVal(d)
	}
}

// Error is a server-side error reply (go-redis treats it as a Redis error,
// not a network failure).
type Error string

func (e Error) Error() string { return string(e) }

// RedisError marks Error as a Redis reply error for go-redis.
func (Error) RedisError() {}
