/*
Copyright 2017 The Kubernetes Authors.
Copyright 2018 Intel Corporation.

SPDX-License-Identifier: Apache-2.0
*/

package pmemcsidriver

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	api "github.com/intel/pmem-csi/pkg/apis/pmemcsi/v1alpha1"
	grpcserver "github.com/intel/pmem-csi/pkg/grpc-server"
	"github.com/intel/pmem-csi/pkg/metrics-merger"
	pmdmanager "github.com/intel/pmem-csi/pkg/pmem-device-manager"
	pmemgrpc "github.com/intel/pmem-csi/pkg/pmem-grpc"
	registry "github.com/intel/pmem-csi/pkg/pmem-registry"
	pmemstate "github.com/intel/pmem-csi/pkg/pmem-state"
	"github.com/intel/pmem-csi/pkg/registryserver"
	"github.com/intel/pmem-csi/pkg/scheduler"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

const (
	connectionTimeout time.Duration = 10 * time.Second
	retryTimeout      time.Duration = 10 * time.Second
	requestTimeout    time.Duration = 10 * time.Second
)

type DriverMode string

func (mode *DriverMode) Set(value string) error {
	switch value {
	case string(Controller), string(Node):
		*mode = DriverMode(value)
	default:
		// The flag package will add the value to the final output, no need to do it here.
		return errors.New("invalid driver mode")
	}
	return nil
}

func (mode *DriverMode) String() string {
	return string(*mode)
}

const (
	//Controller definition for controller driver mode
	Controller DriverMode = "controller"
	//Node definition for noder driver mode
	Node DriverMode = "node"
)

var (
	//PmemDriverTopologyKey key to use for topology constraint
	PmemDriverTopologyKey = ""

	// Mirrored after https://github.com/kubernetes/component-base/blob/dae26a37dccb958eac96bc9dedcecf0eb0690f0f/metrics/version.go#L21-L37
	// just with less information.
	buildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "build_info",
			Help: "A metric with a constant '1' value labeled by version.",
		},
		[]string{"version"},
	)
)

func init() {
	prometheus.MustRegister(buildInfo)
}

var (
	endpointsFormat = `^([a-z]+)=(.+)$`
	endpointsRE     = regexp.MustCompile(endpointsFormat)
)

// Endpoints maps a unique prefix to a URL.
type Endpoints map[string]*url.URL

func (endpoints *Endpoints) Set(value string) error {
	parts := endpointsRE.FindStringSubmatch(value)
	if parts == nil {
		return fmt.Errorf("must match regular expression %q", endpointsFormat)
	}
	url, err := url.Parse(parts[2])
	if err != nil {
		return fmt.Errorf("%s must be a URL: %v", parts[2], err)
	}
	if *endpoints == nil {
		*endpoints = Endpoints{}
	}
	(*endpoints)[parts[1]] = url
	return nil
}

func (endpoints *Endpoints) String() string {
	// Unused, only needed for flag.Value interface.
	return ""
}

//Config type for driver configuration
type Config struct {
	//DriverName name of the csi driver
	DriverName string
	//NodeID node id on which this csi driver is running
	NodeID string
	//Endpoint exported csi driver endpoint
	Endpoint string
	//TestEndpoint adds the controller service to the server listening on Endpoint.
	//Only needed for testing.
	TestEndpoint bool
	//Mode mode fo the driver
	Mode DriverMode
	//RegistryEndpoint exported registry server endpoint
	RegistryEndpoint string
	//CAFile Root certificate authority certificate file
	CAFile string
	//CertFile certificate for server authentication
	CertFile string
	//KeyFile server private key file
	KeyFile string
	//ClientCertFile certificate for client side authentication
	ClientCertFile string
	//ClientKeyFile client private key
	ClientKeyFile string
	//ControllerEndpoint exported node controller endpoint
	ControllerEndpoint string
	//DeviceManager device manager to use
	DeviceManager api.DeviceMode
	//Directory where to persist the node driver state
	StateBasePath string
	//Version driver release version
	Version string

	// parameters for Kubernetes scheduler extender
	schedulerListen string
	client          kubernetes.Interface

	// parameters for Prometheus metrics
	metricsListen string
	metricsPath   string
	metricsMerge  Endpoints
}

type pmemDriver struct {
	cfg             Config
	serverTLSConfig *tls.Config
	clientTLSConfig *tls.Config
}

