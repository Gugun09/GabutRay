package profile

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadAll(path string) ([]Profile, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profiles %s: %w", path, err)
	}
	var profiles []Profile
	if err := yaml.Unmarshal(data, &profiles); err != nil {
		return nil, fmt.Errorf("parse profiles %s: %w", path, err)
	}
	return profiles, nil
}

func SaveAll(path string, profiles []Profile) error {
	data, err := yaml.Marshal(profiles)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write profiles %s: %w", path, err)
	}
	return nil
}

func Add(path string, item Profile) (Profile, error) {
	profiles, err := LoadAll(path)
	if err != nil {
		return Profile{}, err
	}
	base := Slugify(item.Name)
	item.ID = uniqueID(profiles, base, item.RawLink)
	for i := range profiles {
		if profiles[i].ID == item.ID {
			profiles[i] = item
			return item, SaveAll(path, profiles)
		}
	}
	profiles = append(profiles, item)
	return item, SaveAll(path, profiles)
}

func Remove(path, target string) (Profile, error) {
	profiles, err := LoadAll(path)
	if err != nil {
		return Profile{}, err
	}
	for i, item := range profiles {
		if item.ID == target || item.Name == target {
			profiles = append(profiles[:i], profiles[i+1:]...)
			return item, SaveAll(path, profiles)
		}
	}
	return Profile{}, fmt.Errorf("profile not found: %s", target)
}

func Find(path, target string) (Profile, error) {
	profiles, err := LoadAll(path)
	if err != nil {
		return Profile{}, err
	}
	for _, item := range profiles {
		if item.ID == target || item.Name == target {
			return item, nil
		}
	}
	return Profile{}, fmt.Errorf("profile not found: %s", target)
}

func FromTarget(path, target string) (Profile, error) {
	if strings.HasPrefix(target, "vless://") || strings.HasPrefix(target, "vmess://") || strings.HasPrefix(target, "trojan://") {
		parsed, err := ParseShareLink(target)
		if err != nil {
			return Profile{}, err
		}
		return Add(path, parsed)
	}
	return Find(path, target)
}

func Slugify(value string) string {
	var out strings.Builder
	prevDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			out.WriteRune(toLowerASCII(r))
			prevDash = false
			continue
		}
		if !prevDash {
			out.WriteByte('-')
			prevDash = true
		}
	}
	trimmed := strings.Trim(out.String(), "-")
	if trimmed == "" {
		return "profile"
	}
	return trimmed
}

func uniqueID(profiles []Profile, base, rawLink string) string {
	for _, item := range profiles {
		if item.RawLink == rawLink {
			return item.ID
		}
	}
	if !hasID(profiles, base) {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !hasID(profiles, candidate) {
			return candidate
		}
	}
}

func hasID(profiles []Profile, id string) bool {
	for _, item := range profiles {
		if item.ID == id {
			return true
		}
	}
	return false
}

func toLowerASCII(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + 32
	}
	return r
}
