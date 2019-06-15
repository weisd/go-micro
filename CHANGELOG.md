## 更新日志

### 添加 api 返回值接口，方便自定义

```

// ErrHandler 错误处理
type ErrHandler func(w http.ResponseWriter, r *http.Request, err *errors.Error)

// RespHandler 成功返回值处理
type RespHandler func(w http.ResponseWriter, r *http.Request, rsp *api.Response)

// DefaultErrHandler DefaultErrHandler
var DefaultErrHandler = func(w http.ResponseWriter, r *http.Request, err *errors.Error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(500)
	w.Write([]byte(err.Error()))
	return
}

// DefaultRespHandler DefaultRespHandler
var DefaultRespHandler = func(w http.ResponseWriter, r *http.Request, rsp *api.Response) {
	for _, header := range rsp.GetHeader() {
		for _, val := range header.Values {
			w.Header().Add(header.Key, val)
		}
	}

	if len(w.Header().Get("Content-Type")) == 0 {
		w.Header().Set("Content-Type", "application/json")
	}

	w.WriteHeader(int(rsp.StatusCode))
	w.Write([]byte(rsp.Body))
}

```