func GetPMEMDriver(cfg Config) (*pmemDriver, error) {
	validModes := map[DriverMode]struct{}{
		Controller: struct{}{},
		Node:       struct{}{},
	}
	var serverConfig *tls.Config
	var clientConfig *tls.Config
	var err error

	if _, ok := validModes[cfg.Mode]; !ok {
		return nil, fmt.Errorf("Invalid driver mode: %s", string(cfg.Mode))
	}
	if cfg.DriverName == "" || cfg.NodeID == "" || cfg.Endpoint == "" {
		return nil, fmt.Errorf("One of mandatory(Drivername Node id or Endpoint) configuration option missing")
	}
	if cfg.RegistryEndpoint == "" {
		cfg.RegistryEndpoint = cfg.Endpoint
	}
	if cfg.ControllerEndpoint == "" {
		cfg.ControllerEndpoint = cfg.Endpoint
	}

	if cfg.Mode == Node && cfg.StateBasePath == "" {
		cfg.StateBasePath = "/var/lib/" + cfg.DriverName
	}

	peerName := "pmem-registry"
	if cfg.Mode == Controller {
		//When driver running in Controller mode, we connect to node controllers
		//so use appropriate peer name
		peerName = "pmem-node-controller"
	}

	if cfg.CertFile != "" && cfg.KeyFile != "" {
		serverConfig, err = pmemgrpc.LoadServerTLS(cfg.CAFile, cfg.CertFile, cfg.KeyFile, peerName)
		if err != nil {
			return nil, err
		}
	}

	/* if no client certificate details provided use same server certificate to connect to peer server */
	if cfg.ClientCertFile == "" {
		cfg.ClientCertFile = cfg.CertFile
		cfg.ClientKeyFile = cfg.KeyFile
	}

	if cfg.ClientCertFile != "" && cfg.ClientKeyFile != "" {
		clientConfig, err = pmemgrpc.LoadClientTLS(cfg.CAFile, cfg.ClientCertFile, cfg.ClientKeyFile, peerName)
		if err != nil {
			return nil, err
		}
	}

	PmemDriverTopologyKey = cfg.DriverName + "/node"

	// Should GetPMEMDriver get called more than once per process,
	// all of them will record their version.
	buildInfo.With(prometheus.Labels{"version": cfg.Version}).Set(1)

	return &pmemDriver{
		cfg:             cfg,
		serverTLSConfig: serverConfig,
		clientTLSConfig: clientConfig,
	}, nil
}

