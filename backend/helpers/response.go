package helpers

// Response: format {code, message, data} yang sama dipakai di seluruh ekosistem ini
// (sudocore2/APIANDORDER) -- code 0 = sukses, selain itu = error.
type Response struct {
	Code    *int   `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func NewResponse() *Response {
	return &Response{}
}

func (res *Response) SetCode(code int) *Response {
	res.Code = &code
	return res
}

func (res *Response) SetMessage(message string) *Response {
	res.Message = message
	return res
}

func (res *Response) SetData(data any) *Response {
	res.Data = data
	return res
}

func (res *Response) Success() *Response {
	code := 0
	res.Code = &code
	res.Message = "success"
	return res
}

func (res *Response) GeneralError() *Response {
	code := 100
	res.Code = &code
	res.Message = "error"
	return res
}
