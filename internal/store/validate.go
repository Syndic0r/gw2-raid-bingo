package store

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Input limits. These bound both storage growth and, for a public multi-tenant
// bot, per-guild abuse. They are enforced in the store so every caller (bot and
// web) is protected identically.
const (
	MaxEntryTextLen   = 200
	MaxPoolNameLen    = 60
	MaxPoolSlugLen    = 40
	MaxSharedPools    = 50
	MaxEntriesPerPool = 500
)

// ErrValidation marks input that failed a limit or format check. It wraps a
// human-readable reason suitable for showing to the user.
var ErrValidation = errors.New("validation")

func validationErr(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrValidation, fmt.Sprintf(format, args...))
}

// cleanText trims surrounding whitespace and validates an entry's text.
func cleanText(text string) (string, error) {
	t := strings.TrimSpace(text)
	if t == "" {
		return "", validationErr("text must not be empty")
	}
	if !utf8.ValidString(t) {
		return "", validationErr("text must be valid UTF-8")
	}
	if utf8.RuneCountInString(t) > MaxEntryTextLen {
		return "", validationErr("text must be at most %d characters", MaxEntryTextLen)
	}
	return t, nil
}

// cleanPoolName validates a shared pool's display name.
func cleanPoolName(name string) (string, error) {
	n := strings.TrimSpace(name)
	if n == "" {
		return "", validationErr("pool name must not be empty")
	}
	if utf8.RuneCountInString(n) > MaxPoolNameLen {
		return "", validationErr("pool name must be at most %d characters", MaxPoolNameLen)
	}
	return n, nil
}

// cleanSlug validates a shared pool slug: lowercase letters, digits, and hyphens.
func cleanSlug(slug string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(slug))
	if s == "" {
		return "", validationErr("pool slug must not be empty")
	}
	if len(s) > MaxPoolSlugLen {
		return "", validationErr("pool slug must be at most %d characters", MaxPoolSlugLen)
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return "", validationErr("pool slug may contain only a-z, 0-9 and hyphens")
		}
	}
	return s, nil
}
