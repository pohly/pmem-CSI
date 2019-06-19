/*
Copyright 2017 The Kubernetes Authors.
Copyright 2018 Intel Corporation.

SPDX-License-Identifier: Apache-2.0
*/

package pmemcsidriver

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	pmdmanager "github.com/intel/pmem-csi/pkg/pmem-device-manager"
	pmemgrpc "github.com/intel/pmem-csi/pkg/pmem-grpc"
	registry "github.com/intel/pmem-csi/pkg/pmem-registry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/glog"
)

const (
	connectionTimeout time.Duration = 10 * time.Second
	retryTimeout      time.Duration = 10 * time.Second
	requestTimeout    time.Duration = 10 * time.Second
)

type DriverMode string

const (
	//Controller defintion for controller driver mode
	Controller DriverMode = "controller"
	//Node defintion for noder driver mode
	Node DriverMode = "node"
)

var (
	//PmemDriverTopologyKey key to use for topology constraint
	PmemDriverTopologyKey = ""
)

//Config type for driver configuration
type Config struct {
	//DriverName name of the csi driver
	DriverName string
	//NodeID node id on which this csi driver is running
	NodeID string
	//Endpoint exported csi driver endpoint
	Endpoint string
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
	DeviceManager string
	//Version driver release version
	Version string
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

	s := NewNonBlockingGRPCServer()
	// Ensure that the server is stopped before we return.
	defer func() {
		s.ForceStop()
		s.Wait()
	}()

	if pmemd.cfg.Mode == Controller {
		rs := NewRegistryServer(pmemd.clientTLSConfig)
		cs := NewMasterControllerServer(rs)

		// Trigger the race detector to verify that it is active.
		// TODO: remove this
		if true {
			registerReq := registry.RegisterControllerRequest{
				NodeId:   "foo",
				Endpoint: "bar",
			}
			var wg sync.WaitGroup
			wg.Add(2)
			register := func() {
				defer wg.Done()
				// The Go race detector will see two accesses to the same
				// global data in the registry server from two different
				// go routines and warn about it unless the data is
				// protected by a lock.
				rs.RegisterController(context.Background(), &registerReq)
			}
			go register()
			go register()
			wg.Wait()

			// Keep testing.
			go func() {
				rs.RegisterController(context.Background(), &registerReq)
				time.Sleep(30 * time.Second)
			}()
		}

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
	} else if pmemd.cfg.Mode == Node {
		dm, err := newDeviceManager(pmemd.cfg.DeviceManager)
		if err != nil {
			return err
		}
		ns := NewNodeServer(pmemd.cfg.NodeID, dm)
		cs := NewNodeControllerServer(pmemd.cfg.NodeID, dm)

		if pmemd.cfg.Endpoint != pmemd.cfg.ControllerEndpoint {
			if err := s.Start(pmemd.cfg.ControllerEndpoint, pmemd.serverTLSConfig, cs); err != nil {
				return err
			}
			if err := pmemd.registerNodeController(); err != nil {
				return err
			}
			if err := s.Start(pmemd.cfg.Endpoint, nil, ids, ns); err != nil {
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
	sig := <-c
	// Here we want to shut down cleanly, i.e. let running
	// gRPC calls complete.
	glog.Infof("Caught signal %s, terminating.", sig)
	s.Stop()
	s.Wait()

	return nil
}

func (pmemd *pmemDriver) registerNodeController() error {
	var err error
	var conn *grpc.ClientConn

	req := registry.RegisterControllerRequest{
		NodeId:   pmemd.cfg.NodeID,
		Endpoint: pmemd.cfg.ControllerEndpoint,
	}

	for {
		glog.V(3).Infof("Connecting to registry server at: %s\n", pmemd.cfg.RegistryEndpoint)
		conn, err = pmemgrpc.Connect(pmemd.cfg.RegistryEndpoint, pmemd.clientTLSConfig, connectionTimeout)
		if err == nil {
			break
		}
		glog.V(4).Infof("Failed to connect registry server: %s, retrying after %v seconds...", err.Error(), retryTimeout.Seconds())
		time.Sleep(retryTimeout)
	}

	client := registry.NewRegistryClient(conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for {
		glog.V(3).Info("Registering controller...")
		if _, err = client.RegisterController(ctx, &req); err != nil {
			if s, ok := status.FromError(err); ok && s.Code() == codes.InvalidArgument {
				return fmt.Errorf("Registration failed: %s", s.Message())
			}
			glog.V(5).Infof("Failed to register: %s, retrying after %v seconds...", err.Error(), retryTimeout.Seconds())
			time.Sleep(retryTimeout)
		} else {
			break
		}
	}
	glog.V(4).Info("Registration success")

	return nil
}

func newDeviceManager(dmType string) (pmdmanager.PmemDeviceManager, error) {
	switch dmType {
	case "lvm":
		return pmdmanager.NewPmemDeviceManagerLVM()
	case "ndctl":
		return pmdmanager.NewPmemDeviceManagerNdctl()
	}
	return nil, fmt.Errorf("Unsupported device manager type '%s'", dmType)
}
