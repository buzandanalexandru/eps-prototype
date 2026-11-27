// Package redisx builds every Redis key (U2):
//
//	eps:{<organiser>}:<event|_>:<kind>:<rest...>          organiser data (Key)
//	eps:{<organiser>:<event>:<shard>}:<kind>:<rest...>     hot on-sale keys (HotKey)
//	eps:platform:<kind>:<rest...>                          platform data (PlatformKey)
//
// The braces are the Redis Cluster hash tag. Ordinary organiser keys tag on the organiser, so one
// organiser's keys share a slot; hot keys (holds, counters, queues of one event, step 5) tag on
// organiser, event and shard, so a big on-sale spreads over many slots instead of one. Every key
// still starts with the organiser id, so isolation by prefix holds for both.
package redisx

import (
	"errors"
	"strings"

	"github.com/redis/go-redis/v9"
)

// NoEvent is the event placeholder for organiser-wide keys.
const NoEvent = "_"

var (
	ErrNoOrganiser = errors.New("redisx: organiser id is required")
	ErrNoKind      = errors.New("redisx: key kind is required")
	ErrBadPart     = errors.New("redisx: key parts must not be empty or contain ':', '{', '}' or spaces")
)

func validPart(s string) bool {
	return s != "" && !strings.ContainsAny(s, ":{} \t\r\n")
}

// Key builds an organiser key. event may be "" for organiser-wide keys (stored as "_").
func Key(organiserID, eventID, kind string, rest ...string) (string, error) {
	if organiserID == "" {
		return "", ErrNoOrganiser
	}
	if kind == "" {
		return "", ErrNoKind
	}
	if eventID == "" {
		eventID = NoEvent
	}
	parts := append([]string{organiserID, eventID, kind}, rest...)
	for _, p := range parts {
		if !validPart(p) {
			return "", ErrBadPart
		}
	}
	return "eps:{" + organiserID + "}:" + eventID + ":" + kind + joinRest(rest), nil
}

// HotKey builds a key for hot per-event state, tagged on organiser, event and shard. All three are
// required: a hot key without an event or shard would put a whole on-sale on one slot.
func HotKey(organiserID, eventID, shard, kind string, rest ...string) (string, error) {
	if organiserID == "" {
		return "", ErrNoOrganiser
	}
	if kind == "" {
		return "", ErrNoKind
	}
	if eventID == NoEvent {
		return "", ErrBadPart
	}
	for _, p := range append([]string{organiserID, eventID, shard, kind}, rest...) {
		if !validPart(p) {
			return "", ErrBadPart
		}
	}
	return "eps:{" + organiserID + ":" + eventID + ":" + shard + "}:" + kind + joinRest(rest), nil
}

// PlatformKey builds a key for platform data that belongs to no organiser.
func PlatformKey(kind string, rest ...string) (string, error) {
	if kind == "" {
		return "", ErrNoKind
	}
	for _, p := range append([]string{kind}, rest...) {
		if !validPart(p) {
			return "", ErrBadPart
		}
	}
	return "eps:platform:" + kind + joinRest(rest), nil
}

func joinRest(rest []string) string {
	if len(rest) == 0 {
		return ""
	}
	return ":" + strings.Join(rest, ":")
}

// Open connects to Redis from a URL such as redis://eps-redis:6379/0.
func Open(url string) (*redis.Client, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, errors.New("redisx: invalid REDIS_URL")
	}
	return redis.NewClient(opt), nil
}
