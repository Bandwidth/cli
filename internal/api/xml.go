package api

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"sort"
)

// XMLBody wraps a request body with the XML root element name.
// Dashboard API commands use this when calling Post/Put on an XML client.
type XMLBody struct {
	RootElement string
	Data        map[string]interface{}
}

// MapToXML serializes a flat (or nested) Go map to XML with the given root element.
// Values that are maps are recursively serialized as child elements.
// Slice values produce repeated elements with the same tag.
func MapToXML(rootElement string, data map[string]interface{}) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)

	if err := writeMapAsElement(&buf, rootElement, data); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// writeMapAsElement writes <tag>...children...</tag> where children come from the map.
func writeMapAsElement(buf *bytes.Buffer, tag string, data map[string]interface{}) error {
	buf.WriteString("<" + tag + ">")

	// Sort keys for deterministic output.
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if err := writeValue(buf, k, data[k]); err != nil {
			return err
		}
	}

	buf.WriteString("</" + tag + ">")
	return nil
}

// writeValue writes a single key-value pair as XML.
func writeValue(buf *bytes.Buffer, tag string, value interface{}) error {
	switch v := value.(type) {
	case map[string]interface{}:
		return writeMapAsElement(buf, tag, v)

	case map[string]string:
		buf.WriteString("<" + tag + ">")
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			buf.WriteString("<" + k + ">")
			xml.EscapeText(buf, []byte(v[k])) //nolint:errcheck
			buf.WriteString("</" + k + ">")
		}
		buf.WriteString("</" + tag + ">")

	case []interface{}:
		for _, item := range v {
			if err := writeValue(buf, tag, item); err != nil {
				return err
			}
		}

	case []string:
		for _, s := range v {
			buf.WriteString("<" + tag + ">")
			xml.EscapeText(buf, []byte(s)) //nolint:errcheck
			buf.WriteString("</" + tag + ">")
		}

	default:
		buf.WriteString("<" + tag + ">")
		xml.EscapeText(buf, []byte(fmt.Sprintf("%v", v))) //nolint:errcheck
		buf.WriteString("</" + tag + ">")
	}

	return nil
}

// XMLToMap parses an XML document into a nested map[string]interface{}.
// The returned map uses the root element as a top-level key, so callers get
// the full structure including the root element name.
// Repeated sibling elements with the same tag are collected into []interface{}.
// Text-only elements are represented as strings.
func XMLToMap(data []byte) (map[string]interface{}, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	result, err := decodeNextElement(decoder)
	if err != nil {
		return nil, fmt.Errorf("parsing XML: %w", err)
	}
	if m, ok := result.(map[string]interface{}); ok {
		return m, nil
	}
	return map[string]interface{}{"value": fmt.Sprintf("%v", result)}, nil
}

// decodeNextElement advances the decoder to the next start element and decodes it.
func decodeNextElement(decoder *xml.Decoder) (interface{}, error) {
	for {
		tok, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		if start, ok := tok.(xml.StartElement); ok {
			return decodeElement(decoder, start)
		}
	}
}

// decodeElement decodes the content of an already-opened start element and
// returns map[string]interface{}{tagName: content} where content is either a
// string (text-only), or a map[string]interface{} (has child elements).
func decodeElement(decoder *xml.Decoder, start xml.StartElement) (interface{}, error) {
	children := map[string]interface{}{}
	var textBuf bytes.Buffer
	hasChildren := false

	for {
		tok, err := decoder.Token()
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			hasChildren = true
			child, err := decodeElement(decoder, t)
			if err != nil {
				return nil, err
			}
			// child is map[string]interface{}{childTag: childContent}
			childMap, ok := child.(map[string]interface{})
			if !ok {
				continue
			}
			for key, val := range childMap {
				if existing, exists := children[key]; exists {
					switch e := existing.(type) {
					case []interface{}:
						children[key] = append(e, val)
					default:
						children[key] = []interface{}{e, val}
					}
				} else {
					children[key] = val
				}
			}

		case xml.CharData:
			textBuf.Write(t)

		case xml.EndElement:
			tag := start.Name.Local
			if !hasChildren {
				text := string(bytes.TrimSpace(textBuf.Bytes()))
				return map[string]interface{}{tag: text}, nil
			}
			return map[string]interface{}{tag: children}, nil
		}
	}
}
