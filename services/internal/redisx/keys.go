// Package redisx builds every Redis key (U2). Organiser data lives under
// eps:{<organiser>}:<event|_>:<kind>:<rest...>; platform data under eps:platform:<kind>:<rest...>.
// The braces are a Redis Cluster hash tag, so one organiser's keys stay on one slot group.
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
