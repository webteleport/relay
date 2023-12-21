package session

import (
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

	return json.Marshal(m)
}
