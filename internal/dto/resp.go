package dto

type Resp struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	Username string `json:"username"`
}
