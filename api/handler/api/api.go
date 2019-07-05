// Package api provides an http-rpc handler which provides the entire http request over rpc
package api

import (
	"context"
	"net/http"

	goapi "github.com/micro/go-micro/api"
	"github.com/micro/go-micro/api/handler"
	api "github.com/micro/go-micro/api/proto"
	"github.com/micro/go-micro/client"
	"github.com/micro/go-micro/errors"
	"github.com/micro/go-micro/selector"
	"github.com/micro/go-micro/util/ctx"
)

type apiHandler struct {
	opts handler.Options
	s    *goapi.Service
}

const (
	Handler = "api"
)

// API handler is the default handler which takes api.Request and returns api.Response
func (a *apiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	request, err := requestToProto(r)
	if err != nil {
		DefaultErrHandler(context.Background(), w, r, errors.InternalServerError("go.micro.api", err.Error()).(*errors.Error))
		return
	}

	var service *goapi.Service

	if a.s != nil {
		// we were given the service
		service = a.s
	} else if a.opts.Router != nil {
		// try get service from router
		s, err := a.opts.Router.Route(r)
		if err != nil {
			DefaultErrHandler(context.Background(), w, r, errors.InternalServerError("go.micro.api", err.Error()).(*errors.Error))
			return
		}
		service = s
	} else {
		// we have no way of routing the request
		DefaultErrHandler(context.Background(), w, r, errors.InternalServerError("go.micro.api", "no route found").(*errors.Error))
		return
	}

	// create request and response
	c := a.opts.Service.Client()
	req := c.NewRequest(service.Name, service.Endpoint.Name, request)
	rsp := &api.Response{}

	// create the context from headers
	cx := ctx.FromRequest(r)
	// create strategy
	so := selector.WithStrategy(strategy(service.Services))

	if err := c.Call(cx, req, rsp, client.WithSelectOption(so)); err != nil {
		DefaultErrHandler(cx, w, r, errors.Parse(err.Error()))
		return
	} else if rsp.StatusCode == 0 {
		rsp.StatusCode = http.StatusOK
	}

	DefaultRespHandler(cx, w, r, rsp)
}

func (a *apiHandler) String() string {
	return "api"
}

func NewHandler(opts ...handler.Option) handler.Handler {
	options := handler.NewOptions(opts...)
	return &apiHandler{
		opts: options,
	}
}

func WithService(s *goapi.Service, opts ...handler.Option) handler.Handler {
	options := handler.NewOptions(opts...)
	return &apiHandler{
		opts: options,
		s:    s,
	}
}