func (pmemd *pmemDriver) Run() error {
	// Create GRPC servers
	ids, err := NewIdentityServer(pmemd.cfg.DriverName, pmemd.cfg.Version)
	if err != nil {
		return err
	}

	s := grpcserver.NewNonBlockingGRPCServer()
	// Ensure that the server is stopped before we return.
	defer func() {
		s.ForceStop()
		s.Wait()
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if pmemd.cfg.Mode == Controller {
		rs := registryserver.New(pmemd.clientTLSConfig)
		cs := NewMasterControllerServer(rs)

		if pmemd.cfg.Endpoint != pmemd.cfg.RegistryEndpoint {
			if err := s.Start(pmemd.cfg.Endpoint, nil, ids, cs); err != nil {
				return err
			}
			if err := s.Start(pmemd.cfg.RegistryEndpoint, pmemd.serverTLSConfig, rs); err != nil {
				return err
			}
		} else {
			if err := s.Start(pmemd.cfg.Endpoint, pmemd.serverTLSConfig, ids, cs, rs); err != nil {
				return err
			}
		}

		// Also run scheduler extender?
		if _, err := pmemd.startScheduler(ctx, cancel, rs); err != nil {
			return err
		}
		// And metrics server?
		addr, err := pmemd.startMetrics(ctx, cancel)
		if err != nil {
			return err
		}
		klog.V(2).Infof("Prometheus endpoint started at https://%s%s", addr, pmemd.cfg.metricsPath)
	} else if pmemd.cfg.Mode == Node {
		dm, err := newDeviceManager(pmemd.cfg.DeviceManager)
		if err != nil {
			return err
		}
		sm, err := pmemstate.NewFileState(pmemd.cfg.StateBasePath)
		if err != nil {
			return err
		}
		cs := NewNodeControllerServer(pmemd.cfg.NodeID, dm, sm)
		ns := NewNodeServer(cs, filepath.Clean(pmemd.cfg.StateBasePath)+"/mount")

		if pmemd.cfg.Endpoint != pmemd.cfg.ControllerEndpoint {
			if err := s.Start(pmemd.cfg.ControllerEndpoint, pmemd.serverTLSConfig, cs); err != nil {
				return err
			}
			if err := pmemd.registerNodeController(); err != nil {
				return err
			}
			services := []grpcserver.PmemService{ids, ns}
			if pmemd.cfg.TestEndpoint {
				services = append(services, cs)
			}
			if err := s.Start(pmemd.cfg.Endpoint, nil, services...); err != nil {
				return err
			}
		} else {
			if err := s.Start(pmemd.cfg.Endpoint, nil, ids, cs, ns); err != nil {
				return err
			}
			if err := pmemd.registerNodeController(); err != nil {
				return err
			}
		}
	} else {
		return fmt.Errorf("Unsupported device mode '%v", pmemd.cfg.Mode)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	select {
	case sig := <-c:
		// Here we want to shut down cleanly, i.e. let running
		// gRPC calls complete.
		klog.V(3).Infof("Caught signal %s, terminating.", sig)
		if pmemd.cfg.Mode == Node {
			klog.V(3).Info("Unregistering node...")
			if err := pmemd.unregisterNodeController(); err != nil {
				klog.V(4).Infof("Failed to node unregister: %v", err)
			}
		}

	case <-ctx.Done():
		// The scheduler HTTP server must have failed (to start).
		// We quit in that case.
	}
	s.Stop()
	s.Wait()

	return nil
}

func (pmemd *pmemDriver) registerNodeController() error {
	var err error
	var conn *grpc.ClientConn

	for {
		klog.V(3).Infof("Connecting to registry server at: %s\n", pmemd.cfg.RegistryEndpoint)
		conn, err = pmemgrpc.Connect(pmemd.cfg.RegistryEndpoint, pmemd.clientTLSConfig)
		if err == nil {
			break
		}
		klog.V(4).Infof("Failed to connect registry server: %s, retrying after %v seconds...", err.Error(), retryTimeout.Seconds())
		time.Sleep(retryTimeout)
	}

	req := &registry.RegisterControllerRequest{
		NodeId:   pmemd.cfg.NodeID,
		Endpoint: pmemd.cfg.ControllerEndpoint,
	}

	if err := register(context.Background(), conn, req); err != nil {
		return err
	}
	go waitAndWatchConnection(conn, req)

	return nil
}

func (pmemd *pmemDriver) unregisterNodeController() error {
	req := &registry.UnregisterControllerRequest{
		NodeId: pmemd.cfg.NodeID,
	}
	conn, err := pmemgrpc.Connect(pmemd.cfg.RegistryEndpoint, pmemd.clientTLSConfig)
	if err != nil {
		return err
	}

	client := registry.NewRegistryClient(conn)
	_, err = client.UnregisterController(context.Background(), req)

	return err
}

// startScheduler starts the scheduler extender if it is enabled. It
// logs errors and cancels the context when it runs into a problem,
// either during the startup phase (blocking) or later at runtime (in
// a go routine).
func (pmemd *pmemDriver) startScheduler(ctx context.Context, cancel func(), rs *registryserver.RegistryServer) (string, error) {
	if pmemd.cfg.schedulerListen == "" {
		return "", nil
	}

	resyncPeriod := 1 * time.Hour
	factory := informers.NewSharedInformerFactory(pmemd.cfg.client, resyncPeriod)
	pvcLister := factory.Core().V1().PersistentVolumeClaims().Lister()
	scLister := factory.Storage().V1().StorageClasses().Lister()
	sched, err := scheduler.NewScheduler(
		pmemd.cfg.DriverName,
		scheduler.CapacityViaRegistry(rs),
		pmemd.cfg.client,
		pvcLister,
		scLister,
	)
	if err != nil {
		return "", fmt.Errorf("create scheduler: %v", err)
	}
	factory.Start(ctx.Done())
	cacheSyncResult := factory.WaitForCacheSync(ctx.Done())
	klog.V(5).Infof("synchronized caches: %+v", cacheSyncResult)
	for t, v := range cacheSyncResult {
		if !v {
			return "", fmt.Errorf("failed to sync informer for type %v", t)
		}
	}
	return pmemd.startHTTPSServer(ctx, cancel, pmemd.cfg.schedulerListen, sched)
}

// startMetrics starts the HTTPS server for the Prometheus endpoint, if one is configured.
// Error handling is the same as for startScheduler.
func (pmemd *pmemDriver) startMetrics(ctx context.Context, cancel func()) (string, error) {
	if pmemd.cfg.metricsListen == "" {
		return "", nil
	}

	// We must merge our own internal metrics data with the data from the sidecars.
	handler := metricsmerger.Handler{
		ExportersHTTPTimeout: 10, // seconds
	}
	// We use the default Prometheus handler here and thus return all data that
	// is registered globally, including (but not limited to!) our own metrics
	// data. For example, some Go runtime information (https://povilasv.me/prometheus-go-metrics/)
	// are included, which may be useful.
	handler.Handlers = append(handler.Handlers, promhttp.Handler())

	for _, url := range pmemd.cfg.metricsMerge {
		// TODO: handle prefix
		handler.Exporters = append(handler.Exporters, url.String())
	}

	mux := http.NewServeMux()
	mux.Handle(pmemd.cfg.metricsPath, handler)
	return pmemd.startHTTPSServer(ctx, cancel, pmemd.cfg.metricsListen, mux)
}

// startHTTPSServer contains the common logic for starting and
// stopping an HTTPS server.  Returns an error or the address that can
// be used in Dial("tcp") to reach the server (useful for testing when
// "listen" does not include a port).
func (pmemd *pmemDriver) startHTTPSServer(ctx context.Context, cancel func(), listen string, handler http.Handler) (string, error) {
	config, err := pmemgrpc.LoadServerTLS(pmemd.cfg.CAFile, pmemd.cfg.CertFile, pmemd.cfg.KeyFile, "")
	if err != nil {
		return "", fmt.Errorf("initialize HTTPS config: %v", err)
	}
	server := http.Server{
		Addr: listen,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			klog.V(5).Infof("HTTP request: %s %q from %s %s", r.Method, r.URL.Path, r.RemoteAddr, r.UserAgent())
			handler.ServeHTTP(w, r)
		}),
		TLSConfig: config,
	}
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return "", fmt.Errorf("listen on TCP address %q: %v", listen, err)
	}
	tcpListener := listener.(*net.TCPListener)
	go func() {
		defer tcpListener.Close()

		err := server.ServeTLS(listener, pmemd.cfg.CertFile, pmemd.cfg.KeyFile)
		if err != http.ErrServerClosed {
			klog.Errorf("%s HTTPS server error: %v", listen, err)
		}
		// Also stop main thread.
		cancel()
	}()
	go func() {
		// Block until the context is done, then immediately
		// close the server.
		<-ctx.Done()
		server.Close()
	}()

	return tcpListener.Addr().String(), nil
}

