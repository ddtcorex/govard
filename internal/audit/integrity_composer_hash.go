package audit

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Composer decides whether a `composer.lock` still belongs to its
// `composer.json` by hashing a subset of the manifest and storing the digest as
// `content-hash`. Reproducing that digest byte for byte is the only way to
// answer "is this lock stale?" without running PHP, and it is what this file
// does.
//
// It has to reproduce *PHP*, not JSON: Composer's hash is
// `md5(json_encode($relevantContent))`, and PHP's encoder differs from Go's in
// ways that all change the digest — it escapes forward slashes, it escapes
// every non-ASCII character as \uXXXX, and (the one that surprises people) it
// has no way to distinguish an empty JSON object from an empty JSON array, so
// `{}` in a manifest becomes `[]` in the hash input.
//
// A reimplementation that disagrees with Composer would report every project as
// stale, which is worse than not checking at all. The tests pin this against
// digests produced by Composer itself, including one taken from a real
// `composer update` lock file.

// composerHashedKeys are the manifest keys Composer includes in the digest, in
// the order its own implementation lists them. The order only matters before
// the final sort; it is kept for fidelity with the reference implementation.
var composerHashedKeys = []string{
	"name", "version", "require", "require-dev", "conflict", "replace",
	"provide", "minimum-stability", "prefer-stable", "repositories", "extra",
}

// composerContentHash returns the digest Composer would write into
// `composer.lock` for this manifest.
func composerContentHash(manifest []byte) (string, error) {
	parsed, err := parsePHPJSON(manifest)
	if err != nil {
		return "", fmt.Errorf("parse composer.json: %w", err)
	}
	root, ok := parsed.(*phpObject)
	if !ok {
		return "", fmt.Errorf("composer.json must be a JSON object")
	}

	relevant := newPHPObject()
	for _, key := range composerHashedKeys {
		if value, present := root.get(key); present {
			relevant.set(key, value)
		}
	}
	// Composer adds `config.platform` — and only that one key of `config` —
	// when the manifest pins platform packages.
	if config, present := root.get("config"); present {
		if configObject, ok := config.(*phpObject); ok {
			if platform, pinned := configObject.get("platform"); pinned {
				nested := newPHPObject()
				nested.set("platform", platform)
				relevant.set("config", nested)
			}
		}
	}
	relevant.sortKeys()

	var builder bytes.Buffer
	if err := encodePHPJSON(relevant, &builder); err != nil {
		return "", err
	}
	digest := md5.Sum(builder.Bytes())
	return hex.EncodeToString(digest[:]), nil
}

// ComposerContentHashForTest exposes composerContentHash to the tests/ package.
func ComposerContentHashForTest(manifest []byte) (string, error) {
	return composerContentHash(manifest)
}

// phpObject is a JSON object that remembers the order of its keys, because the
// digest depends on the order the manifest's nested values are written in.
type phpObject struct {
	keys   []string
	values map[string]phpValue
}

// phpValue is one of *phpObject, []phpValue, string, json.Number, bool or nil:
// exactly the value set PHP's json_decode produces from a manifest.
type phpValue any

func newPHPObject() *phpObject {
	return &phpObject{values: map[string]phpValue{}}
}

func (o *phpObject) set(key string, value phpValue) {
	if _, present := o.values[key]; !present {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
}

func (o *phpObject) get(key string) (phpValue, bool) {
	value, present := o.values[key]
	return value, present
}

func (o *phpObject) sortKeys() { sort.Strings(o.keys) }

// parsePHPJSON decodes JSON without losing key order or numeric literals.
func parsePHPJSON(raw []byte) (phpValue, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	value, err := decodePHPValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("unexpected trailing content")
	}
	return value, nil
}

func decodePHPValue(decoder *json.Decoder) (phpValue, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch typed := token.(type) {
	case json.Delim:
		switch typed {
		case '{':
			object := newPHPObject()
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, fmt.Errorf("object key is not a string")
				}
				value, err := decodePHPValue(decoder)
				if err != nil {
					return nil, err
				}
				object.set(key, value)
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
			return object, nil
		case '[':
			list := []phpValue{}
			for decoder.More() {
				value, err := decodePHPValue(decoder)
				if err != nil {
					return nil, err
				}
				list = append(list, value)
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
			return list, nil
		default:
			return nil, fmt.Errorf("unexpected delimiter %q", typed)
		}
	case string, bool, nil, json.Number:
		return typed, nil
	default:
		return nil, fmt.Errorf("unsupported JSON token %T", token)
	}
}

// encodePHPJSON writes the value the way PHP's json_encode would.
func encodePHPJSON(value phpValue, out *bytes.Buffer) error {
	switch typed := value.(type) {
	case *phpObject:
		if len(typed.keys) == 0 {
			// PHP cannot tell `{}` from `[]` after decoding, and re-encodes the
			// empty case as an array. Composer's digest depends on that.
			out.WriteString("[]")
			return nil
		}
		out.WriteByte('{')
		for index, key := range typed.keys {
			if index > 0 {
				out.WriteByte(',')
			}
			writePHPJSONString(key, out)
			out.WriteByte(':')
			if err := encodePHPJSON(typed.values[key], out); err != nil {
				return err
			}
		}
		out.WriteByte('}')
		return nil
	case []phpValue:
		out.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				out.WriteByte(',')
			}
			if err := encodePHPJSON(item, out); err != nil {
				return err
			}
		}
		out.WriteByte(']')
		return nil
	case string:
		writePHPJSONString(typed, out)
		return nil
	case json.Number:
		writePHPJSONNumber(typed, out)
		return nil
	case bool:
		if typed {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
		return nil
	case nil:
		out.WriteString("null")
		return nil
	default:
		return fmt.Errorf("cannot encode %T", value)
	}
}

// writePHPJSONString escapes a string exactly as PHP's json_encode does with
// default flags: forward slashes are escaped, `<`/`>`/`&` are not, and every
// character outside ASCII becomes a \uXXXX escape.
func writePHPJSONString(value string, out *bytes.Buffer) {
	out.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"':
			out.WriteString(`\"`)
		case '\\':
			out.WriteString(`\\`)
		case '/':
			out.WriteString(`\/`)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			switch {
			case r < 0x20:
				fmt.Fprintf(out, `\u%04x`, r)
			case r < 0x80:
				out.WriteRune(r)
			case r > 0xFFFF:
				// PHP writes the UTF-16 surrogate pair, in lowercase hex.
				offset := r - 0x10000
				fmt.Fprintf(out, `\u%04x\u%04x`, 0xD800+(offset>>10), 0xDC00+(offset&0x3FF))
			default:
				fmt.Fprintf(out, `\u%04x`, r)
			}
		}
	}
	out.WriteByte('"')
}

// writePHPJSONNumber reproduces PHP's number rendering: an integer literal is
// written as it was read, and a decimal is written in its shortest round-trip
// form with a decimal point, which is what `serialize_precision = -1` produces.
func writePHPJSONNumber(value json.Number, out *bytes.Buffer) {
	text := value.String()
	if !strings.ContainsAny(text, ".eE") {
		out.WriteString(text)
		return
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		out.WriteString(text)
		return
	}
	rendered := strconv.FormatFloat(parsed, 'g', -1, 64)
	if !strings.ContainsAny(rendered, ".eE") {
		rendered += ".0"
	}
	out.WriteString(rendered)
}
