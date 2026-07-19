// Package bingo holds the pure game domain: the playable instances, card
// generation, and win detection. It has no database or network dependencies so
// it can be exercised entirely by unit tests.
package bingo

import "fmt"

// Instance is a playable encounter a bingo game can be run for: the eight raid
// wings (w1-w8) and Harvest Temple CM (htcm).
type Instance string

const (
	W1   Instance = "w1"
	W2   Instance = "w2"
	W3   Instance = "w3"
	W4   Instance = "w4"
	W5   Instance = "w5"
	W6   Instance = "w6"
	W7   Instance = "w7"
	W8   Instance = "w8"
	HTCM Instance = "htcm"
)

// instances is the canonical ordered list of every playable instance.
var instances = []Instance{W1, W2, W3, W4, W5, W6, W7, W8, HTCM}

// labels are the human-facing names, shown in Discord and on the website.
var labels = map[Instance]string{
	W1:   "Wing 1 - Spirit Vale",
	W2:   "Wing 2 - Salvation Pass",
	W3:   "Wing 3 - Stronghold of the Faithful",
	W4:   "Wing 4 - Bastion of the Penitent",
	W5:   "Wing 5 - Hall of Chains",
	W6:   "Wing 6 - Mythwright Gambit",
	W7:   "Wing 7 - The Key of Ahdashim",
	W8:   "Wing 8 - Mount Balrior",
	HTCM: "Harvest Temple CM",
}

// Instances returns the canonical ordered list of playable instances. The
// returned slice is a copy the caller may modify freely.
func Instances() []Instance {
	out := make([]Instance, len(instances))
	copy(out, instances)
	return out
}

// ParseInstance validates s and returns the corresponding Instance. It is the
// single gate every external input (slash-command option, URL path, API body)
// must pass through, so an unknown instance can never reach the store.
func ParseInstance(s string) (Instance, error) {
	for _, inst := range instances {
		if string(inst) == s {
			return inst, nil
		}
	}
	return "", fmt.Errorf("unknown instance %q", s)
}

// Valid reports whether i is a known instance.
func (i Instance) Valid() bool {
	_, ok := labels[i]
	return ok
}

// Label returns the human-facing name, or the raw key if unknown.
func (i Instance) Label() string {
	if l, ok := labels[i]; ok {
		return l
	}
	return string(i)
}

// String implements fmt.Stringer.
func (i Instance) String() string { return string(i) }
