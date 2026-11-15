package platform

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// ErrVolumeNotFound means a successful inventory did not contain the UUID.
// Command failures must never be translated to this sentinel.
var ErrVolumeNotFound = errors.New("volume not connected")
var volumeRun = RunDetachedContext

type Volume struct {
	UUID, Name, MountPoint, Filesystem string
	Internal                           bool
}

func FindVolume(uuid string) (Volume, error) {
	data, err := volumeCommand(context.Background(), 5*time.Second, "list", "-plist")
	if err != nil {
		return Volume{}, err
	}
	root, err := parsePlist(data)
	if err != nil {
		return Volume{}, err
	}
	disks, ok := root["AllDisksAndPartitions"].([]any)
	if !ok {
		return Volume{}, errors.New("diskutil inventory has no AllDisksAndPartitions array")
	}
	if err := validateVolumeInventory(disks); err != nil {
		return Volume{}, err
	}
	if !containsVolume(disks, uuid) {
		return Volume{}, ErrVolumeNotFound
	}
	return VolumeAt(uuid)
}

func validateVolumeInventory(items []any) error {
	for _, item := range items {
		values, ok := item.(map[string]any)
		if !ok {
			return errors.New("diskutil inventory contains a non-dictionary disk")
		}
		if uuid, exists := values["VolumeUUID"]; exists {
			if value, ok := uuid.(string); !ok || value == "" {
				return errors.New("diskutil inventory contains an invalid volume UUID")
			}
		}
		for _, key := range []string{"Partitions", "APFSVolumes"} {
			if value, exists := values[key]; exists {
				children, ok := value.([]any)
				if !ok {
					return fmt.Errorf("diskutil inventory %s is not an array", key)
				}
				if err := validateVolumeInventory(children); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func containsVolume(value any, uuid string) bool {
	switch v := value.(type) {
	case map[string]any:
		if id, ok := v["VolumeUUID"].(string); ok && strings.EqualFold(id, uuid) {
			return true
		}
		for _, child := range v {
			if containsVolume(child, uuid) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if containsVolume(child, uuid) {
				return true
			}
		}
	}
	return false
}

// VolumeAt also accepts a UUID. Callers checking a path must compare MountPoint
// with that path: diskutil may describe the enclosing filesystem instead.
func VolumeAt(path string) (Volume, error) {
	data, err := volumeCommand(context.Background(), 5*time.Second, "info", "-plist", path)
	if err != nil {
		return Volume{}, err
	}
	values, err := parsePlist(data)
	if err != nil {
		return Volume{}, err
	}
	text := func(key string) string { v, _ := values[key].(string); return v }
	v := Volume{UUID: text("VolumeUUID"), Name: text("VolumeName"), MountPoint: text("MountPoint"), Filesystem: text("FilesystemType")}
	if v.UUID == "" {
		return Volume{}, errors.New("diskutil info has no volume UUID")
	}
	v.Internal, _ = values["Internal"].(bool)
	return v, nil
}

func MountVolume(ctx context.Context, uuid string) error {
	_, err := volumeCommand(ctx, 90*time.Second, "mount", uuid)
	return err
}

func volumeCommand(parent context.Context, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	out, code, err := volumeRun(ctx, "diskutil", args...)
	if err != nil {
		return "", fmt.Errorf("diskutil %s: %w", args[0], err)
	}
	if code != 0 {
		message := strings.TrimSpace(out)
		if values, parseErr := parsePlist(out); parseErr == nil {
			if explanation, ok := values["ErrorMessage"].(string); ok && strings.TrimSpace(explanation) != "" {
				message = strings.TrimSpace(explanation)
			}
		}
		return "", fmt.Errorf("diskutil %s exited %d: %s", args[0], code, message)
	}
	return out, nil
}

// decodePlistValue preserves the plist hierarchy, including nested APFS arrays.
// It rejects structurally invalid dictionaries instead of treating them as absence.
func decodePlistValue(d *xml.Decoder, start xml.StartElement) (any, error) {
	switch start.Name.Local {
	case "dict":
		values := map[string]any{}
		for {
			token, err := d.Token()
			if err != nil {
				return nil, err
			}
			switch t := token.(type) {
			case xml.EndElement:
				return values, nil
			case xml.StartElement:
				if t.Name.Local != "key" {
					return nil, errors.New("plist dictionary expected key")
				}
				var key string
				if err := d.DecodeElement(&key, &t); err != nil {
					return nil, err
				}
				if _, ok := values[key]; ok {
					return nil, fmt.Errorf("duplicate plist key %q", key)
				}
				for {
					token, err = d.Token()
					if err != nil {
						return nil, err
					}
					if next, ok := token.(xml.StartElement); ok {
						value, err := decodePlistValue(d, next)
						if err != nil {
							return nil, err
						}
						values[key] = value
						break
					}
					if _, ok := token.(xml.EndElement); ok {
						return nil, errors.New("plist key has no value")
					}
				}
			}
		}
	case "array":
		values := []any{}
		for {
			token, err := d.Token()
			if err != nil {
				return nil, err
			}
			switch t := token.(type) {
			case xml.EndElement:
				return values, nil
			case xml.StartElement:
				value, err := decodePlistValue(d, t)
				if err != nil {
					return nil, err
				}
				values = append(values, value)
			}
		}
	case "true", "false":
		if err := d.Skip(); err != nil {
			return nil, err
		}
		return start.Name.Local == "true", nil
	case "string", "integer", "real", "date", "data":
		var value string
		if err := d.DecodeElement(&value, &start); err != nil {
			return nil, err
		}
		if start.Name.Local == "string" {
			return value, nil
		}
		return struct{}{}, nil
	default:
		return nil, fmt.Errorf("unsupported plist element %q", start.Name.Local)
	}
}

func parsePlist(data string) (map[string]any, error) {
	d := xml.NewDecoder(strings.NewReader(data))
	var result map[string]any
	var inPlist, closed bool
	for {
		token, err := d.Token()
		if err == io.EOF {
			if result == nil || !closed {
				return nil, errors.New("incomplete diskutil plist")
			}
			return result, nil
		}
		if err != nil {
			return nil, fmt.Errorf("parse diskutil plist: %w", err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			if !inPlist && !closed && t.Name.Local == "plist" {
				inPlist = true
				continue
			}
			if !inPlist || result != nil || t.Name.Local != "dict" {
				return nil, errors.New("expected one plist dictionary")
			}
			value, err := decodePlistValue(d, t)
			if err != nil {
				return nil, err
			}
			result = value.(map[string]any)
		case xml.EndElement:
			if t.Name.Local == "plist" {
				inPlist = false
				closed = true
			}
		case xml.CharData:
			if strings.TrimSpace(string(t)) != "" {
				return nil, errors.New("unexpected text in plist")
			}
		}
	}
}
