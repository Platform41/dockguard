package version

import (
	"regexp"
	"strconv"
	"strings"
)

var pseudoVersionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]*)?0\.[0-9]{14}-[0-9a-fA-F]{12,}(?:\+[0-9A-Za-z.-]+)?$`)

func isPseudoVersion(version string) bool {
	return pseudoVersionPattern.MatchString(version)
}

type semanticVersion struct {
	major, minor, patch uint64
	prerelease          []string
}

// Newer reports whether latest is a later semantic version than current.
func Newer(current, latest string) bool {
	a, aOK := parseSemanticVersion(current)
	b, bOK := parseSemanticVersion(latest)
	return aOK && bOK && compareSemanticVersions(a, b) < 0
}

func parseSemanticVersion(value string) (semanticVersion, bool) {
	var parsed semanticVersion
	if !strings.HasPrefix(value, "v") {
		return parsed, false
	}
	value = strings.TrimPrefix(value, "v")
	if plus := strings.IndexByte(value, '+'); plus >= 0 {
		if plus == len(value)-1 {
			return parsed, false
		}
		value = value[:plus]
	}
	base := value
	if dash := strings.IndexByte(value, '-'); dash >= 0 {
		base = value[:dash]
		if dash == len(value)-1 {
			return parsed, false
		}
		parsed.prerelease = strings.Split(value[dash+1:], ".")
	}
	parts := strings.Split(base, ".")
	if len(parts) != 3 {
		return semanticVersion{}, false
	}
	numbers := []*uint64{&parsed.major, &parsed.minor, &parsed.patch}
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return semanticVersion{}, false
		}
		number, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return semanticVersion{}, false
		}
		*numbers[i] = number
	}
	for _, identifier := range parsed.prerelease {
		if identifier == "" || !validIdentifier(identifier) {
			return semanticVersion{}, false
		}
	}
	return parsed, true
}

func validIdentifier(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && r != '-' {
			return false
		}
	}
	return true
}

func compareSemanticVersions(a, b semanticVersion) int {
	for _, pair := range [][2]uint64{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	switch {
	case len(a.prerelease) == 0 && len(b.prerelease) > 0:
		return 1
	case len(a.prerelease) > 0 && len(b.prerelease) == 0:
		return -1
	}
	for i := 0; i < len(a.prerelease) && i < len(b.prerelease); i++ {
		x, y := a.prerelease[i], b.prerelease[i]
		if x == y {
			continue
		}
		xNumber, xErr := strconv.ParseUint(x, 10, 64)
		yNumber, yErr := strconv.ParseUint(y, 10, 64)
		switch {
		case xErr == nil && yErr == nil:
			if xNumber < yNumber {
				return -1
			}
			return 1
		case xErr == nil:
			return -1
		case yErr == nil:
			return 1
		case x < y:
			return -1
		default:
			return 1
		}
	}
	if len(a.prerelease) < len(b.prerelease) {
		return -1
	}
	if len(a.prerelease) > len(b.prerelease) {
		return 1
	}
	return 0
}
