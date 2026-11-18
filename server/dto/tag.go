package dto

import "encoding/json"

type SearchTag struct {
	Type    int8 `json:"type"`     // -1 = any
	Status  int8 `json:"status"`   // -1 = any
	GroupId int  `json:"group_id"` // -1 = any
}

func (t SearchTag) ToString() string {
	data, _ := json.Marshal(t)
	return string(data)
}
