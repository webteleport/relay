package session

import (
	"bytes"
	"encoding/json"
	"net/url"
)

type Tags struct {
	url.Values
}

func (c Tags) MarshalJSON() ([]byte, error) {
	m := make(map[string]interface{})

	for key, values := range c.Values {
		if len(values) == 1 {
			m[key] = values[0]
		} else {
			m[key] = values
		}
	}

	return UnescapedJSONMarshalIndent(m, "  ")
}

func UnescapedJSONMarshalIndent(v interface{}, indent string) ([]byte, error) {
	var resultBytes bytes.Buffer
	enc := json.NewEncoder(&resultBytes)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", indent)
	err := enc.Encode(v)
	return bytes.TrimSpace(resultBytes.Bytes()), err
}
