package lamlam

import "encoding/json"

type payloadType struct {
	FuncKey string `json:"funcKey"`
	Data    string `json:"data"`
}

func (p *payloadType) setData(src any) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}

	p.Data = string(data)
	return nil
}

func (p *payloadType) bindData(dst any) error {
	return json.Unmarshal([]byte(p.Data), dst)
}