// waitAndWatchConnection Keeps watching for connection changes, and whenever the
// connection state changed from lost to ready, it re-register the node controller with registry server.
func waitAndWatchConnection(conn *grpc.ClientConn, req *registry.RegisterControllerRequest) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	connectionLost := false

	for {
		s := conn.GetState()
		if s == connectivity.Ready {
			if connectionLost {
				klog.V(4).Info("ReConnected.")
				if err := register(ctx, conn, req); err != nil {
					klog.Warning(err)
				}
			}
		} else {
			connectionLost = true
			klog.V(4).Info("Connection state: ", s)
		}
		conn.WaitForStateChange(ctx, s)
	}
}

// register Tries to register with RegistryServer in endless loop till,
// either the registration succeeds or RegisterController() returns only possible InvalidArgument error.
func register(ctx context.Context, conn *grpc.ClientConn, req *registry.RegisterControllerRequest) error {
	client := registry.NewRegistryClient(conn)
	for {
		klog.V(3).Info("Registering controller...")
		if _, err := client.RegisterController(ctx, req); err != nil {
			if s, ok := status.FromError(err); ok && s.Code() == codes.InvalidArgument {
				return fmt.Errorf("Registration failed: %s", s.Message())
			}
			klog.V(5).Infof("Failed to register: %s, retrying after %v seconds...", err.Error(), retryTimeout.Seconds())
			time.Sleep(retryTimeout)
		} else {
			break
		}
	}
	klog.V(4).Info("Registration success")

	return nil
}

func newDeviceManager(dmType api.DeviceMode) (pmdmanager.PmemDeviceManager, error) {
	switch dmType {
	case api.DeviceModeLVM:
		return pmdmanager.NewPmemDeviceManagerLVM()
	case api.DeviceModeDirect:
		return pmdmanager.NewPmemDeviceManagerNdctl()
	}
	return nil, fmt.Errorf("Unsupported device manager type '%s'", dmType)
}
