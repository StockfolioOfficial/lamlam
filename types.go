package lamlam

import "encoding/json"

type payloadType struct {
	FuncKey string          `json:"funcKey"`
	Data    json.RawMessage `json:"data"`
}

func (p *payloadType) setData(src any) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}

	p.Data = data
	return nil
}

func (p *payloadType) bindData(dst any) error {
	return json.Unmarshal(p.Data, dst)
}
