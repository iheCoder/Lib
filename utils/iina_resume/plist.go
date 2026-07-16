package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	historyURLKey  = "IINAPHUrl"
	historyNameKey = "IINAPHNme"
	historyHashKey = "IINAPHMpvmd5"
	archiveUIDKey  = "CF$UID"
)

// decodePlaybackHistory converts the small subset of NSKeyedArchiver's object
// graph used by IINA into a case-normalized MD5 index.
func decodePlaybackHistory(data []byte) (map[string]historyRecord, error) {
	rootValue, err := parseXMLPropertyList(data)
	if err != nil {
		return nil, err
	}
	root, ok := rootValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("playback history root is not a dictionary")
	}
	objects, ok := root["$objects"].([]any)
	if !ok {
		return nil, fmt.Errorf("playback history has no object table")
	}
	return indexHistoryObjects(objects), nil
}

// indexHistoryObjects scans for PlaybackHistory-shaped dictionaries instead of
// relying on archive class-table indexes, which may change between IINA builds.
func indexHistoryObjects(objects []any) map[string]historyRecord {
	index := make(map[string]historyRecord)
	for _, object := range objects {
		entry, ok := object.(map[string]any)
		if !ok || entry[historyHashKey] == nil || entry[historyURLKey] == nil {
			continue
		}
		hash, hashOK := resolveArchiveString(objects, entry[historyHashKey])
		path, pathOK := resolveArchiveURL(objects, entry[historyURLKey])
		if !hashOK || !pathOK {
			continue
		}
		name, _ := resolveArchiveString(objects, entry[historyNameKey])
		index[strings.ToUpper(hash)] = historyRecord{Path: path, Name: name}
	}
	return index
}

// resolveArchiveURL follows NSURL's `NS.relative` reference and normalizes file
// URLs to paths accepted by iina-cli. Remote URLs remain unchanged.
func resolveArchiveURL(objects []any, reference any) (string, bool) {
	object, ok := resolveArchiveObject(objects, reference)
	if !ok {
		return "", false
	}
	urlObject, ok := object.(map[string]any)
	if !ok {
		return "", false
	}
	rawURL, ok := resolveArchiveString(objects, urlObject["NS.relative"])
	if !ok {
		return "", false
	}
	parsed, err := url.Parse(rawURL)
	if err == nil && parsed.IsAbs() && parsed.Scheme == "file" {
		return parsed.Path, true
	}
	return rawURL, rawURL != ""
}

func resolveArchiveString(objects []any, reference any) (string, bool) {
	object, ok := resolveArchiveObject(objects, reference)
	if !ok {
		return "", false
	}
	value, ok := object.(string)
	return value, ok
}

// resolveArchiveObject validates UIDs before indexing because a malformed or
// partially-written history file must fail closed rather than panic the service.
func resolveArchiveObject(objects []any, reference any) (any, bool) {
	uidObject, ok := reference.(map[string]any)
	if !ok {
		return nil, false
	}
	uid, ok := uidObject[archiveUIDKey].(int64)
	if !ok || uid < 0 || uid >= int64(len(objects)) {
		return nil, false
	}
	return objects[uid], true
}

// parseXMLPropertyList is deliberately a minimal, dependency-free plist reader.
// plutil has already normalized binary input, so only standard XML plist nodes
// and CF$UID dictionaries need to be represented.
func parseXMLPropertyList(data []byte) (any, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("read property list: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local == "plist" {
			continue
		}
		return decodePlistElement(decoder, start)
	}
}

func decodePlistElement(decoder *xml.Decoder, start xml.StartElement) (any, error) {
	switch start.Name.Local {
	case "dict":
		return decodePlistDictionary(decoder)
	case "array":
		return decodePlistArray(decoder)
	case "string", "date", "data":
		return decodePlistText(decoder, start)
	case "integer":
		return decodePlistInteger(decoder, start)
	case "real":
		return decodePlistReal(decoder, start)
	case "true", "false":
		if err := decoder.Skip(); err != nil {
			return nil, err
		}
		return start.Name.Local == "true", nil
	default:
		return nil, fmt.Errorf("unsupported plist element %q", start.Name.Local)
	}
}

// decodePlistDictionary consumes alternating key/value elements until the
// enclosing dict ends; whitespace and comments are naturally ignored.
func decodePlistDictionary(decoder *xml.Decoder) (map[string]any, error) {
	result := make(map[string]any)
	for {
		start, ended, err := nextPlistElement(decoder, "dict")
		if err != nil || ended {
			return result, err
		}
		if start.Name.Local != "key" {
			return nil, fmt.Errorf("plist dictionary contains %q instead of key", start.Name.Local)
		}
		key, err := decodePlistText(decoder, start)
		if err != nil {
			return nil, err
		}
		valueStart, ended, err := nextPlistElement(decoder, "dict")
		if err != nil || ended {
			return nil, fmt.Errorf("plist key %q has no value", key)
		}
		result[key], err = decodePlistElement(decoder, valueStart)
		if err != nil {
			return nil, err
		}
	}
}

// decodePlistArray preserves archive object indexes; NSKeyedArchiver UIDs rely
// on positional lookup and cannot tolerate filtering or reordering.
func decodePlistArray(decoder *xml.Decoder) ([]any, error) {
	var result []any
	for {
		start, ended, err := nextPlistElement(decoder, "array")
		if err != nil || ended {
			return result, err
		}
		value, err := decodePlistElement(decoder, start)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
}

func nextPlistElement(decoder *xml.Decoder, parent string) (xml.StartElement, bool, error) {
	for {
		token, err := decoder.Token()
		if err != nil {
			return xml.StartElement{}, false, err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			return typed, false, nil
		case xml.EndElement:
			if typed.Name.Local == parent {
				return xml.StartElement{}, true, nil
			}
		}
	}
}

func decodePlistText(decoder *xml.Decoder, start xml.StartElement) (string, error) {
	var value string
	err := decoder.DecodeElement(&value, &start)
	return value, err
}

func decodePlistInteger(decoder *xml.Decoder, start xml.StartElement) (int64, error) {
	value, err := decodePlistText(decoder, start)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(value, 10, 64)
}

func decodePlistReal(decoder *xml.Decoder, start xml.StartElement) (float64, error) {
	value, err := decodePlistText(decoder, start)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(value, 64)
}
