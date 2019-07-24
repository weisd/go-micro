package grpc

import (
	"time"

	"github.com/micro/go-micro"
	mclient "github.com/micro/go-micro/client"
	server "github.com/micro/go-micro/server"
	"github.com/micro/go-micro"
	broker "github.com/micro/go-plugins/broker/grpc"
	client "github.com/micro/go-micro/client/grpc"
	server "github.com/micro/go-micro/server/grpc"
)

// NewService returns a grpc service compatible with go-micro.Service
func NewService(opts ...micro.Option) micro.Service {
	// our grpc broker
	b := broker.NewBroker()
	// our grpc client
	c := client.NewClient(mclient.Broker(b))
	// our grpc server
	s := server.NewServer(mserver.Broker(b))
	

	// create options with priority for our opts
	options := []micro.Option{
		micro.Client(c),
		micro.Server(s),
		micro.Broker(b),
	}

	// append passed in opts
	options = append(options, opts...)

	// generate and return a service
	return micro.NewService(options...)
}

// NewFunction returns a grpc service compatible with go-micro.Function
func NewFunction(opts ...micro.Option) micro.Function {
	// our grpc broker
	b := broker.NewBroker()
	// our grpc client
	c := client.NewClient(mclient.Broker(b))
	// our grpc server
	s := server.NewServer(mserver.Broker(b))

	// create options with priority for our opts
	options := []micro.Option{
		micro.Client(c),
		micro.Server(s),
		micro.Broker(b),
		micro.RegisterTTL(time.Minute),
		micro.RegisterInterval(time.Second * 30),
	}

	// append passed in opts
	options = append(options, opts...)

	// generate and return a function
	return micro.NewFunction(options...)
}
