package router

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/micro/go-micro/registry"
	"github.com/olekukonko/tablewriter"
)

var (
	// AdvertiseTick defines how often in seconds do we scal the local registry
	// to advertise the local services to the network registry
	AdvertiseTick = 5 * time.Second
	// AdvertiseTTL defines network registry TTL in seconds
	// NOTE: this is a rather arbitrary picked value subject to change
	AdvertiseTTL = 120 * time.Second
)

type router struct {
	opts Options
	exit chan struct{}
	wg   *sync.WaitGroup
}

// newRouter creates new router and returns it
func newRouter(opts ...Option) Router {
	// get default options
	options := DefaultOptions()

	// apply requested options
	for _, o := range opts {
		o(&options)
	}

	return &router{
		opts: options,
		exit: make(chan struct{}),
		wg:   &sync.WaitGroup{},
	}
}

// Init initializes router with given options
func (r *router) Init(opts ...Option) error {
	for _, o := range opts {
		o(&r.opts)
	}
	return nil
}

// Options returns router options
func (r *router) Options() Options {
	return r.opts
}

// ID returns router ID
func (r *router) ID() string {
	return r.opts.ID
}

// Table returns routing table
func (r *router) Table() Table {
	return r.opts.Table
}

// Address returns router's bind address
func (r *router) Address() string {
	return r.opts.Address
}

// Network returns the address router advertises to the network
func (r *router) Network() string {
	return r.opts.Advertise
}

// Advertise advertises the router routes to the network.
// Advertise is a blocking function. It launches multiple goroutines that watch
// service registries and advertise the router routes to other routers in the network.
// It returns error if any of the launched goroutines fail with error.
func (r *router) Advertise() error {
	// add local service routes into the routing table
	if err := r.addServiceRoutes(r.opts.Registry, DefaultLocalMetric); err != nil {
		return fmt.Errorf("failed adding routes for local services: %v", err)
	}

	// add network service routes into the routing table
	if err := r.addServiceRoutes(r.opts.Network, DefaultNetworkMetric); err != nil {
		return fmt.Errorf("failed adding routes for network services: %v", err)
	}

	node, err := r.parseToNode()
	if err != nil {
		return fmt.Errorf("failed to parse router into service node: %v", err)
	}

	localWatcher, err := r.opts.Registry.Watch()
	if err != nil {
		return fmt.Errorf("failed to create local registry watcher: %v", err)
	}

	networkWatcher, err := r.opts.Network.Watch()
	if err != nil {
		return fmt.Errorf("failed to create network registry watcher: %v", err)
	}

	// error channel collecting goroutine errors
	errChan := make(chan error, 3)

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		// watch local registry and register routes in routine table
		errChan <- r.manageServiceRoutes(localWatcher, DefaultLocalMetric)
	}()

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		// watch network registry and register routes in routine table
		errChan <- r.manageServiceRoutes(networkWatcher, DefaultNetworkMetric)
	}()

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		// watch local registry and advertise local service to the network
		errChan <- r.advertiseToNetwork(node)
	}()

	return <-errChan
}

// addServiceRoutes adds all services in given registry to the routing table.
// NOTE: this is a one-off operation done when bootstrapping the routing table of the new router.
// It returns error if either the services could not be listed or if the routes could not be added to the routing table.
func (r *router) addServiceRoutes(reg registry.Registry, metric int) error {
	services, err := reg.ListServices()
	if err != nil {
		return fmt.Errorf("failed to list services: %v", err)
	}

	for _, service := range services {
		route := Route{
			Destination: service.Name,
			Router:      r,
			Network:     r.opts.Advertise,
			Metric:      metric,
		}
		if err := r.opts.Table.Add(route); err != nil && err != ErrDuplicateRoute {
			return fmt.Errorf("error adding route for service: %s", service.Name)
		}
	}

	return nil
}

// parseToNode parses router into registry.Node and returns the result.
// It returns error if the router network address could not be parsed into host and port.
func (r *router) parseToNode() (*registry.Node, error) {
	// split router address to host and port part
	addr, portStr, err := net.SplitHostPort(r.opts.Advertise)
	if err != nil {
		return nil, fmt.Errorf("could not parse router address: %v", err)
	}

	// try to parse network port into integer
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("could not parse router network address: %v", err)
	}

	node := &registry.Node{
		Id:      r.opts.ID,
		Address: addr,
		Port:    port,
	}

	return node, nil
}

// advertiseToNetwork periodically scans local registry and registers (i.e. advertises) all the local services in the network registry.
// It returns error if either the local services failed to be listed or if it fails to register local service in network registry.
func (r *router) advertiseToNetwork(node *registry.Node) error {
	// ticker to periodically scan the local registry
	ticker := time.NewTicker(AdvertiseTick)

	for {
		select {
		case <-r.exit:
			return nil
		case <-ticker.C:
			// list all local services
			services, err := r.opts.Registry.ListServices()
			if err != nil {
				return fmt.Errorf("failed to list local services: %v", err)
			}
			// loop through all registered local services and register them in the network registry
			for _, service := range services {
				svc := &registry.Service{
					Name:  service.Name,
					Nodes: []*registry.Node{node},
				}
				// register the local service in the network registry
				if err := r.opts.Network.Register(svc, registry.RegisterTTL(AdvertiseTTL)); err != nil {
					return fmt.Errorf("failed to register service %s in network registry: %v", svc.Name, err)
				}
			}
		}
	}
}

// manageServiceRoutes watches services in given registry and updates the routing table accordingly.
// It returns error if the service registry watcher has stopped or if the routing table failed to be updated.
func (r *router) manageServiceRoutes(w registry.Watcher, metric int) error {
	// wait in the background for the router to stop
	// when the router stops, stop the watcher and exit
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		<-r.exit
		w.Stop()
	}()

	var watchErr error

	for {
		res, err := w.Next()
		if err == registry.ErrWatcherStopped {
			break
		}

		if err != nil {
			watchErr = err
			break
		}

		route := Route{
			Destination: res.Service.Name,
			Router:      r,
			Network:     r.opts.Advertise,
			Metric:      metric,
		}

		switch res.Action {
		case "create":
			if len(res.Service.Nodes) > 0 {
				// only return error if the route is not duplicate, but something else has failed
				if err := r.opts.Table.Add(route); err != nil && err != ErrDuplicateRoute {
					return fmt.Errorf("failed to add route for service: %v", res.Service.Name)
				}
			}
		case "delete":
			if len(res.Service.Nodes) < 1 {
				// only return error if the route is present in the table, but something else has failed
				if err := r.opts.Table.Delete(route); err != nil && err != ErrRouteNotFound {
					return fmt.Errorf("failed to delete route for service: %v", res.Service.Name)
				}
			}
		}
	}

	return watchErr
}

// Stop stops the router
func (r *router) Stop() error {
	// notify all goroutines to finish
	close(r.exit)

	// wait for all goroutines to finish
	r.wg.Wait()

	// NOTE: we need a more efficient way of doing this e.g. network routes
	// should ideally be autodeleted when the router stops gossiping
	query := NewQuery(QueryRouter(r), QueryNetwork(r.opts.Advertise))
	routes, err := r.opts.Table.Lookup(query)
	if err != nil && err != ErrRouteNotFound {
		return fmt.Errorf("failed to lookup routes for router %s: %v", r.opts.ID, err)
	}

	// parse router to registry.Node
	node, err := r.parseToNode()
	if err != nil {
		return fmt.Errorf("failed to parse router into service node: %v", err)
	}

	for _, route := range routes {
		service := &registry.Service{
			Name:  route.Destination,
			Nodes: []*registry.Node{node},
		}
		if err := r.opts.Network.Deregister(service); err != nil {
			return fmt.Errorf("failed to deregister service %s from network registry: %v", service.Name, err)
		}
	}

	return nil
}

// String prints debugging information about router
func (r *router) String() string {
	sb := &strings.Builder{}

	table := tablewriter.NewWriter(sb)
	table.SetHeader([]string{"ID", "Address", "Network", "Table"})

	data := []string{
		r.opts.ID,
		r.opts.Address,
		r.opts.Advertise,
		fmt.Sprintf("%d", r.opts.Table.Size()),
	}
	table.Append(data)

	// render table into sb
	table.Render()

	return sb.String()
}
